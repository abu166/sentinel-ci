package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
)

func (a *App) candidateList(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		fail(w, 400, errors.New("invalid patch UUID"))
		return
	}
	var selected string
	if e := a.db.QueryRow(r.Context(), "SELECT coalesce(selected_candidate_id::text,'') FROM healing_patches WHERE id=$1", id).Scan(&selected); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			fail(w, 404, errors.New("patch not found"))
		} else {
			fail(w, 500, e)
		}
		return
	}
	rows, e := a.query(r.Context(), "SELECT c.*,c.status AS verification_status,c.ast_cost AS ast_diff_cost,c.category AS bug_category FROM patch_candidates c WHERE c.patch_id=$1 ORDER BY c.rank,c.id", id)
	if e != nil {
		fail(w, 500, e)
		return
	}
	var selectedValue any
	if selected != "" {
		selectedValue = selected
	}
	write(w, 200, map[string]any{"candidates": rows, "selected_candidate_id": selectedValue})
}
func (a *App) selectCandidate(w http.ResponseWriter, r *http.Request) {
	patchID := r.PathValue("id")
	if !validID(patchID) {
		fail(w, 400, errors.New("invalid patch UUID"))
		return
	}
	var b struct {
		CandidateID string `json:"candidate_id"`
	}
	if !decode(w, r, &b) {
		return
	}
	if !validID(b.CandidateID) {
		fail(w, 400, errors.New("valid candidate_id required"))
		return
	}
	ctx := r.Context()
	tx, e := a.db.Begin(ctx)
	if e != nil {
		fail(w, 503, e)
		return
	}
	defer tx.Rollback(ctx)
	var approval string
	if e = tx.QueryRow(ctx, "SELECT approval_status FROM healing_patches WHERE id=$1 FOR UPDATE", patchID).Scan(&approval); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			fail(w, 404, errors.New("patch not found"))
		} else {
			fail(w, 500, e)
		}
		return
	}
	if approval != "PENDING" {
		fail(w, 409, errors.New("candidate selection is locked after patch review"))
		return
	}
	var published bool
	if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pull_requests WHERE patch_id=$1)", patchID).Scan(&published); e != nil {
		fail(w, 500, e)
		return
	}
	if published {
		fail(w, 409, errors.New("candidate selection is locked after Git publication"))
		return
	}
	var path, original, patched, diff, category, variant, output, status string
	var diagnostics []byte
	var score float64
	var cost int
	var success bool
	e = tx.QueryRow(ctx, "SELECT file_path,original_code,patched_code,unified_diff,category,variant,test_output,status,diagnostics,score,ast_cost,test_success FROM patch_candidates WHERE id=$1 AND patch_id=$2 FOR UPDATE", b.CandidateID, patchID).Scan(&path, &original, &patched, &diff, &category, &variant, &output, &status, &diagnostics, &score, &cost, &success)
	if e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			fail(w, 404, errors.New("candidate does not belong to this patch"))
		} else {
			fail(w, 500, e)
		}
		return
	}
	if status != "VERIFIED" || !success {
		fail(w, 409, errors.New("only candidates with passing verification and regression tests may be selected"))
		return
	}
	if !json.Valid(diagnostics) {
		fail(w, 500, errors.New("invalid candidate diagnostics"))
		return
	}
	var details map[string]any
	if json.Unmarshal(diagnostics, &details) != nil {
		fail(w, 500, errors.New("invalid candidate diagnostics"))
		return
	}
	summary, _ := details["ast_node_summary"].(string)
	if summary == "" {
		summary = fmt.Sprintf("Candidate %s; AST edit cost %d", variant, cost)
	}
	rootCause, _ := details["root_cause_analysis"].(string)
	if _, e = tx.Exec(ctx, "UPDATE patch_candidates SET selected=false WHERE patch_id=$1 AND selected=true", patchID); e != nil {
		fail(w, 500, e)
		return
	}
	if _, e = tx.Exec(ctx, "UPDATE patch_candidates SET selected=true WHERE id=$1 AND patch_id=$2", b.CandidateID, patchID); e != nil {
		fail(w, 500, e)
		return
	}
	if _, e = tx.Exec(ctx, "UPDATE healing_patches SET selected_candidate_id=$2,file_path=$3,original_code=$4,patched_code=$5,unified_diff=$6,bug_category=$7,test_output=$8,diagnostics=$9,confidence_score=least(1.0,greatest(0.0,$10::float8)),verification_status='VERIFIED',ast_node_summary=$11,root_cause_analysis=coalesce(nullif($12,''),root_cause_analysis) WHERE id=$1", patchID, b.CandidateID, path, original, patched, diff, category, output, diagnostics, score, summary, rootCause); e != nil {
		fail(w, 500, e)
		return
	}
	if e = auditTx(ctx, tx, "patch.candidate.select", "patch", patchID, map[string]any{"candidate_id": b.CandidateID, "variant": variant, "ast_cost": cost, "ranking_score": score}); e != nil {
		fail(w, 500, e)
		return
	}
	rows, e := tx.Query(ctx, "SELECT * FROM healing_patches WHERE id=$1", patchID)
	if e != nil {
		fail(w, 500, e)
		return
	}
	patch, e := pgx.CollectRows(rows, pgx.RowToMap)
	rows.Close()
	if e != nil || len(patch) != 1 {
		if e == nil {
			e = errors.New("updated patch unavailable")
		}
		fail(w, 500, e)
		return
	}
	normalizeUUIDRows(patch)
	if e = tx.Commit(ctx); e != nil {
		fail(w, 503, e)
		return
	}
	write(w, 200, map[string]any{"selected_candidate_id": b.CandidateID, "patch": patch[0]})
}
