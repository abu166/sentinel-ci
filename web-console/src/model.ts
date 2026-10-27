export type Status =
  | "PENDING"
  | "RUNNING"
  | "FAILED"
  | "HEALING"
  | "HEALED"
  | "PASSED"
  | "SKIPPED";
export type Role = "admin" | "maintainer" | "viewer";
export interface SandboxPolicy {
  backend?: string;
  isolated?: boolean;
  read_only_root?: boolean;
  network?: string;
  memory_mb?: number;
  cpus?: number;
  pids_limit?: number;
  secrets_scrubbed?: boolean;
}
export interface Step {
  id: string;
  name: string;
  stage: string;
  command: string;
  status: Status;
  duration_ms?: number;
  exit_code?: number;
  depends_on: string[];
  peak_rss_bytes?: number;
  memory_peak_bytes?: number;
  cpu_user_seconds?: number;
  cpu_system_seconds?: number;
  cpu_user_ms?: number;
  cpu_system_ms?: number;
  sandbox_policy?: string | SandboxPolicy;
  sandbox_status?: string;
}
export interface Pipeline {
  id: string;
  repo_id: string;
  repo_name?: string;
  commit_sha: string;
  commit_message?: string;
  branch: string;
  status: Status;
  triggered_by: string;
  started_at?: string;
  finished_at?: string;
  steps?: Step[];
  patches?: Patch[];
  logs?: Log[];
}
export interface ASTFunctionNode {
  id: string;
  name: string;
  file_path: string;
  start_line: number;
  end_line: number;
  complexity: number;
  suspiciousness?: number;
}
export interface ASTCallGraphData {
  nodes: ASTFunctionNode[];
  edges: { source: string; target: string; kind: string }[];
  root_id?: string;
  provenance?: string;
}
export interface Patch {
  id: string;
  pipeline_id: string;
  step_id: string;
  file_path: string;
  bug_category: string;
  root_cause_analysis: string;
  ast_node_summary: string;
  original_code: string;
  patched_code: string;
  unified_diff: string;
  verification_status: string;
  approval_status?: string;
  selected_candidate_id?: string;
  confidence_score: number;
  test_output: string;
  diagnostics?: {
    rankings?: { name: string; ochiai: number; complexity?: number }[];
    complexity_before?: number;
    complexity_after?: number;
    mutation?: string;
    call_graph?: ASTCallGraphData;
  };
}
export interface Log {
  sequence_num: number;
  stream: string;
  content: string;
  step_id?: string;
  timestamp?: string;
  event?: string;
  patch?: Patch;
}
export interface Repository {
  id: string;
  name: string;
  repo_url: string;
  default_branch: string;
  webhook_configured?: boolean;
  webhook_secret_configured?: boolean;
  auto_commit?: boolean;
  open_pr?: boolean;
  manual_approval?: boolean;
  reliability_score?: number;
  success_rate?: number;
  pipeline_count?: number;
}
export interface Worker {
  id?: string;
  worker_id?: string;
  hostname?: string;
  host?: string;
  metrics_scope?: string;
  sampled_at?: string;
  routine?: number;
  heartbeat_at?: string;
  last_heartbeat?: string;
  status?: string;
  cpu_percent?: number | null;
  memory_bytes?: number | null;
  peak_rss_bytes?: number;
  sandbox_status?: string;
  sandbox_policy?: string | SandboxPolicy;
  active_pipeline_id?: string;
}
export interface Queue {
  name: string;
  messages?: number;
  messages_ready?: number;
  messages_unacknowledged?: number;
  consumers?: number;
  status?: string;
}
export interface Pull {
  id: string;
  pipeline_id?: string;
  title?: string;
  branch?: string;
  head_branch?: string;
  commit_sha?: string;
  status?: string;
  state?: string;
  url?: string;
  provider?: string;
  unified_diff?: string;
  created_at?: string;
}
export interface Sample {
  timestamp?: string;
  build_duration_ms?: number;
  queue_depth?: number;
  cpu_percent?: number | null;
  memory_bytes?: number | null;
}
export interface Metrics {
  total_pipelines: number;
  auto_healed_bugs?: number;
  healed_pipelines?: number;
  mttr_seconds: number;
  active_workers: number;
  queue_depth: number;
  healing_success_rate: number;
  p95_build_duration_ms?: number;
  p95_build_seconds?: number;
  worker_cpu_percent?: number;
  worker_memory_bytes?: number;
  timeseries?: Sample[];
  throughput?: { day?: string; date?: string; count: number }[];
}
export const scenarios = [
  {
    id: "nil-pointer",
    name: "Go Nil-Pointer Dereference",
    repo: "payment-gateway",
  },
  {
    id: "off-by-one",
    name: "Python Off-by-One Slice Boundary",
    repo: "inventory-worker",
  },
  {
    id: "data-race",
    name: "Go Concurrent Map Write / Data Race",
    repo: "order-matching-engine",
  },
  {
    id: "key-error",
    name: "Python Unhandled KeyError in Async Worker",
    repo: "telemetry-ingestor",
  },
  {
    id: "context-timeout",
    name: "Go Context Timeout / Goroutine Leak",
    repo: "auth-proxy",
  },
];
export const previewSteps: Step[] = [
  {
    id: "s1",
    name: "Checkout",
    stage: "checkout",
    command: "git checkout e41b80d",
    status: "PASSED",
    duration_ms: 440,
    exit_code: 0,
    depends_on: [],
  },
  {
    id: "s2",
    name: "Lint & Static Check",
    stage: "lint",
    command: "python3 -m compileall .",
    status: "PASSED",
    duration_ms: 920,
    exit_code: 0,
    depends_on: ["s1"],
  },
  {
    id: "s3",
    name: "Unit Test Suite",
    stage: "test",
    command: "python3 -m unittest discover -v",
    status: "FAILED",
    duration_ms: 3420,
    exit_code: 1,
    depends_on: ["s2"],
  },
  {
    id: "s4",
    name: "AST Fault Localizer",
    stage: "heal",
    command: "AST context slicing",
    status: "HEALED",
    duration_ms: 250,
    exit_code: 0,
    depends_on: ["s3"],
  },
  {
    id: "s5",
    name: "Sandbox Patch Verify",
    stage: "verify",
    command: "python3 -m unittest discover -v",
    status: "PASSED",
    duration_ms: 1080,
    exit_code: 0,
    depends_on: ["s4"],
  },
  {
    id: "s6",
    name: "PR Publisher",
    stage: "publish",
    command: "git apply --check repair.patch",
    status: "PENDING",
    depends_on: ["s5"],
  },
];
const repoNames = [
  "inventory-worker",
  "payment-gateway",
  "order-matching-engine",
  "telemetry-ingestor",
  "auth-proxy",
];
export const previewPipelines: Pipeline[] = Array.from(
  { length: 18 },
  (_, i) => ({
    id: `preview-${i + 1}`,
    repo_id: `repo-${i % 5}`,
    repo_name: repoNames[i % 5],
    commit_sha: ["e41b80d", "7f3a9c2", "9c210fa", "a8f92d1", "c1d7e60"][i % 5],
    commit_message: [
      "fix(inventory): bound pagination cursor",
      "feat(billing): parallelize invoice batches",
      "fix(cache): shard LRU eviction lock",
      "feat(ingest): normalize missing event fields",
      "fix(auth): propagate request deadlines",
    ][i % 5],
    branch: [
      "fix/pagination-boundary",
      "main",
      "feat/order-batching",
      "fix/event-schema",
      "release/2.4",
    ][i % 5],
    status: (
      ["HEALED", "PASSED", "RUNNING", "FAILED", "HEALING", "PASSED"] as Status[]
    )[i % 6],
    triggered_by: ["abukhasym", "s.kassymov", "ci-bot"][i % 3],
    started_at: new Date(Date.now() - i * 600000).toISOString(),
    steps: previewSteps.map((s) => ({
      ...s,
      id: `${i}-${s.id}`,
      depends_on: s.depends_on.map((d) => `${i}-${d}`),
    })),
  }),
);
export const previewPatch: Patch = {
  id: "preview-patch",
  pipeline_id: "preview-1",
  step_id: "0-s3",
  file_path: "pkg/inventory/pagination.py",
  bug_category: "boundary-off-by-one",
  root_cause_analysis:
    "The index expression uses the sequence length as an inclusive offset. The final valid index is len(items) - 1. The candidate modifies only this subscript and preserves the function contract.",
  ast_node_summary: JSON.stringify({
    name: "last_item",
    start_line: 1,
    end_line: 2,
    complexity: 1,
    ochiai: 1,
    tarantula: 1,
    helpers: [],
  }),
  original_code: "def last_item(items):\n    return items[len(items)]\n",
  patched_code: "def last_item(items):\n    return items[len(items) - 1]\n",
  unified_diff:
    "--- a/pkg/inventory/pagination.py\n+++ b/pkg/inventory/pagination.py\n@@ -1,2 +1,2 @@\n def last_item(items):\n-    return items[len(items)]\n+    return items[len(items) - 1]\n",
  verification_status: "VERIFIED",
  confidence_score: 0.94,
  test_output:
    "[failing test command]\ntest_last_item ... ok\n[regression command]\nRan 8 tests in 0.014s\nOK",
};
export const previewLogs: Log[] = [
  "[runner-02] checkout e41b80d into isolated workspace",
  "[sandbox] network=none rootfs=readonly memory=256MiB",
  "[lint] static checks passed",
  "test_last_item (test_pagination.TestPagination) ... ERROR",
  "Traceback (most recent call last):",
  '  File "pkg/inventory/pagination.py", line 2, in last_item',
  "    return items[len(items)]",
  "IndexError: list index out of range",
  "[localizer] FunctionDef last_item · Ochiai 1.00",
  "[repair] Subscript → len(items) - 1",
  "[verification] failing test + regression suite passed",
].map((content, i) => ({
  sequence_num: i + 1,
  stream: i > 2 && i < 8 ? "stderr" : "stdout",
  step_id: i < 3 ? "0-s2" : i < 8 ? "0-s3" : i < 10 ? "0-s4" : "0-s5",
  content,
}));
export const previewRepos: Repository[] = repoNames.map((name, i) => ({
  id: `repo-${i}`,
  name,
  repo_url: `https://github.com/sentinel-labs/${name}`,
  default_branch: "main",
  webhook_configured: true,
  auto_commit: i % 2 === 0,
  open_pr: true,
  manual_approval: true,
  reliability_score: [96.4, 98.2, 91.7, 94.8, 99.1][i],
  pipeline_count: [84, 126, 73, 52, 95][i],
}));
export const previewWorkers: Worker[] = Array.from({ length: 4 }, (_, i) => ({
  id: `runner-eu-01/routine-${i}`,
  hostname: "runner-eu-01",
  routine: i,
  heartbeat_at: new Date(Date.now() - i * 900).toISOString(),
  status: i < 2 ? "BUSY" : "IDLE",
  cpu_percent: [18.2, 34.7, 1.1, 0.4][i],
  memory_bytes: [83100000, 142000000, 18000000, 17000000][i],
  peak_rss_bytes: [123000000, 184000000, 41000000, 39000000][i],
  sandbox_status: "isolated",
  sandbox_policy: "network=none · rootfs=readonly",
}));
export const previewQueues: Queue[] = [
  {
    name: "jobs.dispatch",
    messages_ready: 2,
    messages_unacknowledged: 2,
    consumers: 4,
  },
  {
    name: "logs.stream",
    messages_ready: 0,
    messages_unacknowledged: 0,
    consumers: 0,
  },
  {
    name: "healer.trigger",
    messages_ready: 0,
    messages_unacknowledged: 1,
    consumers: 1,
  },
  {
    name: "healer.result",
    messages_ready: 0,
    messages_unacknowledged: 0,
    consumers: 1,
  },
  {
    name: "healer.dead",
    messages_ready: 0,
    messages_unacknowledged: 0,
    consumers: 0,
  },
];
export const previewMetrics: Metrics = {
  total_pipelines: 126,
  auto_healed_bugs: 18,
  mttr_seconds: 14.2,
  active_workers: 4,
  queue_depth: 2,
  healing_success_rate: 94.7,
  p95_build_duration_ms: 18340,
  worker_cpu_percent: 13.6,
  worker_memory_bytes: 260100000,
  timeseries: [
    12000, 9300, 15400, 11200, 18340, 10200, 14200, 8900, 13400, 11400, 16100,
    12300,
  ].map((n, i) => ({
    timestamp: new Date(Date.now() - (12 - i) * 60000).toISOString(),
    build_duration_ms: n,
    queue_depth: [1, 0, 3, 2, 5, 1, 0, 2, 4, 1, 3, 2][i],
    cpu_percent: [8, 12, 7, 19, 14, 22, 8, 13, 31, 17, 23, 14][i],
    memory_bytes:
      (190 + [12, 32, 21, 41, 19, 54, 31, 28, 60, 44, 71, 70][i]) * 1000000,
  })),
};
let token = "";
export function setToken(value: string) {
  token = value;
}
export function getToken() {
  return token;
}
export async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const response = await fetch(`/api/v1${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...options.headers,
    },
    signal: options.signal ?? AbortSignal.timeout(12000),
  });
  if (!response.ok) {
    let message = await response.text();
    try {
      message = JSON.parse(message).error ?? message;
    } catch {}
    throw new Error(`${response.status}: ${message}`);
  }
  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}
export function collection<T>(value: unknown, key: string): T[] {
  if (Array.isArray(value)) return value as T[];
  if (value && typeof value === "object") {
    const v = value as Record<string, unknown>;
    const found = v[key] ?? v.items ?? v.data;
    return Array.isArray(found) ? (found as T[]) : [];
  }
  return [];
}
export function sha(value: string) {
  return /^[a-f0-9]{7,40}$/i.test(value) ? value.slice(0, 7) : "—";
}
export function duration(ms?: number) {
  return ms === undefined || ms === null ? "—" : `${(ms / 1000).toFixed(2)}s`;
}
export function memory(bytes?: number | null) {
  return bytes == null ? "—" : `${(bytes / 1048576).toFixed(1)} MiB`;
}
export function stripAnsi(value: string) {
  return value.replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, "");
}
export function ansiSegments(
  value: string,
): { text: string; color?: string }[] {
  let color: string | undefined;
  return value.split(/(\x1b\[[0-9;]*m)/).flatMap((text) => {
    const m = text.match(/^\x1b\[([0-9;]*)m$/);
    if (m) {
      for (const code of m[1].split(";").map(Number)) {
        if (code === 0 || code === 39) color = undefined;
        else if (code >= 30 && code <= 37)
          color = [
            "#8492a6",
            "#fb7185",
            "#5ee4b3",
            "#fbbf24",
            "#7db8ff",
            "#c4a0ff",
            "#67e8f9",
            "#e2e8f0",
          ][code - 30];
      }
      return [];
    }
    return text ? [{ text, color }] : [];
  });
}
export function topologicalSteps(steps: Step[]) {
  const remaining = [...steps],
    result: Step[] = [],
    done = new Set<string>();
  while (remaining.length) {
    const index = remaining.findIndex((s) =>
      (s.depends_on ?? []).every(
        (d) => done.has(d) || !steps.some((x) => x.id === d || x.name === d),
      ),
    );
    if (index < 0) return [...result, ...remaining];
    const [s] = remaining.splice(index, 1);
    result.push(s);
    done.add(s.id);
    done.add(s.name);
  }
  return result;
}
export function diagnostic(patch: Patch) {
  try {
    return JSON.parse(patch.ast_node_summary) as {
      name?: string;
      start_line?: number;
      end_line?: number;
      complexity?: number;
      ochiai?: number;
      helpers?: string[];
    };
  } catch {
    return { name: patch.file_path };
  }
}

export function sandboxPolicy(
  value: string | SandboxPolicy | null | undefined,
): string {
  if (!value) return "Policy not reported";
  if (typeof value === "string") return value;
  const fields = [
    value.backend,
    value.network === undefined ? undefined : `network=${value.network}`,
    value.read_only_root === undefined
      ? undefined
      : `rootfs=${value.read_only_root ? "read-only" : "writable"}`,
    value.memory_mb === undefined ? undefined : `${value.memory_mb} MiB`,
    value.cpus === undefined ? undefined : `${value.cpus} CPU`,
    value.pids_limit === undefined ? undefined : `pids=${value.pids_limit}`,
    value.secrets_scrubbed === undefined
      ? undefined
      : `secrets=${value.secrets_scrubbed ? "scrubbed" : "inherited"}`,
  ];
  return fields.filter(Boolean).join(" · ") || "Policy not reported";
}

export function relativeAge(value?: string, now = Date.now()): string {
  if (!value) return "Queued";
  const ms = now - new Date(value).getTime();
  if (!Number.isFinite(ms)) return "Unknown";
  if (ms < 60000) return "Just now";
  if (ms < 3600000) return `${Math.floor(ms / 60000)}m ago`;
  if (ms < 86400000) return `${Math.floor(ms / 3600000)}h ago`;
  return `${Math.floor(ms / 86400000)}d ago`;
}
export function unifiedRows(diff: string) {
  let old = 0,
    newLine = 0;
  return diff
    .split("\n")
    .filter((text, i, all) => text !== "" || i < all.length - 1)
    .map((text) => {
      const hunk = text.match(/^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/);
      if (hunk) {
        old = Number(hunk[1]);
        newLine = Number(hunk[2]);
        return {
          text,
          kind: "hunk",
          oldLine: undefined as number | undefined,
          newLine: undefined as number | undefined,
        };
      }
      if (
        text.startsWith("---") ||
        text.startsWith("+++") ||
        text.startsWith("diff ") ||
        text.startsWith("index ") ||
        text.startsWith("\\")
      )
        return {
          text,
          kind: "metadata",
          oldLine: undefined,
          newLine: undefined,
        };
      if (text.startsWith("-"))
        return { text, kind: "removed", oldLine: old++, newLine: undefined };
      if (text.startsWith("+"))
        return { text, kind: "added", oldLine: undefined, newLine: newLine++ };
      if (text.startsWith(" "))
        return { text, kind: "context", oldLine: old++, newLine: newLine++ };
      return { text, kind: "metadata", oldLine: undefined, newLine: undefined };
    });
}

export function fuzzyScore(query: string, text: string): number | null {
  const needle = query.toLowerCase().replace(/[^a-z0-9]/g, "");
  const haystack = text.toLowerCase().replace(/[^a-z0-9]/g, "");
  if (!needle) return 0;
  let last = -1,
    score = 0;
  for (const character of needle) {
    const at = haystack.indexOf(character, last + 1);
    if (at < 0) return null;
    score += at === last + 1 ? 12 : 2;
    score -= Math.max(0, at - last - 1) * 0.2;
    last = at;
  }
  if (haystack.includes(needle)) score += 100;
  if (haystack.startsWith(needle)) score += 30;
  return score;
}

export const brokerQueueOrder = [
  "jobs.dispatch",
  "logs.stream",
  "healer.trigger",
  "healer.result",
  "healer.dead",
];
export function orderQueueSnapshot(snapshot: Queue[]): Queue[] {
  return [
    ...brokerQueueOrder.map(
      (name) =>
        snapshot.find((queue) => queue.name === name) ?? {
          name,
          status: "UNREPORTED",
        },
    ),
    ...snapshot.filter((queue) => !brokerQueueOrder.includes(queue.name)),
  ];
}

/** RabbitMQ notifications carry HealResult.patch_id; REST snapshots use Patch.id. */
export function normalizeLivePatch(value: unknown): Patch | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value))
    return undefined;
  const data = value as Record<string, unknown>;
  const direct =
    typeof data.id === "string" && data.id.trim() ? data.id : undefined;
  const alias =
    typeof data.patch_id === "string" && data.patch_id.trim()
      ? data.patch_id
      : undefined;
  const id = direct ?? alias;
  if (!id || (direct && alias && direct !== alias)) return undefined;
  if (!["VERIFIED", "REJECTED"].includes(String(data.verification_status)))
    return undefined;
  for (const field of [
    "pipeline_id",
    "step_id",
    "file_path",
    "bug_category",
    "root_cause_analysis",
    "ast_node_summary",
    "original_code",
    "patched_code",
    "unified_diff",
    "test_output",
  ])
    if (typeof data[field] !== "string") return undefined;
  if (
    typeof data.confidence_score !== "number" ||
    !Number.isFinite(data.confidence_score) ||
    data.confidence_score < 0 ||
    data.confidence_score > 1
  )
    return undefined;
  if (
    data.selected_candidate_id != null &&
    typeof data.selected_candidate_id !== "string"
  )
    return undefined;
  if (
    data.diagnostics != null &&
    (typeof data.diagnostics !== "object" || Array.isArray(data.diagnostics))
  )
    return undefined;
  return { ...data, id } as unknown as Patch;
}
