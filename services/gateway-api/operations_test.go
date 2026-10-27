package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pashagolub/pgxmock/v4"
)

func TestAPIKeyCreatedOnlyReturnsRawOnce(t *testing.T) {
	a, db := mockApp(t)
	db.ExpectBegin()
	db.ExpectExec("INSERT INTO api_keys").WithArgs(pgxmock.AnyArg(), "build-bot", pgxmock.AnyArg(), "viewer").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectExec("INSERT INTO audit_logs").WithArgs(pgxmock.AnyArg(), "system", "api_key.create", "api_key", pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectCommit()
	w := httptest.NewRecorder()
	a.apiKeys(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"build-bot","role":"viewer"}`)))
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var result map[string]string
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || !strings.HasPrefix(result["token"], "sentinel_sk_") || !validID(result["id"]) {
		t.Fatal(result)
	}
	if e := db.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
func TestAPIKeyRevocationIsAudited(t *testing.T) {
	a, db := mockApp(t)
	id := uuid()
	db.ExpectBegin()
	db.ExpectExec("UPDATE api_keys SET revoked_at").WithArgs(id).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec("INSERT INTO audit_logs").WithArgs(pgxmock.AnyArg(), "system", "api_key.revoke", "api_key", id, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectCommit()
	w := httptest.NewRecorder()
	a.apiKeys(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"action":"revoke","id":"`+id+`"}`)))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if e := db.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
func TestRejectPatchKeepsVerificationEvidence(t *testing.T) {
	a, db := mockApp(t)
	id := uuid()
	db.ExpectBegin()
	db.ExpectQuery("SELECT verification_status,approval_status").WithArgs(id).WillReturnRows(pgxmock.NewRows([]string{"verification_status", "approval_status"}).AddRow("VERIFIED", "PENDING"))
	db.ExpectExec("UPDATE healing_patches SET approval_status").WithArgs(id, "REJECTED", "system").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec("INSERT INTO audit_logs").WithArgs(pgxmock.AnyArg(), "system", "patch.reject", "patch", id, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectCommit()
	if e := a.reviewPatch(context.Background(), id, "REJECTED"); e != nil {
		t.Fatal(e)
	}
	if e := db.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
func TestReviewCannotApproveRejectedPatch(t *testing.T) {
	a, db := mockApp(t)
	id := uuid()
	db.ExpectBegin()
	db.ExpectQuery("SELECT verification_status,approval_status").WithArgs(id).WillReturnRows(pgxmock.NewRows([]string{"verification_status", "approval_status"}).AddRow("VERIFIED", "REJECTED"))
	db.ExpectRollback()
	if e := a.reviewPatch(context.Background(), id, "APPROVED"); e == nil {
		t.Fatal("rejected patch approved")
	}
	if e := db.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
func TestPolicyValidationBeforeDatabase(t *testing.T) {
	a := &App{}
	for _, body := range []string{`{"default_branch":"-bad"}`, `{"default_branch":"bad branch"}`, `{"webhook_secret":"short"}`, `{"unknown":true}`} {
		r := httptest.NewRequest("PATCH", "/", strings.NewReader(body))
		r.SetPathValue("id", uuid())
		w := httptest.NewRecorder()
		a.updateRepo(w, r)
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
func TestRepositoryConfigurationAuditHidesSecret(t *testing.T) {
	a, db := mockApp(t)
	id := uuid()
	secret := "new-webhook-secret-value"
	db.ExpectBegin()
	db.ExpectQuery("SELECT auto_commit,open_pr,manual_approval").WithArgs(id).WillReturnRows(pgxmock.NewRows([]string{"auto_commit", "open_pr", "manual_approval"}).AddRow(false, false, true))
	db.ExpectExec("UPDATE repositories").WithArgs(id, pgxmock.AnyArg(), pgxmock.AnyArg(), true, true, true).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec("INSERT INTO audit_logs").WithArgs(pgxmock.AnyArg(), "system", "repository.configure", "repository", id, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectCommit()
	r := httptest.NewRequest("PATCH", "/", strings.NewReader(`{"webhook_secret":"`+secret+`","auto_commit":true,"open_pr":true}`))
	r.SetPathValue("id", id)
	w := httptest.NewRecorder()
	a.updateRepo(w, r)
	if w.Code != 200 || strings.Contains(w.Body.String(), secret) {
		t.Fatal(w.Code, w.Body.String())
	}
	if e := db.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
func TestPullRecordReadAndMissing(t *testing.T) {
	for _, found := range []bool{true, false} {
		a, db := mockApp(t)
		id := uuid()
		rows := pgxmock.NewRows([]string{"id", "branch"})
		if found {
			rows.AddRow(id, "sentinel/fix-nil-a8f92d1")
		}
		db.ExpectQuery("SELECT \\* FROM pull_requests WHERE id").WithArgs(id).WillReturnRows(rows)
		r := httptest.NewRequest("GET", "/", nil)
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()
		a.pullDetail(w, r)
		want := 404
		if found {
			want = 200
		}
		if w.Code != want {
			t.Fatal(w.Code)
		}
	}
}
func TestDecodeRejectsTrailingJSON(t *testing.T) {
	w := httptest.NewRecorder()
	var v map[string]any
	if decode(w, httptest.NewRequest("POST", "/", bytes.NewBufferString(`{} {}`)), &v) || w.Code != 400 {
		t.Fatal("accepted ambiguous JSON")
	}
}

func TestWorkerUsageDeduplicatesHostCounters(t *testing.T) {
	cpu, memory := aggregateWorkerUsage([]map[string]any{{"host": "runner1", "cpu_percent": 12.0, "memory_bytes": int64(1024)}, {"host": "runner1", "cpu_percent": 12.0, "memory_bytes": int64(1024)}, {"host": "runner2", "cpu_percent": 3.0, "memory_bytes": 512.0}})
	if cpu != 15 || memory != 1536 {
		t.Fatal(cpu, memory)
	}
}
