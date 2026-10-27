package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

func expectJobWrites(db pgxmock.PgxPoolIface, j Job, repo string, outboxRows int) {
	db.ExpectBegin()
	db.ExpectExec("INSERT INTO pipelines").WithArgs(j.PipelineID, repo, j.CommitSHA, j.Branch, "system", j.CommitMessage).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	for range j.Steps {
		db.ExpectExec("INSERT INTO pipeline_steps").WithArgs(pgxmock.AnyArg(), j.PipelineID, pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	}
	db.ExpectExec("INSERT INTO job_snapshots").WithArgs(j.PipelineID, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectExec("INSERT INTO outbox_events").WithArgs(pgxmock.AnyArg(), j.PipelineID, pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", int64(outboxRows)))
}
func jobFixture() Job {
	return Job{PipelineID: uuid(), CommitSHA: "a8f92d1", Branch: "main", CommitMessage: "fix: test repair", Steps: []Step{{Name: "Unit Tests", Stage: "test", Command: "go test ./..."}}}
}
func TestCreateJobAtomicOutboxAndAudit(t *testing.T) {
	a, db := mockApp(t)
	j := jobFixture()
	repo := uuid()
	expectJobWrites(db, j, repo, 1)
	db.ExpectExec("INSERT INTO audit_logs").WithArgs(pgxmock.AnyArg(), "system", "pipeline.trigger", "pipeline", j.PipelineID, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectCommit()
	if e := a.createJob(context.Background(), repo, &j); e != nil {
		t.Fatal(e)
	}
	if j.Steps[0].ID == "" || j.IdempotencyKey != "pipeline:"+j.PipelineID {
		t.Fatal(j)
	}
	if e := db.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
func TestCreateJobRollbackOnDatabaseFailure(t *testing.T) {
	a, db := mockApp(t)
	j := jobFixture()
	db.ExpectBegin()
	db.ExpectExec("INSERT INTO pipelines").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnError(errors.New("db unavailable"))
	db.ExpectRollback()
	if e := a.createJob(context.Background(), uuid(), &j); e == nil {
		t.Fatal("failed tx accepted")
	}
	if e := db.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
func TestCreateJobIdempotentConflictReturnsOriginal(t *testing.T) {
	a, db := mockApp(t)
	j := jobFixture()
	j.IdempotencyKey = "request-repeat"
	repo := uuid()
	original := uuid()
	expectJobWrites(db, j, repo, 0)
	db.ExpectRollback()
	db.ExpectQuery("SELECT aggregate_id::text FROM outbox_events").WithArgs(j.IdempotencyKey).WillReturnRows(pgxmock.NewRows([]string{"aggregate_id"}).AddRow(original))
	if e := a.createJob(context.Background(), repo, &j); e != nil {
		t.Fatal(e)
	}
	if j.PipelineID != original {
		t.Fatal(j.PipelineID)
	}
	if e := db.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
func TestOutboxNoPendingReturnsIdle(t *testing.T) {
	a, db := mockApp(t)
	db.ExpectBegin()
	db.ExpectQuery("SELECT id::text,event_type,payload,attempts").WillReturnError(pgx.ErrNoRows)
	db.ExpectRollback()
	worked, e := a.relayOne(context.Background())
	if e != nil || worked {
		t.Fatal(worked, e)
	}
	if e := db.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
func TestOutboxPublishesBeforeCommit(t *testing.T) {
	a, db := mockApp(t)
	id := uuid()
	published := false
	a.publishFunc = func(ctx context.Context, key string, v any) error {
		if key != "jobs.dispatch" || v.(map[string]any)["pipeline_id"] != "job-id" {
			t.Fatal(key, v)
		}
		published = true
		return nil
	}
	db.ExpectBegin()
	db.ExpectQuery("SELECT id::text,event_type,payload,attempts").WillReturnRows(pgxmock.NewRows([]string{"id", "kind", "payload", "attempts", "headers"}).AddRow(id, "jobs.dispatch", []byte(`{"pipeline_id":"job-id"}`), 0, []byte(`{}`)))
	db.ExpectExec("UPDATE outbox_events SET status='PUBLISHED'").WithArgs(id, 1).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()
	worked, e := a.relayOne(context.Background())
	if e != nil || !worked || !published {
		t.Fatal(worked, published, e)
	}
	if e := db.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
func TestOutboxFailureBackoffAndDeadLetter(t *testing.T) {
	for _, attempts := range []int{0, 7} {
		a, db := mockApp(t)
		id := uuid()
		a.publishFunc = func(context.Context, string, any) error { return errors.New("broker unavailable") }
		db.ExpectBegin()
		db.ExpectQuery("SELECT id::text,event_type,payload,attempts").WillReturnRows(pgxmock.NewRows([]string{"id", "kind", "payload", "attempts", "headers"}).AddRow(id, "jobs.dispatch", []byte(`{}`), attempts, []byte(`{}`)))
		status := "PENDING"
		if attempts == 7 {
			status = "DEAD"
		}
		db.ExpectExec("UPDATE outbox_events SET status=\\$2").WithArgs(id, status, attempts+1, pgxmock.AnyArg(), "broker unavailable").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		db.ExpectCommit()
		worked, e := a.relayOne(context.Background())
		if e != nil || !worked {
			t.Fatal(worked, e)
		}
		if e := db.ExpectationsWereMet(); e != nil {
			t.Fatal(e)
		}
	}
}
func TestOutboxMalformedPayloadIsRetried(t *testing.T) {
	a, db := mockApp(t)
	id := uuid()
	db.ExpectBegin()
	db.ExpectQuery("SELECT id::text,event_type,payload,attempts").WillReturnRows(pgxmock.NewRows([]string{"id", "kind", "payload", "attempts", "headers"}).AddRow(id, "jobs.dispatch", []byte(`{`), 0, []byte(`{}`)))
	db.ExpectExec("UPDATE outbox_events SET status=\\$2").WithArgs(id, "PENDING", 1, pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()
	worked, e := a.relayOne(context.Background())
	if e != nil || !worked {
		t.Fatal(worked, e)
	}
}
func TestRetryDelayIsBoundedExponential(t *testing.T) {
	if retryDelay(1) != time.Second || retryDelay(2) != 2*time.Second || retryDelay(0) != time.Second || retryDelay(100) != 512*time.Second {
		t.Fatal("incorrect backoff")
	}
}
func TestRepositoryPolicyInvariant(t *testing.T) {
	for _, c := range []struct{ auto, open, manual, valid bool }{{false, false, true, true}, {true, true, true, true}, {true, true, false, true}, {false, true, true, false}, {false, false, false, false}} {
		if (validateRepoPolicy(c.auto, c.open, c.manual) == nil) != c.valid {
			t.Fatal(c)
		}
	}
}
func TestTruncateErrorContext(t *testing.T) {
	if truncate("abc", 2) != "ab" || truncate("a", 2) != "a" || len(truncate(strings.Repeat("x", 3000), 2000)) != 2000 {
		t.Fatal("incorrect truncate")
	}
}

func TestApprovedPatchQueuesGitPublicationAtomically(t *testing.T) {
	a, db := mockApp(t)
	j := jobFixture()
	j.SourcePatchID = uuid()
	repo := uuid()
	expectJobWrites(db, j, repo, 1)
	db.ExpectExec("UPDATE healing_patches SET approval_status").WithArgs(j.SourcePatchID, "system", pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec("INSERT INTO audit_logs").WithArgs(pgxmock.AnyArg(), "system", "patch.approve", "patch", j.SourcePatchID, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectQuery("SELECT open_pr FROM repositories").WithArgs(repo).WillReturnRows(pgxmock.NewRows([]string{"open_pr"}).AddRow(false))
	db.ExpectExec("INSERT INTO outbox_events").WithArgs(pgxmock.AnyArg(), j.SourcePatchID, pgxmock.AnyArg(), "git.publish:"+j.SourcePatchID, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectExec("INSERT INTO audit_logs").WithArgs(pgxmock.AnyArg(), "system", "pipeline.trigger", "pipeline", j.PipelineID, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectCommit()
	if e := a.createJob(context.Background(), repo, &j); e != nil {
		t.Fatal(e)
	}
	if e := db.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
