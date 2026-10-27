import { useState } from "react";
import { Activity, Cpu, HardDrive } from "lucide-react";
import { memory, relativeAge } from "../../model";
import CanvasSparkline from "./CanvasSparkline";
import { WorkerHistory } from "./series";
export default function WorkerTelemetryPanel({
  history,
  offline,
}: {
  history: WorkerHistory;
  offline: boolean;
}) {
  const [selection, setSelection] = useState("");
  const entries = [...history.values()].sort((a, b) =>
    a.name.localeCompare(b.name),
  );
  const series = entries.find((entry) => entry.key === selection) ?? entries[0];
  const latest = series?.samples.at(-1);
  const cpu = latest?.cpuPercent,
    memoryBytes = latest?.memoryBytes;
  return (
    <section
      className="worker-telemetry-panel"
      aria-label="Live worker resource charts"
    >
      <div className="telemetry-panel-heading">
        <div>
          <Activity size={14} />
          <h2>Resource history</h2>
          <span>{series?.scope ?? "Awaiting live samples"}</span>
        </div>
        <select
          aria-label="Select telemetry host"
          disabled={!entries.length}
          value={series?.key ?? ""}
          onChange={(e) => setSelection(e.target.value)}
        >
          {!entries.length && <option value="">Awaiting host samples</option>}
          {entries.map((entry) => (
            <option key={entry.key} value={entry.key}>
              {entry.name}
            </option>
          ))}
        </select>
      </div>
      <div className="worker-chart-grid">
        <div className="worker-chart">
          <div>
            <span>
              <Cpu size={12} />
              CPU utilization
            </span>
            <output aria-label="Latest host CPU">
              {cpu == null ? "—" : `${cpu.toFixed(1)}%`}
            </output>
          </div>
          <CanvasSparkline
            samples={series?.samples ?? []}
            metric="cpu"
            label={`${series?.name ?? "Unreported host"} ${series?.scope ?? "unreported"} CPU history`}
          />
        </div>
        <div className="worker-chart">
          <div>
            <span>
              <HardDrive size={12} />
              Resident memory
            </span>
            <output aria-label="Latest host memory">
              {memory(memoryBytes)}
            </output>
          </div>
          <CanvasSparkline
            samples={series?.samples ?? []}
            metric="memory"
            label={`${series?.name ?? "Unreported host"} ${series?.scope ?? "unreported"} memory history`}
          />
        </div>
      </div>
      <div className="telemetry-panel-foot">
        <span>
          {series
            ? `${series.samples.length} real samples · latest ${relativeAge(latest ? new Date(latest.timestamp).toISOString() : undefined)}`
            : "History starts with the first live API sample; no synthetic points."}
        </span>
        <span>
          {offline
            ? "Connection paused"
            : series?.scope === "controller_host"
              ? "Shared host measurements; not per-routine attribution"
              : latest?.timestampSource === "client_observation"
                ? "Client-observed timestamps"
                : "Runner-reported timestamps"}
        </span>
      </div>
    </section>
  );
}
