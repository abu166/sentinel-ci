package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

type Step struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Stage     string   `json:"stage"`
	Command   string   `json:"command"`
	DependsOn []string `json:"depends_on"`
}
type Job struct {
	PipelineID string            `json:"pipeline_id"`
	RepoURL    string            `json:"repo_url"`
	CommitSHA  string            `json:"commit_sha"`
	Branch     string            `json:"branch"`
	Steps      []Step            `json:"steps"`
	Files      map[string]string `json:"files"`
}
type Log struct {
	PipelineID string    `json:"pipeline_id"`
	StepID     string    `json:"step_id"`
	Sequence   int64     `json:"sequence_num"`
	Stream     string    `json:"stream"`
	Content    string    `json:"content"`
	Timestamp  time.Time `json:"timestamp"`
}
type RunnerDB interface {
	Begin(context.Context) (pgx.Tx, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	SendBatch(context.Context, *pgx.Batch) pgx.BatchResults
}
type Runner struct {
	db      RunnerDB
	redis   *redis.Client
	pub     *amqp.Channel
	mu      sync.Mutex
	timeout time.Duration
	sandbox SandboxExecutor
}

// appendTrace retains the most recent failure context without unbounded memory growth.
func appendTrace(buffer *bytes.Buffer, line string) {
	const limit = 2_000_000
	chunk := []byte(line + "\n")
	if len(chunk) >= limit {
		buffer.Reset()
		buffer.Write(chunk[len(chunk)-limit:])
		return
	}
	if excess := buffer.Len() + len(chunk) - limit; excess > 0 {
		buffer.Next(excess)
	}
	buffer.Write(chunk)
}

func (r *Runner) publish(ctx context.Context, key string, v any) error {
	ctx, span := otel.Tracer("sentinel/runner").Start(ctx, "rabbitmq publish "+key, trace.WithSpanKind(trace.SpanKindProducer), trace.WithAttributes(attribute.String("messaging.system", "rabbitmq"), attribute.String("messaging.destination.name", key)))
	defer span.End()
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	confirmation, err := r.pub.PublishWithDeferredConfirmWithContext(ctx, "sentinel", key, false, false, amqp.Publishing{Headers: rabbitTraceHeaders(ctx), DeliveryMode: amqp.Persistent, ContentType: "application/json", Body: b})
	if err != nil {
		return err
	}
	ok, err := confirmation.WaitContext(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("broker rejected publication")
	}
	return nil
}
func safeWrite(root, path, content string) error {
	if filepath.IsAbs(path) || path == "." || strings.HasPrefix(filepath.Clean(path), "..") {
		return errors.New("unsafe source path")
	}
	p := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(content), 0600)
}
func collectFiles(root string) (map[string]string, error) {
	files := map[string]string{}
	total := int64(0)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == ".venv" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		ext := filepath.Ext(path)
		if ext != ".go" && ext != ".py" && d.Name() != "go.mod" && d.Name() != "go.sum" && d.Name() != "requirements.txt" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > 1<<20 || total+info.Size() > 4<<20 || len(files) >= 100 {
			return errors.New("repair source snapshot exceeds size/file limits")
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		total += info.Size()
		files[rel] = string(b)
		return nil
	})
	return files, err
}
func (r *Runner) execute(ctx context.Context, j Job) (resultErr error) {
	ctx, jobSpan := otel.Tracer("sentinel/runner").Start(ctx, "pipeline execute", trace.WithAttributes(attribute.String("sentinel.pipeline_id", j.PipelineID)))
	defer func() {
		if resultErr != nil {
			jobSpan.SetStatus(codes.Error, "pipeline execution failed")
		}
		jobSpan.End()
	}()
	lease, acquired, err := acquireLease(ctx, r.redis, "pipeline-lock:"+j.PipelineID)
	if err != nil {
		return err
	}
	if !acquired {
		return nil
	}
	defer lease.Close()
	ctx = lease.Context
	var previousStatus string
	if err = r.db.QueryRow(ctx, "SELECT status FROM pipelines WHERE id=$1", j.PipelineID).Scan(&previousStatus); err != nil {
		return err
	}
	if pipelineTerminal(previousStatus) {
		return nil
	}

	root, err := os.MkdirTemp("", "sentinel-job-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	defer r.sandbox.Cleanup(root)
	if len(j.Files) > 0 {
		for p, c := range j.Files {
			if err = safeWrite(root, p, c); err != nil {
				return err
			}
		}
	} else {
		if !strings.HasPrefix(j.RepoURL, "https://") {
			return errors.New("repository must use HTTPS")
		}
		cloneMasker := maskerFromEnvironment()
		cloneCtx, cancel := context.WithTimeout(ctx, r.timeout)
		code, _, err := runCommand(cloneCtx, root, "git clone --depth 1 -- "+shellQuote(j.RepoURL)+" .", func(stream, line string) {
			safe, _ := cloneMasker.Filter(stream, line)
			slog.Debug("clone", "stream", stream, "line", safe)
		})
		cancel()
		if err != nil || code != 0 {
			return fmt.Errorf("clone failed: exit=%d error=%v", code, err)
		}
		ref := j.CommitSHA
		if ref == "" {
			ref = j.Branch
		}
		if ref != "" {
			if strings.HasPrefix(ref, "-") || strings.ContainsAny(ref, "\r\n") {
				return errors.New("invalid git ref")
			}
			checkoutCtx, cancel := context.WithTimeout(ctx, r.timeout)
			code, _, err = runCommand(checkoutCtx, root, "git fetch --depth 1 origin "+shellQuote(ref)+" && git checkout --detach FETCH_HEAD", func(stream, line string) {
				safe, _ := cloneMasker.Filter(stream, line)
				slog.Debug("checkout", "stream", stream, "line", safe)
			})
			cancel()
			if err != nil || code != 0 {
				return fmt.Errorf("checkout failed: exit=%d error=%v", code, err)
			}
		}
	}
	if _, err = r.db.Exec(ctx, "UPDATE pipelines SET status='RUNNING',started_at=now() WHERE id=$1", j.PipelineID); err != nil {
		return err
	}
	done := map[string]bool{}
	for len(done) < len(j.Steps) {
		progress := false
		for _, s := range j.Steps {
			if done[s.Name] {
				continue
			}
			ready := true
			for _, dep := range s.DependsOn {
				ready = ready && done[dep]
			}
			if !ready {
				continue
			}
			progress = true
			decision, claimErr := r.claimStep(ctx, j.PipelineID, s.ID, lease.token)
			if claimErr != nil {
				return claimErr
			}
			if decision == decisionSkip {
				done[s.Name] = true
				continue
			}
			if decision == decisionInterrupted {
				return interruptedStep(s.ID)
			}
			if _, err = r.db.Exec(ctx, "UPDATE pipeline_steps SET status='RUNNING',started_at=now() WHERE id=$1", s.ID); err != nil {
				return err
			}
			stepCtx, cancel := context.WithTimeout(ctx, r.timeout)
			stepCtx, stepSpan := otel.Tracer("sentinel/runner").Start(stepCtx, "step execute", trace.WithAttributes(attribute.String("sentinel.pipeline_id", j.PipelineID), attribute.String("sentinel.step_id", s.ID)))
			start := time.Now()
			var seq atomic.Int64
			var previousSequence int64
			if err = r.db.QueryRow(ctx, "SELECT coalesce(max(sequence_num),0) FROM log_chunks WHERE step_id=$1", s.ID).Scan(&previousSequence); err != nil {
				stepSpan.SetStatus(codes.Error, "log sequence unavailable")
				stepSpan.End()
				cancel()
				return err
			}
			seq.Store(previousSequence)
			var buffer bytes.Buffer
			masker := maskerFromEnvironment()
			var logMu sync.Mutex
			var logErr error
			pending := make([]Log, 0, 32)
			flush := func() {
				if len(pending) == 0 {
					return
				}
				batch := &pgx.Batch{}
				for _, entry := range pending {
					batch.Queue("INSERT INTO log_chunks(step_id,sequence_num,stream,content,timestamp) VALUES($1,$2,$3,$4,$5)", entry.StepID, entry.Sequence, entry.Stream, entry.Content, entry.Timestamp)
				}
				result := r.db.SendBatch(ctx, batch)
				if e := result.Close(); e != nil {
					logErr = e
				}
				pending = pending[:0]
			}
			emit := func(stream, line string) {
				logMu.Lock()
				defer logMu.Unlock()
				line, _ = masker.Filter(stream, line)
				entry := Log{PipelineID: j.PipelineID, StepID: s.ID, Sequence: seq.Add(1), Stream: stream, Content: line, Timestamp: time.Now().UTC()}
				appendTrace(&buffer, line)
				b, _ := json.Marshal(entry)
				pending = append(pending, entry)
				if len(pending) >= 32 {
					flush()
				}
				if e := r.publish(stepCtx, "logs.stream", entry); e != nil {
					logErr = e
				}
				if e := r.redis.Publish(stepCtx, "pipeline:"+j.PipelineID+":logs", b).Err(); e != nil {
					logErr = e
				}
			}
			code, usage, execErr := r.sandbox.Execute(stepCtx, root, s.Command, emit)
			memory := usage.PeakRSSBytes
			flush()
			stepSpan.SetAttributes(attribute.Int("process.exit_code", code), attribute.Int("log.redaction.count", masker.Redactions()))
			if code != 0 {
				stepSpan.SetStatus(codes.Error, "step command failed")
			}
			stepSpan.End()
			cancel()
			if execErr != nil {
				slog.Warn("step execution", "error", execErr)
			}
			status := "PASSED"
			if code != 0 {
				status = "FAILED"
			}
			policyJSON, _ := json.Marshal(usage.Policy)
			if _, err = r.db.Exec(ctx, "UPDATE pipeline_steps SET status=$2,exit_code=$3,duration_ms=$4,finished_at=now(),peak_rss_bytes=$5,cpu_user_ms=$6,cpu_system_ms=$7,sandbox_policy=$8 WHERE id=$1", s.ID, status, code, time.Since(start).Milliseconds(), usage.PeakRSSBytes, usage.CPUUserMS, usage.CPUSystemMS, policyJSON); err != nil {
				return err
			}
			if _, err = r.db.Exec(ctx, "UPDATE step_executions SET status=$2,finished_at=now() WHERE step_id=$1", s.ID, map[bool]string{true: "COMPLETED", false: "FAILED"}[code == 0]); err != nil {
				return err
			}
			slog.Info("step complete", "pipeline", j.PipelineID, "step", s.Name, "exit_code", code, "memory_high_water", memory, "duration_ms", time.Since(start).Milliseconds())
			if logErr != nil {
				return fmt.Errorf("log delivery: %w", logErr)
			}
			if code != 0 {
				files, e := collectFiles(root)
				if e != nil {
					return e
				}

				j.Files = files
				snapshot, e := json.Marshal(j)
				if e != nil {
					return e
				}
				if _, e = r.db.Exec(ctx, "INSERT INTO job_snapshots(pipeline_id,payload) VALUES($1,$2) ON CONFLICT(pipeline_id) DO UPDATE SET payload=EXCLUDED.payload", j.PipelineID, snapshot); e != nil {
					return e
				}
				language := "python"
				for path := range files {
					if strings.HasSuffix(path, ".go") {
						language = "go"
						break
					}
				}
				return r.enqueueHealing(trace.ContextWithSpanContext(ctx, stepSpan.SpanContext()), j, s, buffer.String(), files, language)
			}
			done[s.Name] = true
		}
		if !progress {
			return errors.New("invalid or cyclic DAG")
		}
	}
	_, err = r.db.Exec(ctx, "UPDATE pipelines SET status='PASSED',finished_at=now() WHERE id=$1", j.PipelineID)
	return err
}
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

func heartbeatHealth(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	options, err := redis.ParseURL(env("REDIS_URL", "redis://localhost:6379/0"))
	if err != nil {
		return err
	}
	n, err := strconv.Atoi(env("WORKER_CONCURRENCY", "4"))
	if err != nil || n < 1 || n > 64 {
		return errors.New("invalid worker concurrency")
	}
	client := redis.NewClient(options)
	defer client.Close()
	keys := make([]string, n)
	for i := range keys {
		keys[i] = fmt.Sprintf("worker:%s:%d", env("HOSTNAME", "local"), i)
	}
	values, err := client.MGet(ctx, keys...).Result()
	if err != nil {
		return err
	}
	return checkHeartbeatValues(values, time.Now())
}

func checkHeartbeatValues(values []interface{}, now time.Time) error {
	if len(values) == 0 {
		return errors.New("no workers configured")
	}
	for i, value := range values {
		stamp, ok := value.(string)
		if !ok {
			return fmt.Errorf("worker %d heartbeat missing", i)
		}
		seen, err := time.Parse(time.RFC3339, stamp)
		if err != nil || now.Sub(seen) > 15*time.Second || seen.Sub(now) > 5*time.Second {
			return fmt.Errorf("worker %d heartbeat invalid or stale", i)
		}
	}
	return nil
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		if err := heartbeatHealth(context.Background()); err != nil {
			slog.Error("worker healthcheck", "error", err)
			os.Exit(1)
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	shutdownTracing, err := setupTracing(ctx)
	if err != nil {
		panic(err)
	}
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTracing(flushCtx)
	}()
	db, err := pgxpool.New(ctx, env("DATABASE_URL", "postgres://sentinel:sentinel@localhost:5432/sentinel?sslmode=disable"))
	if err != nil {
		panic(err)
	}
	defer db.Close()
	opts, err := redis.ParseURL(env("REDIS_URL", "redis://localhost:6379/0"))
	if err != nil {
		panic(err)
	}
	rc := redis.NewClient(opts)
	defer rc.Close()
	conn, err := amqp.Dial(env("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"))
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	go func() {
		select {
		case err := <-conn.NotifyClose(make(chan *amqp.Error, 1)):
			if err != nil {
				slog.Error("broker disconnected; restarting worker", "error", err)
				stop()
			}
		case <-ctx.Done():
		}
	}()
	ch, err := conn.Channel()
	if err != nil {
		panic(err)
	}
	defer ch.Close()
	pub, err := conn.Channel()
	if err != nil {
		panic(err)
	}
	defer pub.Close()
	if err = ch.ExchangeDeclare("sentinel", "topic", true, false, false, false, nil); err != nil {
		panic(err)
	}
	if _, err = ch.QueueDeclare("jobs.dispatch", true, false, false, false, nil); err != nil {
		panic(err)
	}
	if err = ch.QueueBind("jobs.dispatch", "jobs.dispatch", "sentinel", false, nil); err != nil {
		panic(err)
	}
	n, err := strconv.Atoi(env("WORKER_CONCURRENCY", "4"))
	if err != nil || n < 1 || n > 64 {
		panic("WORKER_CONCURRENCY must be 1..64")
	}
	if err = ch.Qos(n, 0, false); err != nil {
		panic(err)
	}
	timeout, err := time.ParseDuration(env("STEP_TIMEOUT", "2m"))
	if err != nil {
		panic(err)
	}
	if err = pub.Confirm(false); err != nil {
		panic(err)
	}
	sandbox, err := sandboxConfig()
	if err != nil {
		panic(err)
	}
	if err = sandbox.Reap(ctx); err != nil {
		panic(err)
	}
	r := &Runner{db: db, redis: rc, pub: pub, timeout: timeout, sandbox: sandbox}
	deliveries, err := ch.Consume("jobs.dispatch", "", false, false, false, false, nil)
	if err != nil {
		panic(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			workerCtx, workerCancel := context.WithCancel(ctx)
			defer workerCancel()
			key := fmt.Sprintf("worker:%s:%d", env("HOSTNAME", "local"), i)
			heartbeat := time.NewTicker(5 * time.Second)
			defer heartbeat.Stop()
			go func() {
				for {
					stamp := time.Now().UTC().Format(time.RFC3339)
					rc.Set(workerCtx, key, stamp, 15*time.Second)
					info := workerTelemetry(i, stamp, r.sandbox.policy())
					data, _ := json.Marshal(info)
					rc.Set(workerCtx, fmt.Sprintf("workerinfo:%s:%d", env("HOSTNAME", "local"), i), data, 15*time.Second)
					select {
					case <-workerCtx.Done():
						return
					case <-heartbeat.C:
					}
				}
			}()
			defer rc.Del(context.Background(), key)
			for {
				select {
				case <-ctx.Done():
					return
				case d, ok := <-deliveries:
					if !ok {
						return
					}
					var j Job
					if err := json.Unmarshal(d.Body, &j); err != nil {
						d.Reject(false)
						continue
					}
					consumeCtx, consumeSpan := otel.Tracer("sentinel/runner").Start(extractRabbitTrace(ctx, d.Headers), "rabbitmq consume jobs.dispatch", trace.WithSpanKind(trace.SpanKindConsumer), trace.WithAttributes(attribute.String("messaging.system", "rabbitmq"), attribute.String("sentinel.pipeline_id", j.PipelineID)))
					executeErr := r.execute(consumeCtx, j)
					if executeErr != nil {
						consumeSpan.SetStatus(codes.Error, "job execution failed")
					}
					consumeSpan.End()
					if err := executeErr; err != nil {
						slog.Error("job failed", "pipeline", j.PipelineID, "error", err)
						if ctx.Err() != nil {
							d.Nack(false, true)
							return
						}
						_, _ = db.Exec(ctx, "UPDATE pipelines SET status='FAILED',finished_at=now() WHERE id=$1", j.PipelineID)
						d.Nack(false, false)
					} else {
						d.Ack(false)
					}
				}
			}
		}(i)
	}
	wg.Wait()
}
