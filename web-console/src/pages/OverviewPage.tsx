import { ReactNode } from "react";
import { ArrowUpRight, GitBranch, Search } from "lucide-react";
import { Badge, Commit } from "../components";
import { Pipeline, relativeAge } from "../model";
interface Props {
  visible: Pipeline[];
  selected: string;
  total: number;
  offline: boolean;
  filter: string;
  onFilter: (value: string) => void;
  onSelect: (pipeline: Pipeline) => void;
  selectedName: string;
  onOpenStudio: () => void;
  children: ReactNode;
}
export default function OverviewPage({
  visible,
  selected,
  total,
  offline,
  filter,
  onFilter,
  onSelect,
  selectedName,
  onOpenStudio,
  children,
}: Props) {
  return (
    <div className="overview-layout">
      <section className="pipeline-master">
        <div className="section-heading">
          <h1>Executions</h1>
          <span className="heading-count">{visible.length}</span>
        </div>
        <label className="master-search">
          <Search size={14} />
          <input
            aria-label="Filter executions"
            placeholder="Filter executions…"
            value={filter}
            onChange={(e) => onFilter(e.target.value)}
          />
          <kbd>⌕</kbd>
        </label>
        <div className="master-list">
          {visible.map((p) => (
            <button
              key={p.id}
              className={`pipeline-item ${p.id === selected ? "selected" : ""}`}
              onClick={() => onSelect(p)}
            >
              <div>
                <strong>{p.repo_name ?? p.repo_id.slice(0, 8)}</strong>
                <Badge status={p.status} />
              </div>
              <p>{p.commit_message ?? "Commit execution"}</p>
              <div>
                <code>
                  <GitBranch size={11} />
                  {p.branch}
                </code>
                <Commit value={p.commit_sha} />
              </div>
              <small>
                <span className="author-mark">
                  {p.triggered_by.slice(0, 1).toUpperCase()}
                </span>
                {p.triggered_by}
                <span title={p.started_at}>{relativeAge(p.started_at)}</span>
              </small>
            </button>
          ))}
          {!visible.length && (
            <div className="empty">No executions match your filter.</div>
          )}
        </div>
        <div className="pane-foot">
          {offline ? "Reference history" : "Execution history"}
          <span>{total} runs</span>
        </div>
      </section>
      <section className="overview-detail">
        <div className="section-heading">
          <div className="detail-heading">
            <span className="detail-repo-mark">
              <GitBranch size={13} />
            </span>
            <h1>{selectedName}</h1>
          </div>
          <button onClick={onOpenStudio}>
            Open studio <ArrowUpRight size={13} />
          </button>
        </div>
        {children}
      </section>
    </div>
  );
}
