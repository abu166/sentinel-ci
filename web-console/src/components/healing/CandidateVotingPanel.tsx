import { useEffect, useState } from "react";
import { Check, GitCompareArrows, RefreshCw, ShieldCheck } from "lucide-react";
import { Patch, request, stripAnsi } from "../../model";
export interface RepairCandidate {
  id: string;
  patch_id: string;
  rank: number;
  category: string;
  variant: string;
  file_path: string;
  original_code: string;
  patched_code: string;
  unified_diff: string;
  ast_cost: number;
  test_success: boolean;
  score: number;
  status: string;
  selected: boolean;
  test_output: string;
  diagnostics?: Patch["diagnostics"];
}
interface CandidateResponse {
  candidates: RepairCandidate[];
  selected_candidate_id?: string;
}
interface Props {
  patch: Patch;
  disabled: boolean;
  published: boolean;
  onSelected: (patch: Patch) => void;
}
export default function CandidateVotingPanel({
  patch,
  disabled,
  published,
  onSelected,
}: Props) {
  const [candidates, setCandidates] = useState<RepairCandidate[]>([]),
    [loading, setLoading] = useState(true),
    [error, setError] = useState(""),
    [selecting, setSelecting] = useState(""),
    [revision, setRevision] = useState(0);
  const locked =
    published ||
    patch.approval_status === "APPROVED" ||
    patch.approval_status === "REJECTED";
  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    setLoading(true);
    setError("");
    void request<CandidateResponse>(`/patches/${patch.id}/candidates`, {
      signal: controller.signal,
    })
      .then((result) => {
        if (active)
          setCandidates(
            [...result.candidates].sort((a, b) => a.rank - b.rank).slice(0, 3),
          );
      })
      .catch((reason) => {
        if (active) setError(String(reason));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [patch.id, revision]);
  async function choose(candidate: RepairCandidate) {
    if (
      disabled ||
      locked ||
      candidate.status !== "VERIFIED" ||
      candidate.test_success !== true
    )
      return;
    setSelecting(candidate.id);
    setError("");
    try {
      const result = await request<{
        selected_candidate_id: string;
        patch: Patch;
      }>(`/patches/${patch.id}/select-candidate`, {
        method: "POST",
        body: JSON.stringify({ candidate_id: candidate.id }),
      });
      setCandidates((previous) =>
        previous.map((item) => ({
          ...item,
          selected: item.id === result.selected_candidate_id,
        })),
      );
      onSelected(result.patch);
    } catch (reason) {
      setError(String(reason));
    } finally {
      setSelecting("");
    }
  }
  return (
    <div className="candidate-voting-panel">
      <div className="candidate-heading">
        <div>
          <span className="eyebrow">VERIFIED CANDIDATE VOTING</span>
          <h3>Compare repair evidence</h3>
          <p>
            Rank reflects the repair engine's test evidence and AST edit cost.
            Selection changes the reviewed patch; it does not approve or publish
            it.
          </p>
        </div>
        <button
          aria-label="Reload repair candidates"
          onClick={() => setRevision((v) => v + 1)}
          disabled={loading}
        >
          <RefreshCw size={13} />
        </button>
      </div>
      {loading ? (
        <div className="candidate-loading" role="status">
          Loading recorded candidates…
        </div>
      ) : error ? (
        <div className="candidate-error" role="alert">
          {error}
        </div>
      ) : !candidates.length ? (
        <div className="empty">
          No candidate records were stored for this repair. Run a new analysis
          to compare actual verified candidates.
        </div>
      ) : (
        <div className="candidate-list">
          {candidates.map((candidate) => (
            <article
              key={candidate.id}
              className={`repair-candidate ${candidate.selected ? "selected" : ""}`}
            >
              <div className="candidate-card-heading">
                <span className="candidate-rank">{`#${candidate.rank}`}</span>
                <div>
                  <h4>{candidate.variant || candidate.category}</h4>
                  <code>{candidate.file_path}</code>
                </div>
                {candidate.selected && (
                  <span className="selected-candidate">
                    <Check size={12} />
                    Selected
                  </span>
                )}
              </div>
              <dl className="candidate-measurements">
                <div>
                  <dt>Vote score</dt>
                  <dd>
                    {typeof candidate.score === "number" &&
                    Number.isFinite(candidate.score)
                      ? candidate.score.toFixed(3)
                      : "—"}
                  </dd>
                </div>
                <div>
                  <dt>Verification</dt>
                  <dd
                    className={
                      candidate.test_success ? "green" : "candidate-failed"
                    }
                  >
                    {candidate.test_success ? "Passed" : "Failed"}
                  </dd>
                </div>
                <div>
                  <dt>AST edit cost</dt>
                  <dd>
                    {typeof candidate.ast_cost === "number" &&
                    Number.isFinite(candidate.ast_cost)
                      ? candidate.ast_cost
                      : "—"}
                  </dd>
                </div>
              </dl>
              <details>
                <summary>
                  <ShieldCheck size={12} />
                  Test evidence
                </summary>
                <pre>
                  {stripAnsi(
                    candidate.test_output || "No test output was recorded.",
                  )}
                </pre>
              </details>
              <div className="candidate-card-footer">
                <span>{candidate.status.toLowerCase()}</span>
                <button
                  disabled={
                    disabled ||
                    locked ||
                    !!selecting ||
                    candidate.selected ||
                    candidate.status !== "VERIFIED" ||
                    candidate.test_success !== true
                  }
                  onClick={() => void choose(candidate)}
                >
                  <GitCompareArrows size={12} />
                  {selecting === candidate.id
                    ? "Selecting…"
                    : "Select candidate"}
                </button>
              </div>
            </article>
          ))}
        </div>
      )}
      {disabled && (
        <p className="candidate-access-note">
          Read-only role: candidate evidence is visible; selection requires a
          Maintainer or Admin.
        </p>
      )}
      {locked && (
        <p className="candidate-access-note">
          This repair is locked after approval, rejection, or publication.
        </p>
      )}
    </div>
  );
}
