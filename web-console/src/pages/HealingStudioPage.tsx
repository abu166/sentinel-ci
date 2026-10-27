import { ReactNode } from "react";
import { Sparkles } from "lucide-react";
import { Pipeline, sha } from "../model";
interface Props {
  pipelines: Pipeline[];
  selected: string;
  onSelect: (pipeline: Pipeline) => void;
  children: ReactNode;
}
export default function HealingStudioPage({
  pipelines,
  selected,
  onSelect,
  children,
}: Props) {
  return (
    <div className="studio-layout">
      <div className="studio-heading">
        <div>
          <span className="studio-heading-icon">
            <Sparkles size={17} />
          </span>
          <div>
            <h1>Healing studio</h1>
            <p>Inspect the evidence. Ship a verified repair.</p>
          </div>
        </div>
        <select
          aria-label="Select repair pipeline"
          value={selected}
          onChange={(e) => {
            const p = pipelines.find((p) => p.id === e.target.value);
            if (p) onSelect(p);
          }}
        >
          {pipelines.map((p) => (
            <option value={p.id} key={p.id}>
              {p.repo_name ?? p.repo_id} / {sha(p.commit_sha)} / {p.status}
            </option>
          ))}
        </select>
      </div>
      {children}
    </div>
  );
}
