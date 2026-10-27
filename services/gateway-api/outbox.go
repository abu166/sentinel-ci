package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var errPatchReview = errors.New("patch is rejected or no longer verified")

func auditTx(ctx context.Context, tx pgx.Tx, action, resource, id string, details any) error {
	b, e := json.Marshal(details)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "INSERT INTO audit_logs(id,actor,action,resource_type,resource_id,details) VALUES($1,$2,$3,$4,$5,$6)", uuid(), actor(ctx).Username, action, resource, id, b)
	return e
}
func (a *App) createJob(ctx context.Context, repo string, j *Job) error {
	if j.IdempotencyKey == "" {
		j.IdempotencyKey = "pipeline:" + j.PipelineID
	}
	tx, e := a.db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "INSERT INTO pipelines(id,repo_id,commit_sha,branch,status,triggered_by,started_at,commit_message) VALUES($1,$2,$3,$4,'PENDING',$5,now(),$6)", j.PipelineID, repo, j.CommitSHA, j.Branch, actor(ctx).Username, j.CommitMessage); e != nil {
		return e
	}
	for i := range j.Steps {
		s := &j.Steps[i]
		s.ID = uuid()
		if s.DependsOn == nil {
			s.DependsOn = []string{}
		}
		if _, e = tx.Exec(ctx, "INSERT INTO pipeline_steps(id,pipeline_id,name,stage,command,status,depends_on) VALUES($1,$2,$3,$4,$5,'PENDING',$6)", s.ID, j.PipelineID, s.Name, s.Stage, s.Command, s.DependsOn); e != nil {
			return e
		}
	}
	snapshot, e := json.Marshal(j)
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO job_snapshots(pipeline_id,payload) VALUES($1,$2)", j.PipelineID, snapshot); e != nil {
		return e
	}
	tag, e := tx.Exec(ctx, "INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload,idempotency_key,status,attempts,next_retry_at,headers) VALUES($1,'pipeline',$2,'jobs.dispatch',$3,$4,'PENDING',0,now(),$5) ON CONFLICT(idempotency_key) DO NOTHING", uuid(), j.PipelineID, snapshot, j.IdempotencyKey, traceHeadersJSON(ctx))
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		if e = tx.Rollback(ctx); e != nil && !errors.Is(e, pgx.ErrTxClosed) {
			return e
		}
		return a.db.QueryRow(ctx, "SELECT aggregate_id::text FROM outbox_events WHERE idempotency_key=$1", j.IdempotencyKey).Scan(&j.PipelineID)
	}
	if j.SourcePatchID != "" {
		tag, e = tx.Exec(ctx, "UPDATE healing_patches SET approval_status='APPROVED',approved_by=$2,approved_at=now() WHERE id=$1 AND verification_status='VERIFIED' AND approval_status<>'REJECTED' AND selected_candidate_id IS NOT DISTINCT FROM $3::uuid", j.SourcePatchID, actor(ctx).Username, j.SourceCandidateID)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return errPatchReview
		}
		if e = auditTx(ctx, tx, "patch.approve", "patch", j.SourcePatchID, map[string]any{"rerun_pipeline_id": j.PipelineID}); e != nil {
			return e
		}
		var openPR bool
		if e = tx.QueryRow(ctx, "SELECT open_pr FROM repositories WHERE id=$1", repo).Scan(&openPR); e != nil {
			return e
		}
		gitPayload, e := json.Marshal(map[string]any{"patch_id": j.SourcePatchID, "approved": true, "open_pr": openPR})
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload,idempotency_key,status,attempts,next_retry_at,headers) VALUES($1,'patch',$2,'git.publish',$3,$4,'PENDING',0,now(),$5) ON CONFLICT(idempotency_key) DO NOTHING", uuid(), j.SourcePatchID, gitPayload, "git.publish:"+j.SourcePatchID, traceHeadersJSON(ctx)); e != nil {
			return e
		}
	}
	if e = auditTx(ctx, tx, "pipeline.trigger", "pipeline", j.PipelineID, map[string]any{"repository_id": repo, "commit_sha": j.CommitSHA, "idempotency_key": j.IdempotencyKey}); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 10 {
		attempt = 10
	}
	return time.Second * time.Duration(1<<uint(attempt-1))
}
func (a *App) relayOne(ctx context.Context) (bool, error) {
	tx, e := a.db.Begin(ctx)
	if e != nil {
		return false, e
	}
	defer tx.Rollback(ctx)
	var id, kind string
	var payload, headers []byte
	var attempts int
	e = tx.QueryRow(ctx, "SELECT id::text,event_type,payload,attempts,headers FROM outbox_events WHERE status='PENDING' AND next_retry_at<=now() ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1").Scan(&id, &kind, &payload, &attempts, &headers)
	if errors.Is(e, pgx.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	spanCtx, span := otel.Tracer("sentinel/gateway/outbox").Start(restoreTrace(ctx, headers), "outbox.relay "+kind, trace.WithSpanKind(trace.SpanKindInternal), trace.WithAttributes(attribute.String("outbox.event_id", id), attribute.String("messaging.operation.name", kind)))
	defer span.End()
	publishCtx, cancel := context.WithTimeout(spanCtx, 5*time.Second)
	defer cancel()
	var value any
	if e = json.Unmarshal(payload, &value); e == nil {
		e = a.publish(publishCtx, kind, value)
	}
	attempts++
	if e != nil {
		span.RecordError(e)
		span.SetStatus(codes.Error, "Outbox dispatch failed")
		status := "PENDING"
		if attempts >= 8 {
			status = "DEAD"
		}
		if _, updateErr := tx.Exec(ctx, "UPDATE outbox_events SET status=$2,attempts=$3,next_retry_at=$4,last_error=$5 WHERE id=$1", id, status, attempts, time.Now().Add(retryDelay(attempts)), truncate(e.Error(), 2000)); updateErr != nil {
			return false, updateErr
		}
	} else {
		if _, e = tx.Exec(ctx, "UPDATE outbox_events SET status='PUBLISHED',attempts=$2,last_error='',published_at=now() WHERE id=$1", id, attempts); e != nil {
			return false, e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return false, e
	}
	return true, nil
}
func (a *App) relayOutbox(ctx context.Context) {
	for {
		worked, e := a.relayOne(ctx)
		if e != nil && ctx.Err() == nil {
			slog.Error("outbox relay", "error", e)
		}
		if worked && e == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
}
func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
func validateRepoPolicy(auto, open, manual bool) error {
	if open && !auto {
		return fmt.Errorf("open_pr requires auto_commit")
	}
	if !manual && !auto {
		return errors.New("automatic healing requires auto_commit")
	}
	return nil
}
