package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"time"
)

var errInterrupted = errors.New("previous execution interrupted; create a new pipeline to retry safely")

type executionDecision string

const (
	decisionRun         executionDecision = "run"
	decisionSkip        executionDecision = "skip"
	decisionInterrupted executionDecision = "interrupted"
)

func decideExecution(status string, exists bool) executionDecision {
	if !exists {
		return decisionRun
	}
	if status == "COMPLETED" {
		return decisionSkip
	}
	return decisionInterrupted
}

type redisLease struct {
	client     *redis.Client
	key, token string
	cancel     context.CancelFunc
	done       chan struct{}
	Context    context.Context
}

func acquireLease(ctx context.Context, client *redis.Client, key string) (*redisLease, bool, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return nil, false, err
	}
	token := hex.EncodeToString(b)
	ok, err := client.SetNX(ctx, key, token, 30*time.Second).Result()
	if err != nil || !ok {
		return nil, ok, err
	}
	renewCtx, cancel := context.WithCancel(ctx)
	l := &redisLease{client: client, key: key, token: token, cancel: cancel, done: make(chan struct{}), Context: renewCtx}
	go func() {
		defer close(l.done)
		tick := time.NewTicker(5 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-renewCtx.Done():
				return
			case <-tick.C:
				result, err := client.Eval(renewCtx, `if redis.call('GET',KEYS[1]) == ARGV[1] then return redis.call('PEXPIRE',KEYS[1],30000) else return 0 end`, []string{key}, token).Int()
				if err != nil || result != 1 {
					cancel()
					return
				}
			}
		}
	}()
	return l, true, nil
}
func (l *redisLease) Close() {
	l.cancel()
	<-l.done
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = l.client.Eval(ctx, `if redis.call('GET',KEYS[1]) == ARGV[1] then return redis.call('DEL',KEYS[1]) else return 0 end`, []string{l.key}, l.token).Err()
}
func (r *Runner) claimStep(ctx context.Context, pipeline, step, owner string) (executionDecision, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var old string
	err = tx.QueryRow(ctx, "SELECT status FROM step_executions WHERE step_id=$1 FOR UPDATE", step).Scan(&old)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	decision := decideExecution(old, err == nil)
	if decision == decisionRun {
		tag, e := tx.Exec(ctx, "INSERT INTO step_executions(step_id,pipeline_id,owner,status) VALUES($1,$2,$3,'RUNNING') ON CONFLICT(step_id) DO NOTHING", step, pipeline, owner)
		if e != nil {
			return "", e
		}
		if tag.RowsAffected() != 1 {
			return decisionInterrupted, nil
		}
	}
	if decision == decisionInterrupted {
		if _, err = tx.Exec(ctx, "UPDATE step_executions SET status='INTERRUPTED',finished_at=now() WHERE step_id=$1 AND status='RUNNING'", step); err != nil {
			return "", err
		}
	}
	return decision, tx.Commit(ctx)
}
func (r *Runner) enqueueHealing(ctx context.Context, j Job, step Step, logs string, files map[string]string, language string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	payload, err := jsonPayload(map[string]any{"pipeline_id": j.PipelineID, "step_id": step.ID, "logs": logs, "files": files, "test_command": step.Command, "language": language})
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE pipelines SET status='HEALING' WHERE id=$1", j.PipelineID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO outbox_events(aggregate_type,aggregate_id,event_type,payload,idempotency_key,headers) VALUES('pipeline',$1,'healer.trigger',$2,$3,$4) ON CONFLICT(idempotency_key) DO NOTHING", j.PipelineID, payload, "healer.trigger:"+step.ID, traceHeadersJSON(ctx))
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func jsonPayload(v any) ([]byte, error) { return json.Marshal(v) }
func pipelineTerminal(status string) bool {
	return status == "PASSED" || status == "HEALED" || status == "FAILED" || status == "HEALING"
}
func interruptedStep(step string) error { return fmt.Errorf("%w: step %s", errInterrupted, step) }
