import ASTCallGraph from "./components/healing/ASTCallGraph";
import CandidateVotingPanel from "./components/healing/CandidateVotingPanel";
import { useEffect, useRef, useState } from "react";
import {
  ArrowDown,
  Copy,
  Check,
  ChevronRight,
  ChevronDown,
  FileCode2,
  GitBranch,
  Search,
  Terminal as TerminalIcon,
} from "lucide-react";
import {
  ansiSegments,
  diagnostic,
  duration,
  stripAnsi,
  unifiedRows,
  Log,
  Patch,
  sha,
  Step,
  topologicalSteps,
} from "./model";
export function Badge({ status }: { status: string }) {
  return (
    <span className={`badge ${status.toLowerCase()}`}>
      <i />
      {status.toLowerCase().replace("_", " ")}
    </span>
  );
}
export function Sparkline({
  values,
  label,
}: {
  values: number[];
  label: string;
}) {
  if (values.length < 2)
    return <span className="no-series">collecting samples</span>;
  const low = Math.min(...values),
    high = Math.max(...values);
  const points = values
    .map(
      (v, i) =>
        `${(i * 92) / (values.length - 1)},${19 - ((v - low) / (high - low || 1)) * 16}`,
    )
    .join(" ");
  return (
    <svg
      width="92"
      height="22"
      viewBox="0 0 92 22"
      role="img"
      aria-label={label}
    >
      <polyline
        fill="none"
        stroke="currentColor"
        strokeWidth="1.4"
        points={points}
      />
    </svg>
  );
}
export function DAG({
  steps,
  selected,
  onSelect,
}: {
  steps: Step[];
  selected: string;
  onSelect: (step: Step) => void;
}) {
  const sorted = topologicalSteps(steps);
  const lookup = new Map(
    sorted.flatMap(
      (s, i) =>
        [
          [s.id, i],
          [s.name, i],
        ] as [string, number][],
    ),
  );
  const w = Math.max(900, sorted.length * 174 + 36);
  return (
    <div className="dag-canvas">
      <div className="canvas-grid" />
      <svg
        viewBox={`0 0 ${w} 154`}
        className="dag-svg"
        role="img"
        aria-label="Interactive pipeline dependency graph"
      >
        <defs>
          <marker
            id="dag-arrow"
            markerWidth="5"
            markerHeight="5"
            refX="4"
            refY="2.5"
            orient="auto"
          >
            <path d="M0,0 L5,2.5 L0,5" fill="var(--dag-edge)" />
          </marker>
        </defs>
        {sorted.flatMap((s, i) =>
          (s.depends_on ?? []).map((dep) => {
            const from = lookup.get(dep);
            if (from === undefined) return null;
            const x = from * 174 + 172,
              end = i * 174 + 20;
            return (
              <path
                key={`${dep}-${s.id}`}
                className={`dag-connector ${s.status.toLowerCase()}`}
                d={`M${x} 72 C${x + 30} 72 ${end - 30} 72 ${end} 72`}
                markerEnd="url(#dag-arrow)"
              />
            );
          }),
        )}
        {sorted.map((s, i) => (
          <g
            key={s.id}
            className={`graph-node ${s.status.toLowerCase()} ${selected === s.id ? "selected" : ""}`}
            transform={`translate(${i * 174 + 20},40)`}
            role="button"
            tabIndex={0}
            aria-label={`Inspect ${s.name}`}
            onClick={() => onSelect(s)}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") onSelect(s);
            }}
          >
            <text x="1" y="-11" className="node-stage">
              {String(i + 1).padStart(2, "0")} / {s.stage.toUpperCase()}
            </text>
            <rect width="152" height="65" rx="7" />
            <circle cx="14" cy="18" r="3.5" className="node-dot" />
            <text x="25" y="22" className="node-title">
              {s.name.length > 21 ? s.name.slice(0, 20) + "…" : s.name}
            </text>
            <text x="12" y="47" className="node-meta">
              {s.exit_code === undefined || s.exit_code === null
                ? s.status.toLowerCase()
                : `exit ${s.exit_code}`}
            </text>
            <text x="140" y="47" textAnchor="end" className="node-meta">
              {duration(s.duration_ms)}
            </text>
          </g>
        ))}
      </svg>
      {!steps.length && (
        <div className="canvas-empty">
          No execution steps have been recorded.
        </div>
      )}
      <span className="canvas-label">
        DAG / dependency graph · click a node to inspect
      </span>
    </div>
  );
}
function LogText({
  text,
  onLocation,
}: {
  text: string;
  onLocation: (file: string, line: number) => void;
}) {
  return (
    <>
      {ansiSegments(text).map((segment, s) => {
        const parts = segment.text.split(
          /((?:[\w./-]+\.(?:go|py|ts|tsx)):\d+(?::\d+)?|File "[^"\n]+", line \d+)/g,
        );
        return (
          <span key={s} style={{ color: segment.color }}>
            {parts.map((part, i) => {
              const go = part.match(/^([\w./-]+\.(?:go|py|ts|tsx)):(\d+)/);
              const py = part.match(/^File "([^"\n]+)", line (\d+)/);
              const m = go ?? py;
              return m ? (
                <button
                  key={i}
                  className="source-link"
                  onClick={() => onLocation(m[1], Number(m[2]))}
                >
                  {part}
                </button>
              ) : (
                <span key={i}>{part}</span>
              );
            })}
          </span>
        );
      })}
    </>
  );
}
export function Terminal({
  logs,
  selectedStep,
  onLocation,
  state,
}: {
  logs: Log[];
  selectedStep: string;
  onLocation: (file: string, line: number) => void;
  state: string;
}) {
  const [search, setSearch] = useState(""),
    [stream, setStream] = useState("all"),
    [copyState, setCopyState] = useState("idle"),
    [auto, setAuto] = useState(true);
  const ref = useRef<HTMLDivElement>(null);
  const shown = logs.filter(
    (l) =>
      (!selectedStep || l.step_id === selectedStep) &&
      (stream === "all" || l.stream === stream) &&
      l.content.toLowerCase().includes(search.toLowerCase()),
  );
  useEffect(() => {
    if (auto && ref.current) ref.current.scrollTop = ref.current.scrollHeight;
  }, [logs, auto]);
  return (
    <section className="terminal-pane">
      <div className="pane-title">
        <span>
          <TerminalIcon size={13} /> Terminal & stack trace
        </span>
        <div className="terminal-header-controls">
          <i className={`status-dot ${state === "live" ? "green" : ""}`} />
          <code>{state}</code>
          <button
            aria-label="Copy terminal output"
            title={
              copyState === "copied"
                ? "Copied"
                : copyState === "error"
                  ? "Clipboard unavailable"
                  : "Copy visible output"
            }
            onClick={async () => {
              try {
                await navigator.clipboard.writeText(
                  shown.map((l) => stripAnsi(l.content)).join("\n"),
                );
                setCopyState("copied");
              } catch {
                setCopyState("error");
              }
            }}
          >
            {copyState === "copied" ? <Check size={13} /> : <Copy size={13} />}
          </button>
          <button
            aria-label="Toggle auto-scroll"
            aria-pressed={auto}
            className={auto ? "active" : ""}
            onClick={() => setAuto(!auto)}
          >
            <ArrowDown size={12} />
          </button>
        </div>
      </div>
      <div className="terminal-tools">
        <label>
          <Search size={12} />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search logs…"
            aria-label="Search logs"
          />
        </label>
        <div
          className="terminal-stream-tabs"
          role="tablist"
          aria-label="Terminal streams"
        >
          {[
            { id: "all", name: "All output" },
            { id: "stdout", name: "stdout" },
            { id: "stderr", name: "stderr" },
          ].map((item) => (
            <button
              key={item.id}
              role="tab"
              aria-selected={stream === item.id}
              className={stream === item.id ? "active" : ""}
              onClick={() => setStream(item.id)}
            >
              {item.name}
              <span>
                {
                  logs.filter((l) => item.id === "all" || l.stream === item.id)
                    .length
                }
              </span>
            </button>
          ))}
        </div>
      </div>
      <div
        className="terminal-lines"
        role="log"
        aria-label="Execution logs"
        ref={ref}
      >
        {shown.map((l, i) => (
          <div
            className={`terminal-line ${l.stream} ${/\b(?:panic:|ERROR|FAIL(?:ED)?:|Traceback|IndexError|KeyError|TypeError|data race|fatal error)\b/i.test(stripAnsi(l.content)) ? "failure" : ""}`}
            key={`${l.step_id}-${l.sequence_num}-${i}`}
          >
            <span className="line-num">{l.sequence_num}</span>
            <span className="stream-label">
              {l.stream === "stderr" ? "ERR" : "OUT"}
            </span>
            <span>
              <LogText text={l.content} onLocation={onLocation} />
            </span>
          </div>
        ))}
        {!shown.length && (
          <div className="empty">
            {logs.length
              ? "No output matches this filter."
              : "Waiting for execution output."}
          </div>
        )}
      </div>
      <div className="pane-foot">
        <span>{`${shown.length} lines · UTF-8`}</span>
        <span>{selectedStep ? "selected step" : "all pipeline steps"}</span>
      </div>
    </section>
  );
}
function Syntax({ text, changed }: { text: string; changed?: string }) {
  const parts = text.split(
    /("[^"\n]*"|'[^'\n]*'|\b(?:func|def|return|if|else|for|range|var|import|package|nil|None|True|False|int|string|async|await|class|defer|go|select|case)\b|\b\d+\b)/g,
  );
  return (
    <>
      {parts.map((p, i) => (
        <span
          key={i}
          className={`${/^['"]/.test(p) ? "syntax-string" : /^(?:func|def|return|if|else|for|range|var|import|package|nil|None|True|False|async|await|class|defer|go|select|case)$/.test(p) ? "syntax-keyword" : /^\d+$/.test(p) ? "syntax-number" : ""} ${changed && p.trim() && !changed.includes(p) ? "token-changed" : ""}`}
        >
          {p}
        </span>
      ))}
    </>
  );
}
function linePairs(before: string, after: string) {
  const a = before.split("\n"),
    b = after.split("\n");
  const n = a.length,
    m = b.length;
  if (n * m > 200000)
    return a.map((line, i) => ({
      left: line,
      right: b[i] ?? "",
      leftNum: i + 1,
      rightNum: i + 1,
      changed: line !== b[i],
    }));
  const dp = Array.from({ length: n + 1 }, () => new Uint16Array(m + 1));
  for (let i = n - 1; i >= 0; i--)
    for (let j = m - 1; j >= 0; j--)
      dp[i][j] =
        a[i] === b[j]
          ? dp[i + 1][j + 1] + 1
          : Math.max(dp[i + 1][j], dp[i][j + 1]);
  const rows: {
    left: string;
    right: string;
    leftNum?: number;
    rightNum?: number;
    changed: boolean;
  }[] = [];
  let i = 0,
    j = 0;
  while (i < n || j < m) {
    if (i < n && j < m && a[i] === b[j]) {
      rows.push({
        left: a[i],
        right: b[j],
        leftNum: ++i,
        rightNum: ++j,
        changed: false,
      });
    } else {
      const removed: { s: string; num: number }[] = [],
        added: { s: string; num: number }[] = [];
      while ((i < n || j < m) && !(i < n && j < m && a[i] === b[j])) {
        if (i < n && (j >= m || dp[i + 1][j] >= dp[i][j + 1]))
          removed.push({ s: a[i], num: ++i });
        else added.push({ s: b[j], num: ++j });
      }
      for (let k = 0; k < Math.max(removed.length, added.length); k++)
        rows.push({
          left: removed[k]?.s ?? "",
          right: added[k]?.s ?? "",
          leftNum: removed[k]?.num,
          rightNum: added[k]?.num,
          changed: true,
        });
    }
  }
  return rows;
}
export function RepairPane({
  patch,
  location,
  onAction,
  disabled,
  busy,
  pulls,
  onSourceLocation,
  onPatchSelected,
  published = false,
}: {
  patch?: Patch;
  location?: { file: string; line: number };
  onAction: (action: string) => void;
  disabled: boolean;
  busy: boolean;
  pulls: React.ReactNode;
  onSourceLocation?: (file: string, line: number) => void;
  onPatchSelected?: (patch: Patch) => void;
  published?: boolean;
}) {
  const [mode, setMode] = useState("split"),
    [tab, setTab] = useState("code"),
    [graphFunction, setGraphFunction] = useState("");
  const repairScroll = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!location) return;
    setTab("code");
  }, [location]);
  useEffect(() => {
    if (!location || tab !== "code" || !repairScroll.current) return;
    const frame = requestAnimationFrame(() => {
      const content = repairScroll.current,
        row = content?.querySelector<HTMLElement>(".focused-line");
      if (content && row)
        content.scrollTop = Math.max(
          0,
          content.scrollTop +
            row.getBoundingClientRect().top -
            content.getBoundingClientRect().top -
            content.clientHeight / 3,
        );
    });
    return () => cancelAnimationFrame(frame);
  }, [location, tab]);
  if (!patch)
    return (
      <section className="repair-pane">
        <div className="pane-title">
          <span>
            <FileCode2 size={13} /> Repair inspector
          </span>
        </div>
        <div className="empty large">
          No repair selected.
          <br />
          Select a healed or failed pipeline to inspect its evidence.
        </div>
      </section>
    );
  const sourceMatches =
    !!location &&
    (location.file === patch.file_path ||
      location.file.endsWith(patch.file_path) ||
      patch.file_path.endsWith(location.file));
  const ast = diagnostic(patch),
    rows = linePairs(patch.original_code, patch.patched_code);
  const rankings =
    patch.diagnostics?.rankings ??
    (ast.name
      ? [
          {
            name: ast.name,
            ochiai: ast.ochiai ?? 0,
            complexity: ast.complexity,
          },
        ]
      : []);
  return (
    <section className="repair-pane">
      <div className="pane-title">
        <span>
          <FileCode2 size={13} />
          <code>{patch.file_path}</code>
        </span>
        <Badge status={patch.verification_status} />
      </div>
      <div className="repair-tabs">
        <div>
          {[
            "code",
            "diagnostics",
            "candidates",
            "verification",
            "pull requests",
          ].map((t) => (
            <button
              key={t}
              className={tab === t ? "active" : ""}
              onClick={() => setTab(t)}
            >
              {t}
            </button>
          ))}
        </div>
        {tab === "code" && (
          <div className="segmented">
            <button
              className={mode === "split" ? "active" : ""}
              onClick={() => setMode("split")}
            >
              Split
            </button>
            <button
              className={mode === "unified" ? "active" : ""}
              onClick={() => setMode("unified")}
            >
              Unified
            </button>
          </div>
        )}
      </div>
      {location && (
        <div className="source-focus">
          AST focus{" "}
          <code>
            {location.file}:{location.line}
          </code>
        </div>
      )}
      <div className="repair-content" key={patch.id} ref={repairScroll}>
        {tab === "code" ? (
          mode === "split" ? (
            <div className="split-diff">
              <div className="diff-column-head">
                Original <span>read only</span>
              </div>
              <div className="diff-column-head">
                Patched <span>candidate</span>
              </div>
              {rows.map((row, i) => (
                <div className="diff-row" key={i}>
                  <div
                    className={`code-line ${row.changed ? "removed" : ""} ${sourceMatches && location?.line === row.leftNum ? "focused-line" : ""}`}
                  >
                    <span>{row.leftNum ?? ""}</span>
                    <code>
                      <Syntax
                        text={row.left}
                        changed={row.changed ? row.right : undefined}
                      />
                    </code>
                  </div>
                  <div
                    className={`code-line ${row.changed ? "added" : ""} ${sourceMatches && location?.line === row.rightNum ? "focused-line" : ""}`}
                  >
                    <span>{row.rightNum ?? ""}</span>
                    <code>
                      <Syntax
                        text={row.right}
                        changed={row.changed ? row.left : undefined}
                      />
                    </code>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="unified-diff">
              {unifiedRows(patch.unified_diff).map((row, i) => (
                <div className={`code-line unified-line ${row.kind}`} key={i}>
                  <span className="old-line-number">{row.oldLine ?? ""}</span>
                  <span className="new-line-number">{row.newLine ?? ""}</span>
                  <code>
                    <Syntax text={row.text} />
                  </code>
                </div>
              ))}
            </div>
          )
        ) : tab === "diagnostics" ? (
          <div className="diagnostics">
            <div className="diagnostic-summary">
              <span className="eyebrow">AST ROOT-CAUSE DIAGNOSTIC</span>
              <h3>{patch.bug_category}</h3>
              <p>{patch.root_cause_analysis}</p>
            </div>
            <ASTCallGraph
              graph={patch.diagnostics?.call_graph}
              onSelect={(node) => setGraphFunction(node.name)}
              onInspect={(file, line) => {
                onSourceLocation?.(file, line);
                setTab("code");
              }}
            />
            <table>
              <thead>
                <tr>
                  <th>Suspicious function</th>
                  <th>Ochiai</th>
                  <th>Complexity</th>
                </tr>
              </thead>
              <tbody>
                {rankings.map((r) => (
                  <tr
                    key={r.name}
                    className={
                      graphFunction === r.name ? "ast-ranked-active" : ""
                    }
                  >
                    <td>
                      <code>{r.name}</code>
                    </td>
                    <td>
                      <div className="score">
                        <i
                          style={{
                            width: `${Math.max(0, Math.min(1, r.ochiai)) * 100}%`,
                          }}
                        />
                        <code>{r.ochiai.toFixed(3)}</code>
                      </div>
                    </td>
                    <td>
                      <code>{r.complexity ?? "—"}</code>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            <dl>
              <dt>Mutation</dt>
              <dd>
                {patch.diagnostics?.mutation ?? patch.root_cause_analysis}
              </dd>
              <dt>Complexity delta</dt>
              <dd>
                <code>
                  {patch.diagnostics?.complexity_before !== undefined
                    ? `${patch.diagnostics.complexity_before} → ${patch.diagnostics.complexity_after ?? "—"}`
                    : "Not reported by analyzer"}
                </code>
              </dd>
              <dt>Source span</dt>
              <dd>
                <code>
                  {ast.start_line ?? "—"}–{ast.end_line ?? "—"}
                </code>
              </dd>
              <dt>Verification confidence</dt>
              <dd>
                {`${Math.round(patch.confidence_score * 100)}% · candidate evidence, not a correctness guarantee`}
              </dd>
            </dl>
          </div>
        ) : tab === "candidates" ? (
          <CandidateVotingPanel
            patch={patch}
            disabled={disabled}
            published={published}
            onSelected={(next) => {
              onPatchSelected?.(next);
            }}
          />
        ) : tab === "verification" ? (
          <pre className="verification-output">
            {patch.test_output || "No verification output recorded."}
          </pre>
        ) : (
          pulls
        )}
      </div>
      <div className="repair-action-dock">
        <span className="action-dock-status">
          <span className="status-dot green" />
          {patch.verification_status === "VERIFIED"
            ? "Verified candidate"
            : "Repair evidence"}
        </span>
        <div className="repair-actions">
          <button
            disabled={
              disabled ||
              busy ||
              patch.verification_status !== "VERIFIED" ||
              patch.approval_status === "REJECTED"
            }
            className="primary"
            onClick={() => onAction("apply")}
          >
            Approve & Apply
          </button>
          <button
            disabled={
              disabled ||
              busy ||
              patch.verification_status !== "VERIFIED" ||
              patch.approval_status === "REJECTED"
            }
            onClick={() => onAction("publish")}
          >
            <GitBranch size={12} />
            Publish Branch / PR
          </button>
          <button
            disabled={disabled || busy}
            onClick={() => onAction("verify")}
          >
            Re-run Regression
          </button>
          <button
            disabled={disabled || busy}
            onClick={() => onAction("race")}
            title="Go race detector verification"
          >
            Go Race
          </button>
          <button
            disabled={disabled || busy || patch.approval_status === "REJECTED"}
            className="danger"
            onClick={() => onAction("reject")}
          >
            Reject
          </button>
        </div>
      </div>
      {disabled && (
        <div className="readonly-note">
          Read-only session · sign in as Admin to mutate executions.
        </div>
      )}
    </section>
  );
}
export function Commit({ value }: { value: string }) {
  return <code className="commit-sha">{sha(value)}</code>;
}
export function Select({
  label,
  value,
  onChange,
  options,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: { value: string; label: string }[];
}) {
  return (
    <label className="select-label">
      <span>{label}</span>
      <select
        aria-label={label}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      >
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
      <ChevronDown size={11} />
    </label>
  );
}
