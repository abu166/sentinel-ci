package main

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"github.com/alicebob/miniredis/v2"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/redis/go-redis/v9"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSandboxConfigDefaults(t *testing.T) {
	t.Setenv("SANDBOX_MODE", "docker")
	s, e := sandboxConfig()
	if e != nil || s.Network != "none" || s.MemoryMB != 768 || !s.policy().Isolated {
		t.Fatal(s, e)
	}
}
func TestSandboxRejectsUnsafeProcess(t *testing.T) {
	t.Setenv("SANDBOX_MODE", "process")
	if _, e := sandboxConfig(); e == nil {
		t.Fatal("unsafe process permitted")
	}
}
func TestSandboxRejectsInvalidLimits(t *testing.T) {
	for _, tc := range []struct{ k, v string }{{"SANDBOX_MEMORY_MB", "0"}, {"SANDBOX_MEMORY_MB", "oops"}, {"SANDBOX_MEMORY_MB", "9000"}, {"SANDBOX_CPUS", "0"}, {"SANDBOX_CPUS", "oops"}, {"SANDBOX_CPUS", "99"}, {"SANDBOX_CPUS", "NaN"}, {"SANDBOX_CPUS", "Inf"}, {"SANDBOX_NETWORK", "host"}} {
		t.Run(tc.k+tc.v, func(t *testing.T) {
			t.Setenv(tc.k, tc.v)
			if _, e := sandboxConfig(); e == nil {
				t.Fatal("invalid limits accepted")
			}
		})
	}
}
func TestScrubbedSecrets(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://secret")
	t.Setenv("INTERNAL_TOKEN", "very-secret")
	for _, entry := range cleanEnvironment(t.TempDir()) {
		if strings.Contains(entry, "secret") || strings.HasPrefix(entry, "DATABASE_URL=") {
			t.Fatal(entry)
		}
	}
}
func TestEnvironmentDisablesDependencyNetwork(t *testing.T) {
	got := strings.Join(cleanEnvironment(t.TempDir()), " ")
	for _, v := range []string{"GOPROXY=off", "GOTOOLCHAIN=local", "GOMAXPROCS=2"} {
		if !strings.Contains(got, v) {
			t.Fatal(got)
		}
	}
}
func TestWorkspaceArchiveRoundTrip(t *testing.T) {
	root := t.TempDir()
	if e := safeWrite(root, "pkg/source.go", "package pkg\n"); e != nil {
		t.Fatal(e)
	}
	b, e := workspaceTar(root)
	if e != nil {
		t.Fatal(e)
	}
	tr := tar.NewReader(bytes.NewReader(b))
	h, e := tr.Next()
	if e != nil || h.Name != "pkg/source.go" || h.Mode != 0600 {
		t.Fatal(h, e)
	}
	content, _ := io.ReadAll(tr)
	if string(content) != "package pkg\n" {
		t.Fatal(string(content))
	}
}
func TestWorkspaceRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	if e := os.Symlink("/etc/passwd", filepath.Join(root, "outside")); e != nil {
		t.Fatal(e)
	}
	if _, e := workspaceTar(root); e == nil {
		t.Fatal("symlink accepted")
	}
}
func TestWorkspaceRejectsOversize(t *testing.T) {
	root := t.TempDir()
	f, e := os.Create(filepath.Join(root, "large"))
	if e != nil {
		t.Fatal(e)
	}
	if e = f.Truncate((64 << 20) + 1); e != nil {
		t.Fatal(e)
	}
	f.Close()
	if _, e = workspaceTar(root); e == nil {
		t.Fatal("oversize archive accepted")
	}
}
func TestWorkspaceExcludesGitAndCaches(t *testing.T) {
	root := t.TempDir()
	safeWrite(root, ".git/config", "credential")
	safeWrite(root, ".gocache/cache", "build")
	b, e := workspaceTar(root)
	if e != nil || bytes.Contains(b, []byte("credential")) {
		t.Fatal(e)
	}
}
func TestVolumeNamesStableAndDistinct(t *testing.T) {
	if volumeName("a") != volumeName("a") || volumeName("a") == volumeName("b") || len(volumeName("a")) > 64 {
		t.Fatal("bad volume identity")
	}
}
func policyArgs() []string {
	return (SandboxExecutor{Mode: "docker", Image: "sandbox:test", MemoryMB: 768, CPUs: 1, Pids: 128, Network: "none"}).dockerArgs("/tmp/work", "safe-name", "go test ./...")
}
func argsContain(args []string, key, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key && args[i+1] == value {
			return true
		}
	}
	return false
}
func TestDockerReadOnlyRoot(t *testing.T) {
	if !strings.Contains(strings.Join(policyArgs(), " "), "--read-only") {
		t.Fatal(policyArgs())
	}
}
func TestDockerNetworkIsolated(t *testing.T) {
	if !argsContain(policyArgs(), "--network", "none") {
		t.Fatal(policyArgs())
	}
}
func TestDockerResourceQuotas(t *testing.T) {
	for k, v := range map[string]string{"--memory": "768m", "--memory-swap": "768m", "--cpus": "1.00", "--pids-limit": "128"} {
		if !argsContain(policyArgs(), k, v) {
			t.Fatal(k)
		}
	}
}
func TestDockerNoCapabilitiesNonroot(t *testing.T) {
	if !argsContain(policyArgs(), "--cap-drop", "ALL") || !argsContain(policyArgs(), "--user", "65532:65532") || !argsContain(policyArgs(), "--security-opt", "no-new-privileges:true") {
		t.Fatal(policyArgs())
	}
}
func TestDockerNoControllerMountOrCredentials(t *testing.T) {
	args := strings.Join(policyArgs(), " ")
	for _, bad := range []string{"docker.sock", "DATABASE_URL", "INTERNAL_TOKEN", "--privileged", "--network host"} {
		if strings.Contains(args, bad) {
			t.Fatal(args)
		}
	}
}
func TestResourceMeasurementUnits(t *testing.T) {
	r, ok := resourceLine("__SENTINEL_RESOURCE__ 1024 1.25 0.5")
	if !ok || r.PeakRSSBytes != 1048576 || r.CPUUserMS != 1250 || r.CPUSystemMS != 500 {
		t.Fatal(r)
	}
}
func TestMalformedResourceLines(t *testing.T) {
	for _, s := range []string{"", "other 1 2 3", "__SENTINEL_RESOURCE__ x 2 3", "__SENTINEL_RESOURCE__ -1 2 3", "__SENTINEL_RESOURCE__ 1 -2 3", "__SENTINEL_RESOURCE__ 1 2 x", "__SENTINEL_RESOURCE__ 1 NaN 1", "__SENTINEL_RESOURCE__ 1 2 Inf", "__SENTINEL_RESOURCE__ 9223372036854775807 2 3", "__SENTINEL_RESOURCE__ 1 1e300 1"} {
		if _, ok := resourceLine(s); ok {
			t.Fatal(s)
		}
	}
}
func TestProcessMetricsCollected(t *testing.T) {
	code, usage, e := runProcess(context.Background(), t.TempDir(), "echo hello", nil, func(string, string) {})
	if e != nil || code != 0 || usage.PeakRSSBytes <= 0 {
		t.Fatal(code, usage, e)
	}
}
func TestProcessCannotReadServiceEnvironment(t *testing.T) {
	t.Setenv("INTERNAL_TOKEN", "should-not-leak")
	var got string
	var lock sync.Mutex
	s := SandboxExecutor{Mode: "process"}
	code, usage, e := s.Execute(context.Background(), t.TempDir(), "printf '%s' \"${INTERNAL_TOKEN-unset}\"", func(_, line string) { lock.Lock(); got += line; lock.Unlock() })
	if e != nil || code != 0 || got != "unset" || usage.Policy.Isolated {
		t.Fatal(code, got, e)
	}
}
func TestUnknownSandboxBackendFailsClosed(t *testing.T) {
	code, _, e := (SandboxExecutor{Mode: "unknown"}).Execute(context.Background(), t.TempDir(), "true", func(string, string) {})
	if code != -1 || e == nil {
		t.Fatal(code, e)
	}
}
func TestMissingClaimRuns(t *testing.T) {
	if decideExecution("", false) != decisionRun {
		t.Fatal("new execution denied")
	}
}
func TestCompletedClaimSkipped(t *testing.T) {
	if decideExecution("COMPLETED", true) != decisionSkip {
		t.Fatal("duplicate would execute")
	}
}
func TestAmbiguousClaimNeverReexecutes(t *testing.T) {
	for _, s := range []string{"RUNNING", "FAILED", "INTERRUPTED", "unknown"} {
		if decideExecution(s, true) != decisionInterrupted {
			t.Fatal(s)
		}
	}
}
func TestTerminalPipelinesNotReplayed(t *testing.T) {
	for _, s := range []string{"PASSED", "FAILED", "HEALED", "HEALING"} {
		if !pipelineTerminal(s) {
			t.Fatal(s)
		}
	}
	if pipelineTerminal("PENDING") || pipelineTerminal("RUNNING") {
		t.Fatal("active treated terminal")
	}
}
func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	server := miniredis.RunT(t)
	c := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { c.Close() })
	return c
}
func TestLeaseExclusive(t *testing.T) {
	c := testRedis(t)
	l, ok, e := acquireLease(context.Background(), c, "job")
	if e != nil || !ok {
		t.Fatal(e)
	}
	defer l.Close()
	other, ok, e := acquireLease(context.Background(), c, "job")
	if e != nil || ok || other != nil {
		t.Fatal("duplicate lease", e)
	}
}
func TestLeaseRelease(t *testing.T) {
	c := testRedis(t)
	l, _, _ := acquireLease(context.Background(), c, "job")
	l.Close()
	if c.Exists(context.Background(), "job").Val() != 0 {
		t.Fatal("lease retained")
	}
}
func TestLeaseCannotDeleteNewOwner(t *testing.T) {
	c := testRedis(t)
	l, _, _ := acquireLease(context.Background(), c, "job")
	c.Set(context.Background(), "job", "new-owner", time.Minute)
	l.Close()
	if c.Get(context.Background(), "job").Val() != "new-owner" {
		t.Fatal("deleted another owner")
	}
}
func TestLeaseContextCancelled(t *testing.T) {
	c := testRedis(t)
	ctx, cancel := context.WithCancel(context.Background())
	l, _, _ := acquireLease(ctx, c, "job")
	cancel()
	select {
	case <-l.Context.Done():
	case <-time.After(time.Second):
		t.Fatal("lease did not cancel")
	}
	l.Close()
}
func TestClaimTransaction(t *testing.T) {
	db, e := pgxmock.NewPool()
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	db.ExpectBegin()
	db.ExpectQuery("SELECT status FROM step_executions").WithArgs("step").WillReturnRows(pgxmock.NewRows([]string{"status"}))
	db.ExpectExec("INSERT INTO step_executions").WithArgs("step", "pipeline", "owner").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectCommit()
	r := Runner{db: db}
	d, e := r.claimStep(context.Background(), "pipeline", "step", "owner")
	if e != nil || d != decisionRun {
		t.Fatal(d, e)
	}
	if e = db.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
func TestClaimCompletedIsReadOnly(t *testing.T) {
	db, _ := pgxmock.NewPool()
	defer db.Close()
	db.ExpectBegin()
	db.ExpectQuery("SELECT status FROM step_executions").WithArgs("step").WillReturnRows(pgxmock.NewRows([]string{"status"}).AddRow("COMPLETED"))
	db.ExpectCommit()
	d, e := (&Runner{db: db}).claimStep(context.Background(), "pipeline", "step", "owner")
	if e != nil || d != decisionSkip {
		t.Fatal(d, e)
	}
	if e = db.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
func TestClaimInterruptedPersists(t *testing.T) {
	db, _ := pgxmock.NewPool()
	defer db.Close()
	db.ExpectBegin()
	db.ExpectQuery("SELECT status FROM step_executions").WithArgs("step").WillReturnRows(pgxmock.NewRows([]string{"status"}).AddRow("RUNNING"))
	db.ExpectExec("UPDATE step_executions").WithArgs("step").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()
	d, e := (&Runner{db: db}).claimStep(context.Background(), "pipeline", "step", "owner")
	if e != nil || d != decisionInterrupted {
		t.Fatal(d, e)
	}
}
func TestClaimDatabaseFailureRollsBack(t *testing.T) {
	db, _ := pgxmock.NewPool()
	defer db.Close()
	db.ExpectBegin()
	db.ExpectQuery("SELECT status FROM step_executions").WithArgs("step").WillReturnError(errors.New("unavailable"))
	db.ExpectRollback()
	if _, e := (&Runner{db: db}).claimStep(context.Background(), "pipeline", "step", "owner"); e == nil {
		t.Fatal("error ignored")
	}
}
func TestHealingOutboxAtomic(t *testing.T) {
	db, _ := pgxmock.NewPool()
	defer db.Close()
	db.ExpectBegin()
	db.ExpectExec("UPDATE pipelines SET status='HEALING'").WithArgs("pipeline").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec("INSERT INTO outbox_events").WithArgs("pipeline", pgxmock.AnyArg(), "healer.trigger:step", pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectCommit()
	e := (&Runner{db: db}).enqueueHealing(context.Background(), Job{PipelineID: "pipeline"}, Step{ID: "step", Command: "test"}, "failed", map[string]string{"a.py": "x"}, "python")
	if e != nil {
		t.Fatal(e)
	}
	if e = db.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
func TestHealingOutboxRollback(t *testing.T) {
	db, _ := pgxmock.NewPool()
	defer db.Close()
	db.ExpectBegin()
	db.ExpectExec("UPDATE pipelines SET status='HEALING'").WithArgs("pipeline").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec("INSERT INTO outbox_events").WithArgs("pipeline", pgxmock.AnyArg(), "healer.trigger:step", pgxmock.AnyArg()).WillReturnError(errors.New("disk full"))
	db.ExpectRollback()
	e := (&Runner{db: db}).enqueueHealing(context.Background(), Job{PipelineID: "pipeline"}, Step{ID: "step"}, "failed", nil, "python")
	if e == nil {
		t.Fatal("uncommitted transition accepted")
	}
}
