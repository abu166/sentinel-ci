package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

func (a *App) extendedRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/auth/login", a.login)
	m.HandleFunc("POST /api/v1/auth/keys", a.apiKeys)
	m.HandleFunc("GET /api/v1/audit-logs", a.auditLogs)
	m.HandleFunc("POST /api/v1/webhooks/github", a.webhook)
	m.HandleFunc("PATCH /api/v1/repos/{id}", a.updateRepo)
	m.HandleFunc("GET /api/v1/workers", a.workers)
	m.HandleFunc("GET /api/v1/queue", a.queue)
	m.HandleFunc("GET /api/v1/pulls", a.pulls)
	m.HandleFunc("GET /api/v1/pulls/{id}", a.pullDetail)
	m.HandleFunc("POST /api/v1/patches/{id}/publish", a.publishPatch)
	m.HandleFunc("POST /api/v1/patches/{id}/reject", a.rejectPatch)
	m.HandleFunc("POST /api/v1/patches/{id}/verify", a.verifyPatch)
	m.HandleFunc("GET /api/v1/simulations", simulationCatalog)
	m.HandleFunc("GET /api/v1/patches/{id}/candidates", a.candidateList)
	m.HandleFunc("POST /api/v1/patches/{id}/select-candidate", a.selectCandidate)
}
func (a *App) auditLogs(w http.ResponseWriter, r *http.Request) {
	v, e := a.query(r.Context(), "SELECT * FROM audit_logs ORDER BY created_at DESC LIMIT 500")
	if e != nil {
		fail(w, 500, e)
		return
	}
	write(w, 200, v)
}
func (a *App) updateRepo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		fail(w, 400, errors.New("invalid UUID"))
		return
	}
	var b struct {
		DefaultBranch  *string `json:"default_branch"`
		WebhookSecret  *string `json:"webhook_secret"`
		AutoCommit     *bool   `json:"auto_commit"`
		OpenPR         *bool   `json:"open_pr"`
		ManualApproval *bool   `json:"manual_approval"`
	}
	if !decode(w, r, &b) {
		return
	}
	if b.DefaultBranch != nil && (strings.TrimSpace(*b.DefaultBranch) == "" || strings.HasPrefix(*b.DefaultBranch, "-") || strings.ContainsAny(*b.DefaultBranch, " \n\r\x00")) {
		fail(w, 400, errors.New("invalid default branch"))
		return
	}
	if b.WebhookSecret != nil && len(*b.WebhookSecret) < 16 {
		fail(w, 400, errors.New("webhook secret must contain at least 16 characters"))
		return
	}
	tx, e := a.db.Begin(r.Context())
	if e != nil {
		fail(w, 503, e)
		return
	}
	defer tx.Rollback(r.Context())
	var auto, open, manual bool
	if e = tx.QueryRow(r.Context(), "SELECT auto_commit,open_pr,manual_approval FROM repositories WHERE id=$1 FOR UPDATE", id).Scan(&auto, &open, &manual); e != nil {
		fail(w, 404, errors.New("repository not found"))
		return
	}
	if b.AutoCommit != nil {
		auto = *b.AutoCommit
	}
	if b.OpenPR != nil {
		open = *b.OpenPR
	}
	if b.ManualApproval != nil {
		manual = *b.ManualApproval
	}
	if e = validateRepoPolicy(auto, open, manual); e != nil {
		fail(w, 400, e)
		return
	}
	if _, e = tx.Exec(r.Context(), "UPDATE repositories SET default_branch=coalesce($2,default_branch),webhook_secret=coalesce($3,webhook_secret),auto_commit=$4,open_pr=$5,manual_approval=$6 WHERE id=$1", id, b.DefaultBranch, b.WebhookSecret, auto, open, manual); e != nil {
		fail(w, 500, e)
		return
	}
	if e = auditTx(r.Context(), tx, "repository.configure", "repository", id, map[string]any{"auto_commit": auto, "open_pr": open, "manual_approval": manual, "webhook_secret_changed": b.WebhookSecret != nil, "default_branch": b.DefaultBranch}); e != nil {
		fail(w, 500, e)
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		fail(w, 503, e)
		return
	}
	write(w, 200, map[string]any{"id": id, "auto_commit": auto, "open_pr": open, "manual_approval": manual})
}
func (a *App) workerRecords(ctx context.Context) ([]map[string]any, error) {
	records := []map[string]any{}
	iter := a.redis.Scan(ctx, 0, "workerinfo:*", 100).Iterator()
	for iter.Next(ctx) {
		b, e := a.redis.Get(ctx, iter.Val()).Bytes()
		if e != nil {
			continue
		}
		var v map[string]any
		if json.Unmarshal(b, &v) != nil {
			continue
		}
		v["metrics_scope"] = "controller_host"
		records = append(records, v)
	}
	return records, iter.Err()
}
func (a *App) workers(w http.ResponseWriter, r *http.Request) {
	v, e := a.workerRecords(r.Context())
	if e != nil {
		fail(w, 503, e)
		return
	}
	write(w, 200, v)
}
func (a *App) queue(w http.ResponseWriter, r *http.Request) {
	queues := []map[string]any{}
	for _, name := range []string{"jobs.dispatch", "logs.stream", "healer.trigger", "healer.result", "healer.dead"} {
		q, e := a.queueInfo(name)
		if e != nil {
			fail(w, 503, e)
			return
		}
		queues = append(queues, map[string]any{"name": name, "messages": q[0], "consumers": q[1]})
	}
	rows, e := a.query(r.Context(), "SELECT count(*) FILTER(WHERE status='PENDING') AS pending,count(*) FILTER(WHERE status='PUBLISHED') AS published,count(*) FILTER(WHERE status='DEAD') AS dead FROM outbox_events")
	if e != nil {
		fail(w, 500, e)
		return
	}
	events, e := a.query(r.Context(), "SELECT id,aggregate_id,event_type,status,attempts,next_retry_at,last_error,created_at FROM outbox_events ORDER BY created_at DESC LIMIT 100")
	if e != nil {
		fail(w, 500, e)
		return
	}
	write(w, 200, map[string]any{"queues": queues, "outbox": rows[0], "events": events})
}
func (a *App) queueInfo(name string) ([2]int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.channel == nil {
		return [2]int{}, errors.New("broker channel unavailable")
	}
	q, e := a.channel.QueueInspect(name)
	return [2]int{q.Messages, q.Consumers}, e
}
func (a *App) pulls(w http.ResponseWriter, r *http.Request) {
	v, e := a.query(r.Context(), "SELECT * FROM pull_requests ORDER BY created_at DESC LIMIT 200")
	if e != nil {
		fail(w, 500, e)
		return
	}
	write(w, 200, v)
}
func (a *App) pullDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		fail(w, 400, errors.New("invalid UUID"))
		return
	}
	v, e := a.query(r.Context(), "SELECT * FROM pull_requests WHERE id=$1", id)
	if e != nil {
		fail(w, 500, e)
		return
	}
	if len(v) == 0 {
		fail(w, 404, errors.New("pull request not found"))
		return
	}
	write(w, 200, v[0])
}
func (a *App) reviewPatch(ctx context.Context, id, status string) error {
	tx, e := a.db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var verification, approval string
	if e = tx.QueryRow(ctx, "SELECT verification_status,approval_status FROM healing_patches WHERE id=$1 FOR UPDATE", id).Scan(&verification, &approval); e != nil {
		return e
	}
	if status == "APPROVED" && (verification != "VERIFIED" || approval == "REJECTED") {
		return errors.New("only verified patches can be approved")
	}
	if _, e = tx.Exec(ctx, "UPDATE healing_patches SET approval_status=$2,approved_by=$3,approved_at=now() WHERE id=$1", id, status, actor(ctx).Username); e != nil {
		return e
	}
	action := "patch.approve"
	if status == "REJECTED" {
		action = "patch.reject"
	}
	if e = auditTx(ctx, tx, action, "patch", id, map[string]any{}); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (a *App) rejectPatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		fail(w, 400, errors.New("invalid UUID"))
		return
	}
	if e := a.reviewPatch(r.Context(), id, "REJECTED"); e != nil {
		fail(w, 409, e)
		return
	}
	write(w, 200, map[string]string{"id": id, "approval_status": "REJECTED"})
}
func (a *App) publishPatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		fail(w, 400, errors.New("invalid UUID"))
		return
	}
	var b struct {
		OpenPR   *bool  `json:"open_pr"`
		Provider string `json:"provider"`
	}
	if r.ContentLength != 0 && !decode(w, r, &b) {
		return
	}
	if b.Provider != "" && b.Provider != "local" && b.Provider != "github" && b.Provider != "gitlab" {
		fail(w, 400, errors.New("unsupported Git provider"))
		return
	}
	if e := a.reviewPatch(r.Context(), id, "APPROVED"); e != nil {
		fail(w, 409, e)
		return
	}
	bodyMap := map[string]any{"patch_id": id, "approved": true}
	if b.OpenPR != nil {
		bodyMap["open_pr"] = *b.OpenPR
	}
	if b.Provider != "" {
		bodyMap["provider"] = b.Provider
	}
	payload, _ := json.Marshal(bodyMap)
	req, e := http.NewRequestWithContext(r.Context(), "POST", env("HEALER_URL", "http://healer-service:8081")+"/api/v1/publish", bytes.NewReader(payload))
	if e != nil {
		fail(w, 500, e)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Sentinel-Internal-Token", env("INTERNAL_TOKEN", ""))
	client := &http.Client{Timeout: 90 * time.Second, Transport: traceTransport{}}
	resp, e := client.Do(req)
	if e != nil {
		fail(w, 502, errors.New("Git publisher unavailable"))
		return
	}
	defer resp.Body.Close()
	body, e := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if e != nil {
		fail(w, 502, e)
		return
	}
	if resp.StatusCode >= 400 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
		return
	}
	var result any
	if json.Unmarshal(body, &result) != nil {
		fail(w, 502, errors.New("invalid publisher response"))
		return
	}
	write(w, 202, result)
}
func (a *App) verifyPatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		fail(w, 400, errors.New("invalid UUID"))
		return
	}
	var b struct {
		Race bool `json:"race"`
	}
	if r.ContentLength != 0 && !decode(w, r, &b) {
		return
	}
	var pipeline, path, patched, status string
	if e := a.db.QueryRow(r.Context(), "SELECT pipeline_id::text,file_path,patched_code,verification_status FROM healing_patches WHERE id=$1", id).Scan(&pipeline, &path, &patched, &status); e != nil {
		fail(w, 404, errors.New("patch not found"))
		return
	}
	if status != "VERIFIED" {
		fail(w, 409, errors.New("verified patch required"))
		return
	}
	var snapshot []byte
	var repo string
	if e := a.db.QueryRow(r.Context(), "SELECT j.payload,p.repo_id::text FROM job_snapshots j JOIN pipelines p ON p.id=j.pipeline_id WHERE j.pipeline_id=$1", pipeline).Scan(&snapshot, &repo); e != nil {
		fail(w, 409, errors.New("source snapshot unavailable"))
		return
	}
	var j Job
	if e := json.Unmarshal(snapshot, &j); e != nil {
		fail(w, 500, e)
		return
	}
	if j.Files == nil {
		fail(w, 409, errors.New("source snapshot unavailable"))
		return
	}
	if b.Race && !strings.HasSuffix(path, ".go") {
		fail(w, 400, errors.New("race detector requires a Go patch"))
		return
	}
	j.Files[path] = patched
	j.PipelineID = uuid()
	j.SourcePatchID = ""
	j.SourceCandidateID = nil
	j.IdempotencyKey = "verify:" + id + ":" + j.PipelineID
	j.RepoURL = ""
	j.CommitMessage = "test: verify reviewed Sentinel repair"
	if b.Race {
		j.Steps = []Step{{Name: "Race Detector Verification", Stage: "verify", Command: "go test -race ./...", DependsOn: []string{}}}
	}
	if e := a.createJob(r.Context(), repo, &j); e != nil {
		fail(w, 503, e)
		return
	}
	write(w, 202, map[string]string{"id": j.PipelineID, "pipeline_id": j.PipelineID, "status": "PENDING"})
}
