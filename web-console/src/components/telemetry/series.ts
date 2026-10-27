import { Worker } from "../../model";
export interface TelemetrySample {
  timestamp: number;
  cpuPercent: number | null;
  memoryBytes: number | null;
  timestampSource: "runner" | "client_observation";
}
export interface WorkerSeries {
  key: string;
  name: string;
  scope: string;
  samples: TelemetrySample[];
  lastObservedAt: number;
}
export type WorkerHistory = Map<string, WorkerSeries>;
export const MAX_TELEMETRY_SAMPLES = 120;
export const MAX_TELEMETRY_HOSTS = 64;
export function validReading(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) && value >= 0
    ? value
    : null;
}
export function workerSeriesKey(worker: Worker): {
  key: string;
  name: string;
  scope: string;
} {
  const scope = worker.metrics_scope ?? "unreported";
  const name =
    scope === "controller_host"
      ? (worker.hostname ??
        worker.host ??
        worker.id ??
        worker.worker_id ??
        "unreported-host")
      : (worker.id ??
        worker.worker_id ??
        worker.hostname ??
        worker.host ??
        "unreported-worker");
  return { key: `${scope}:${name}`, name, scope };
}
export function observeWorkerSamples(
  previous: WorkerHistory,
  workers: Worker[],
  observedAt: number,
): WorkerHistory {
  const next = new Map(previous),
    groups = new Map<
      string,
      {
        worker: Worker;
        sample: TelemetrySample;
        identity: ReturnType<typeof workerSeriesKey>;
      }
    >();
  for (const worker of workers) {
    const identity = workerSeriesKey(worker),
      raw = worker.sampled_at ?? worker.heartbeat_at ?? worker.last_heartbeat;
    const stamp = raw ? Date.parse(raw) : NaN;
    const sample: TelemetrySample = {
      timestamp: Number.isFinite(stamp) ? stamp : observedAt,
      cpuPercent: validReading(worker.cpu_percent),
      memoryBytes: validReading(worker.memory_bytes),
      timestampSource: Number.isFinite(stamp) ? "runner" : "client_observation",
    };
    const prior = groups.get(identity.key);
    if (!prior || sample.timestamp >= prior.sample.timestamp)
      groups.set(identity.key, { worker, sample, identity });
  }
  for (const { sample, identity } of groups.values()) {
    const existing = next.get(identity.key),
      last = existing?.samples.at(-1);
    if (existing && last && sample.timestamp <= last.timestamp) {
      next.set(identity.key, { ...existing, lastObservedAt: observedAt });
      continue;
    }
    next.set(identity.key, {
      ...identity,
      samples: [...(existing?.samples ?? []), sample].slice(
        -MAX_TELEMETRY_SAMPLES,
      ),
      lastObservedAt: observedAt,
    });
  }
  return new Map(
    [...next.entries()]
      .sort((a, b) => b[1].lastObservedAt - a[1].lastObservedAt)
      .slice(0, MAX_TELEMETRY_HOSTS),
  );
}
export function metricSegments(
  samples: TelemetrySample[],
  metric: "cpu" | "memory",
): { timestamp: number; value: number }[][] {
  const result: { timestamp: number; value: number }[][] = [];
  let current: { timestamp: number; value: number }[] = [];
  for (const sample of samples) {
    const raw = metric === "cpu" ? sample.cpuPercent : sample.memoryBytes;
    if (raw === null) {
      if (current.length) result.push(current);
      current = [];
      continue;
    }
    current.push({
      timestamp: sample.timestamp,
      value: metric === "memory" ? raw / 1048576 : raw,
    });
  }
  if (current.length) result.push(current);
  return result;
}
