package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func uuid() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	s := hex.EncodeToString(b)
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}

type Step struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Stage     string   `json:"stage"`
	Command   string   `json:"command"`
	DependsOn []string `json:"depends_on"`
}
type Job struct {
	PipelineID        string            `json:"pipeline_id"`
	RepoURL           string            `json:"repo_url"`
	CommitSHA         string            `json:"commit_sha"`
	Branch            string            `json:"branch"`
	Steps             []Step            `json:"steps"`
	Files             map[string]string `json:"files,omitempty"`
	CommitMessage     string            `json:"commit_message,omitempty"`
	IdempotencyKey    string            `json:"idempotency_key,omitempty"`
	SourcePatchID     string            `json:"source_patch_id,omitempty"`
	SourceCandidateID *string           `json:"source_candidate_id,omitempty"`
}
type Database interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Begin(context.Context) (pgx.Tx, error)
	Ping(context.Context) error
}

type App struct {
	db          Database
	redis       *redis.Client
	rabbit      *amqp.Connection
	channel     *amqp.Channel
	mu          sync.Mutex
	auth        *AuthConfig
	testMode    bool
	telemetry   []map[string]any
	sampleAt    time.Time
	publishFunc func(context.Context, string, any) error
}

func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Debug("response encode", "error", err)
	}
}
func fail(w http.ResponseWriter, status int, err error) {
	write(w, status, map[string]string{"error": err.Error()})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		fail(w, 400, err)
		return false
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		fail(w, 400, errors.New("exactly one JSON value required"))
		return false
	}
	return true
}
func (a *App) query(ctx context.Context, sql string, args ...any) ([]map[string]any, error) {
	rows, err := a.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result, err := pgx.CollectRows(rows, pgx.RowToMap)
	if err != nil {
		return nil, err
	}
	normalizeUUIDRows(result)
	return result, nil
}
func normalizeUUIDRows(result []map[string]any) {
	for _, row := range result {
		for key, value := range row {
			if id, ok := value.([16]byte); ok {
				raw := hex.EncodeToString(id[:])
				row[key] = raw[:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:]
			}
		}
	}
}
func (a *App) publish(ctx context.Context, key string, v any) (err error) {
	ctx, span := otel.Tracer("sentinel/gateway/messaging").Start(ctx, "publish "+key, trace.WithSpanKind(trace.SpanKindProducer), trace.WithAttributes(attribute.String("messaging.system", "rabbitmq"), attribute.String("messaging.destination.name", "sentinel"), attribute.String("messaging.rabbitmq.destination.routing_key", key)))
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "RabbitMQ publication failed")
		}
		span.End()
	}()
	if a.publishFunc != nil {
		return a.publishFunc(ctx, key, v)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	confirmation, err := a.channel.PublishWithDeferredConfirmWithContext(ctx, "sentinel", key, false, false, amqp.Publishing{DeliveryMode: amqp.Persistent, ContentType: "application/json", Body: b, Headers: rabbitTraceHeaders(ctx)})
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
func validID(s string) bool {
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	return err == nil && len(strings.ReplaceAll(s, "-", "")) == 32
}
func (a *App) routes() http.Handler {
	m := http.NewServeMux()
	a.extendedRoutes(m)
	m.HandleFunc("GET /api/v1/health", a.health)
	m.HandleFunc("GET /api/v1/repos", a.repos)
	m.HandleFunc("POST /api/v1/repos", a.register)
	m.HandleFunc("GET /api/v1/pipelines", a.pipelines)
	m.HandleFunc("POST /api/v1/pipelines/trigger", a.trigger)
	m.HandleFunc("GET /api/v1/pipelines/{id}", a.detail)
	m.HandleFunc("POST /api/v1/pipelines/{id}/simulate-failure", a.simulate)
	m.HandleFunc("GET /api/v1/patches", a.patches)
	m.HandleFunc("POST /api/v1/patches/{id}/apply", a.apply)
	m.HandleFunc("GET /api/v1/metrics/overview", a.metrics)
	m.HandleFunc("GET /ws/pipelines/{id}/logs", a.logs)
	return traceHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", env("CONSOLE_ORIGIN", "http://localhost:3000"))
		w.Header().Set("Access-Control-Expose-Headers", "X-Trace-ID")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Sentinel-Token, Idempotency-Key, traceparent, tracestate")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		if id := r.PathValue("id"); id != "" && !validID(id) {
			fail(w, 400, errors.New("invalid UUID"))
			return
		}
		a.authenticate(m).ServeHTTP(w, r)
	}))
}
func (a *App) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	checks := map[string]bool{"postgres": a.db.Ping(ctx) == nil, "redis": a.redis.Ping(ctx).Err() == nil, "rabbitmq": !a.rabbit.IsClosed() && !a.channel.IsClosed()}
	status := 200
	for _, ok := range checks {
		if !ok {
			status = 503
		}
	}
	write(w, status, map[string]any{"status": map[bool]string{true: "healthy", false: "degraded"}[status == 200], "checks": checks})
}
func (a *App) repos(w http.ResponseWriter, r *http.Request) {
	v, e := a.query(r.Context(), "SELECT r.id,r.name,r.repo_url,r.default_branch,r.created_at,r.auto_commit,r.open_pr,r.manual_approval,(r.webhook_secret<>'') AS webhook_configured,coalesce((SELECT 100.0*count(*) FILTER(WHERE p.status IN ('PASSED','HEALED'))/nullif(count(*),0) FROM pipelines p WHERE p.repo_id=r.id),0)::float8 AS reliability_score FROM repositories r ORDER BY r.created_at DESC")
	if e != nil {
		fail(w, 500, e)
		return
	}
	write(w, 200, v)
}
func (a *App) register(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name   string `json:"name"`
		URL    string `json:"repo_url"`
		Branch string `json:"default_branch"`
	}
	if !decode(w, r, &b) {
		return
	}
	u, e := url.Parse(b.URL)
	if e != nil || u.Scheme != "https" || u.Host == "" || strings.TrimSpace(b.Name) == "" {
		fail(w, 400, errors.New("name and HTTPS repo_url required"))
		return
	}
	if b.Branch == "" {
		b.Branch = "main"
	}
	id := uuid()
	tx, e := a.db.Begin(r.Context())
	if e != nil {
		fail(w, 503, e)
		return
	}
	defer tx.Rollback(r.Context())
	if _, e = tx.Exec(r.Context(), "INSERT INTO repositories(id,name,repo_url,default_branch) VALUES($1,$2,$3,$4)", id, b.Name, b.URL, b.Branch); e != nil {
		fail(w, 409, e)
		return
	}
	if e = auditTx(r.Context(), tx, "repository.register", "repository", id, map[string]any{"name": b.Name, "repo_url": b.URL}); e != nil {
		fail(w, 500, e)
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		fail(w, 503, e)
		return
	}
	write(w, 201, map[string]any{"id": id, "name": b.Name, "repo_url": b.URL, "default_branch": b.Branch})
}
func (a *App) pipelines(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	repo := r.URL.Query().Get("repo_id")
	v, e := a.query(r.Context(), "SELECT p.*, r.name AS repo_name, coalesce((SELECT jsonb_agg(to_jsonb(s) ORDER BY s.stage,s.name) FROM pipeline_steps s WHERE s.pipeline_id=p.id),'[]'::jsonb) AS steps FROM pipelines p JOIN repositories r ON r.id=p.repo_id WHERE ($1='' OR p.status=$1) AND ($2='' OR p.repo_id::text=$2) ORDER BY p.started_at DESC NULLS LAST LIMIT 200", status, repo)
	if e != nil {
		fail(w, 500, e)
		return
	}
	write(w, 200, v)
}
func (a *App) detail(w http.ResponseWriter, r *http.Request) {
	if !validID(r.PathValue("id")) {
		fail(w, 400, errors.New("invalid UUID"))
		return
	}
	id := r.PathValue("id")
	v, e := a.query(r.Context(), "SELECT p.*,r.name AS repo_name FROM pipelines p JOIN repositories r ON r.id=p.repo_id WHERE p.id=$1", id)
	if e != nil {
		fail(w, 500, e)
		return
	}
	if len(v) == 0 {
		fail(w, 404, errors.New("pipeline not found"))
		return
	}
	steps, e := a.query(r.Context(), "SELECT * FROM pipeline_steps WHERE pipeline_id=$1 ORDER BY stage,name", id)
	if e != nil {
		fail(w, 500, e)
		return
	}
	patches, e := a.query(r.Context(), "SELECT * FROM healing_patches WHERE pipeline_id=$1 ORDER BY created_at DESC", id)
	if e != nil {
		fail(w, 500, e)
		return
	}
	logs, e := a.query(r.Context(), "SELECT l.* FROM log_chunks l JOIN pipeline_steps s ON s.id=l.step_id WHERE s.pipeline_id=$1 ORDER BY l.id LIMIT 10000", id)
	if e != nil {
		fail(w, 500, e)
		return
	}
	v[0]["steps"] = steps
	v[0]["patches"] = patches
	v[0]["logs"] = logs
	write(w, 200, v[0])
}
func validateSteps(steps []Step) error {
	if len(steps) == 0 || len(steps) > 100 {
		return errors.New("between 1 and 100 steps required")
	}
	names := map[string]bool{}
	for _, s := range steps {
		if s.Name == "" || s.Command == "" || names[s.Name] {
			return errors.New("unique nonempty step names and commands required")
		}
		names[s.Name] = true
	}
	done := map[string]bool{}
	for len(done) < len(steps) {
		progress := false
		for _, s := range steps {
			if done[s.Name] {
				continue
			}
			ready := true
			for _, d := range s.DependsOn {
				if !names[d] {
					return fmt.Errorf("unknown dependency %s", d)
				}
				ready = ready && done[d]
			}
			if ready {
				done[s.Name] = true
				progress = true
			}
		}
		if !progress {
			return errors.New("DAG contains cycle")
		}
	}
	return nil
}
func (a *App) trigger(w http.ResponseWriter, r *http.Request) {
	var b struct {
		RepoID        string `json:"repo_id"`
		CommitSHA     string `json:"commit_sha"`
		Branch        string `json:"branch"`
		Steps         []Step `json:"steps"`
		CommitMessage string `json:"commit_message"`
	}
	if !decode(w, r, &b) {
		return
	}
	if !validID(b.RepoID) {
		fail(w, 400, errors.New("valid repo_id required"))
		return
	}
	if !validSHA(b.CommitSHA) {
		fail(w, 400, errors.New("valid hexadecimal commit_sha required"))
		return
	}
	if len(r.Header.Get("Idempotency-Key")) > 200 {
		fail(w, 400, errors.New("idempotency key too long"))
		return
	}
	if err := validateSteps(b.Steps); err != nil {
		fail(w, 400, err)
		return
	}
	var repo, branch string
	if err := a.db.QueryRow(r.Context(), "SELECT repo_url,default_branch FROM repositories WHERE id=$1", b.RepoID).Scan(&repo, &branch); err != nil {
		fail(w, 404, errors.New("repository not found"))
		return
	}
	if b.Branch == "" {
		b.Branch = branch
	}
	j := Job{PipelineID: uuid(), RepoURL: repo, CommitSHA: b.CommitSHA, Branch: b.Branch, Steps: b.Steps, CommitMessage: b.CommitMessage, IdempotencyKey: r.Header.Get("Idempotency-Key")}
	if err := a.createJob(r.Context(), b.RepoID, &j); err != nil {
		fail(w, 503, err)
		return
	}
	write(w, 202, map[string]string{"id": j.PipelineID, "status": "PENDING"})
}
func (a *App) patches(w http.ResponseWriter, r *http.Request) {
	v, e := a.query(r.Context(), "SELECT * FROM healing_patches WHERE ($1='' OR pipeline_id::text=$1) ORDER BY created_at DESC LIMIT 200", r.URL.Query().Get("pipeline_id"))
	if e != nil {
		fail(w, 500, e)
		return
	}
	write(w, 200, v)
}
func (a *App) apply(w http.ResponseWriter, r *http.Request) {
	if !validID(r.PathValue("id")) {
		fail(w, 400, errors.New("invalid UUID"))
		return
	}
	var pipeline, path, patched, status string
	var sourceCandidate string
	if err := a.db.QueryRow(r.Context(), "SELECT pipeline_id::text,file_path,patched_code,verification_status,coalesce(selected_candidate_id::text,'') FROM healing_patches WHERE id=$1", r.PathValue("id")).Scan(&pipeline, &path, &patched, &status, &sourceCandidate); err != nil {
		fail(w, 404, errors.New("patch not found"))
		return
	}
	if status != "VERIFIED" {
		fail(w, 409, errors.New("only verified patches may be applied"))
		return
	}
	var payload []byte
	var repo string
	if err := a.db.QueryRow(r.Context(), "SELECT j.payload,p.repo_id::text FROM job_snapshots j JOIN pipelines p ON p.id=j.pipeline_id WHERE j.pipeline_id=$1", pipeline).Scan(&payload, &repo); err != nil {
		fail(w, 409, errors.New("original execution snapshot unavailable; trigger a new repair run"))
		return
	}
	var j Job
	if err := json.Unmarshal(payload, &j); err != nil {
		fail(w, 500, err)
		return
	}
	if len(j.Files) == 0 {
		fail(w, 409, errors.New("source snapshot unavailable"))
		return
	}
	j.Files[path] = patched
	j.PipelineID = uuid()
	j.IdempotencyKey = "apply:" + r.PathValue("id")
	j.SourcePatchID = r.PathValue("id")
	j.SourceCandidateID = nil
	if sourceCandidate != "" {
		j.SourceCandidateID = &sourceCandidate
	}
	j.CommitMessage = "fix: apply verified Sentinel repair"
	j.RepoURL = ""
	if err := a.createJob(r.Context(), repo, &j); err != nil {
		if errors.Is(err, errPatchReview) {
			fail(w, 409, err)
			return
		}
		fail(w, 503, err)
		return
	}
	write(w, 202, map[string]string{"id": j.PipelineID, "pipeline_id": j.PipelineID, "status": "PENDING"})
}

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
	o := r.Header.Get("Origin")
	return o == "" || o == env("CONSOLE_ORIGIN", "http://localhost:3000")
}}

func (a *App) logs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		fail(w, 400, errors.New("invalid UUID"))
		return
	}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	sub := a.redis.Subscribe(ctx, "pipeline:"+id+":logs")
	defer sub.Close()
	if _, err = sub.Receive(ctx); err != nil {
		return
	}
	go func() {
		defer cancel()
		c.SetReadLimit(1024)
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}()
	tick := time.NewTicker(25 * time.Second)
	defer tick.Stop()
	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			c.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := c.WriteMessage(websocket.TextMessage, []byte(msg.Payload)); err != nil {
				return
			}
		case <-tick.C:
			c.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if c.WriteMessage(websocket.PingMessage, nil) != nil {
				return
			}
		}
	}
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	shutdownTracing, traceErr := setupTracing(ctx)
	if traceErr != nil {
		slog.Error("tracing setup", "error", traceErr)
		os.Exit(1)
	}
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if e := shutdownTracing(flushCtx); e != nil {
			slog.Error("tracing shutdown", "error", e)
		}
	}()
	auth, err := authConfigFromEnv()
	if err != nil {
		slog.Error("authentication configuration", "error", err)
		os.Exit(1)
	}
	db, err := pgxpool.New(ctx, env("DATABASE_URL", "postgres://sentinel:sentinel@localhost:5432/sentinel?sslmode=disable"))
	if err != nil {
		slog.Error("database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	ro, err := redis.ParseURL(env("REDIS_URL", "redis://localhost:6379/0"))
	if err != nil {
		slog.Error("redis URL", "error", err)
		os.Exit(1)
	}
	rc := redis.NewClient(ro)
	defer rc.Close()
	conn, err := amqp.Dial(env("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"))
	if err != nil {
		slog.Error("rabbitmq", "error", err)
		os.Exit(1)
	}
	defer conn.Close()
	go func() {
		select {
		case err := <-conn.NotifyClose(make(chan *amqp.Error, 1)):
			if err != nil {
				slog.Error("broker disconnected; restarting gateway", "error", err)
				stop()
			}
		case <-ctx.Done():
		}
	}()
	ch, err := conn.Channel()
	if err != nil {
		slog.Error("channel", "error", err)
		os.Exit(1)
	}
	defer ch.Close()
	go func() {
		select {
		case err := <-ch.NotifyClose(make(chan *amqp.Error, 1)):
			if err != nil {
				slog.Error("broker channel closed", "error", err)
				stop()
			}
		case <-ctx.Done():
		}
	}()
	if err = ch.ExchangeDeclare("sentinel", "topic", true, false, false, false, nil); err != nil {
		slog.Error("exchange", "error", err)
		os.Exit(1)
	}
	for _, name := range []string{"jobs.dispatch", "healer.trigger", "healer.result", "healer.dead"} {
		if _, err = ch.QueueDeclare(name, true, false, false, false, nil); err != nil {
			panic(err)
		}
		if err = ch.QueueBind(name, name, "sentinel", false, nil); err != nil {
			panic(err)
		}
	}
	if err = ch.QueueBind("healer.trigger", "git.publish", "sentinel", false, nil); err != nil {
		panic(err)
	}
	if _, err = ch.QueueDeclare("logs.stream", true, false, false, false, amqp.Table{"x-max-length": int32(10000), "x-message-ttl": int32(3600000)}); err != nil {
		panic(err)
	}
	if err = ch.QueueBind("logs.stream", "logs.stream", "sentinel", false, nil); err != nil {
		panic(err)
	}
	if err = ch.Confirm(false); err != nil {
		panic(err)
	}
	a := &App{db: db, redis: rc, rabbit: conn, channel: ch, auth: &auth}
	go a.relayOutbox(ctx)
	resultChannel, err := conn.Channel()
	if err != nil {
		panic(err)
	}
	defer resultChannel.Close()
	results, err := resultChannel.Consume("healer.result", "gateway-results", false, false, false, false, nil)
	if err != nil {
		panic(err)
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case delivery, ok := <-results:
				if !ok {
					return
				}
				a.handleHealerResult(ctx, delivery)
			}
		}
	}()
	server := &http.Server{Addr: env("LISTEN_ADDR", ":8080"), Handler: a.routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(c)
	}()
	slog.Info("gateway listening", "address", server.Addr)
	if err = server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server", "error", err)
		os.Exit(1)
	}
}
