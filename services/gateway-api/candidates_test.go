package main

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

func TestCandidateListContainsRankedEvidence(t *testing.T) {
	a, db := mockApp(t)
	patchID, candidateID := uuid(), uuid()
	db.ExpectQuery("SELECT coalesce").WithArgs(patchID).WillReturnRows(pgxmock.NewRows([]string{"selected_candidate_id"}).AddRow(candidateID))
	db.ExpectQuery("SELECT c.\\*,c.status AS verification_status").WithArgs(patchID).WillReturnRows(pgxmock.NewRows([]string{"id", "rank", "status", "score", "ast_cost", "selected"}).AddRow(candidateID, 1, "VERIFIED", 0.95, 3, true))
	r := httptest.NewRequest("GET", "/", nil)
	r.SetPathValue("id", patchID)
	w := httptest.NewRecorder()
	a.candidateList(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), candidateID) || !strings.Contains(w.Body.String(), `"rank":1`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if e := db.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
func TestCandidateListMissingPatch(t *testing.T) {
	a, db := mockApp(t)
	id := uuid()
	db.ExpectQuery("SELECT coalesce").WithArgs(id).WillReturnError(pgx.ErrNoRows)
	r := httptest.NewRequest("GET", "/", nil)
	r.SetPathValue("id", id)
	w := httptest.NewRecorder()
	a.candidateList(w, r)
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}
func TestCandidateSelectionViewerIsForbidden(t *testing.T) {
	a := &App{auth: testAuth()}
	token, _ := signJWT(*a.auth, Principal{"reader", "viewer"}, time.Now())
	r := httptest.NewRequest("POST", "/api/v1/patches/"+uuid()+"/select-candidate", strings.NewReader(`{"candidate_id":"`+uuid()+`"}`))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestCandidateSelectionValidatesIDs(t *testing.T) {
	for _, body := range []string{`{}`, `{"candidate_id":"bad"}`, `{} {}`} {
		a := &App{}
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		r.SetPathValue("id", uuid())
		w := httptest.NewRecorder()
		a.selectCandidate(w, r)
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
func TestCandidateSelectionLocksReviewedPatch(t *testing.T) {
	for _, status := range []string{"APPROVED", "REJECTED"} {
		a, db := mockApp(t)
		patchID := uuid()
		db.ExpectBegin()
		db.ExpectQuery("SELECT approval_status").WithArgs(patchID).WillReturnRows(pgxmock.NewRows([]string{"approval_status"}).AddRow(status))
		db.ExpectRollback()
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"candidate_id":"`+uuid()+`"}`))
		r.SetPathValue("id", patchID)
		w := httptest.NewRecorder()
		a.selectCandidate(w, r)
		if w.Code != 409 {
			t.Fatal(w.Code, w.Body.String())
		}
		if e := db.ExpectationsWereMet(); e != nil {
			t.Fatal(e)
		}
	}
}
func expectPendingCandidate(db pgxmock.PgxPoolIface, patchID string, published bool) {
	db.ExpectBegin()
	db.ExpectQuery("SELECT approval_status").WithArgs(patchID).WillReturnRows(pgxmock.NewRows([]string{"approval_status"}).AddRow("PENDING"))
	db.ExpectQuery("SELECT EXISTS").WithArgs(patchID).WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(published))
}
func TestCandidateSelectionLocksPublishedPatch(t *testing.T) {
	a, db := mockApp(t)
	patchID := uuid()
	expectPendingCandidate(db, patchID, true)
	db.ExpectRollback()
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"candidate_id":"`+uuid()+`"}`))
	r.SetPathValue("id", patchID)
	w := httptest.NewRecorder()
	a.selectCandidate(w, r)
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	if e := db.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
func TestCandidateCannotCrossPatchBoundary(t *testing.T) {
	a, db := mockApp(t)
	patchID, candidateID := uuid(), uuid()
	expectPendingCandidate(db, patchID, false)
	db.ExpectQuery("SELECT file_path,original_code").WithArgs(candidateID, patchID).WillReturnError(pgx.ErrNoRows)
	db.ExpectRollback()
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"candidate_id":"`+candidateID+`"}`))
	r.SetPathValue("id", patchID)
	w := httptest.NewRecorder()
	a.selectCandidate(w, r)
	if w.Code != 404 {
		t.Fatal(w.Code, w.Body.String())
	}
	if e := db.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
func candidateRows(status string, success bool) *pgxmock.Rows {
	return pgxmock.NewRows([]string{"file_path", "original_code", "patched_code", "unified_diff", "category", "variant", "test_output", "status", "diagnostics", "score", "ast_cost", "test_success"}).AddRow("inventory.py", "broken", "fixed", "@@ diff", "boundary", "guarded-index", "2 tests passed", status, []byte(`{"root_cause_analysis":"boundary exceeded","ast_node_summary":"Subscript index narrowed"}`), 0.94, 3, success)
}
func TestCandidateSelectionRequiresPassingFullVerification(t *testing.T) {
	for _, tc := range []struct {
		status  string
		success bool
	}{{"REJECTED", false}, {"VERIFIED", false}} {
		a, db := mockApp(t)
		patchID, candidateID := uuid(), uuid()
		expectPendingCandidate(db, patchID, false)
		db.ExpectQuery("SELECT file_path,original_code").WithArgs(candidateID, patchID).WillReturnRows(candidateRows(tc.status, tc.success))
		db.ExpectRollback()
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"candidate_id":"`+candidateID+`"}`))
		r.SetPathValue("id", patchID)
		w := httptest.NewRecorder()
		a.selectCandidate(w, r)
		if w.Code != 409 {
			t.Fatal(w.Code, w.Body.String())
		}
		if e := db.ExpectationsWereMet(); e != nil {
			t.Fatal(e)
		}
	}
}
func TestCandidateSelectionCommitsEvidenceAndAuditAtomically(t *testing.T) {
	a, db := mockApp(t)
	patchID, candidateID := uuid(), uuid()
	expectPendingCandidate(db, patchID, false)
	db.ExpectQuery("SELECT file_path,original_code").WithArgs(candidateID, patchID).WillReturnRows(candidateRows("VERIFIED", true))
	db.ExpectExec("UPDATE patch_candidates SET selected=false").WithArgs(patchID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec("UPDATE patch_candidates SET selected=true").WithArgs(candidateID, patchID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec("UPDATE healing_patches SET selected_candidate_id").WithArgs(patchID, candidateID, "inventory.py", "broken", "fixed", "@@ diff", "boundary", "2 tests passed", pgxmock.AnyArg(), 0.94, "Subscript index narrowed", "boundary exceeded").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec("INSERT INTO audit_logs").WithArgs(pgxmock.AnyArg(), "system", "patch.candidate.select", "patch", patchID, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectQuery("SELECT \\* FROM healing_patches").WithArgs(patchID).WillReturnRows(pgxmock.NewRows([]string{"id", "selected_candidate_id", "patched_code", "verification_status"}).AddRow(patchID, candidateID, "fixed", "VERIFIED"))
	db.ExpectCommit()
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"candidate_id":"`+candidateID+`"}`))
	r.SetPathValue("id", patchID)
	w := httptest.NewRecorder()
	a.selectCandidate(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), candidateID) {
		t.Fatal(w.Code, w.Body.String())
	}
	if e := db.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
func TestApprovalRejectsChangedCandidateWithoutDispatch(t *testing.T) {
	a, db := mockApp(t)
	j := jobFixture()
	j.SourcePatchID = uuid()
	candidate := uuid()
	j.SourceCandidateID = &candidate
	repo := uuid()
	expectJobWrites(db, j, repo, 1)
	db.ExpectExec("UPDATE healing_patches SET approval_status").WithArgs(j.SourcePatchID, "system", &candidate).WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	db.ExpectRollback()
	if e := a.createJob(context.Background(), repo, &j); !errors.Is(e, errPatchReview) {
		t.Fatal(e)
	}
	if e := db.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
