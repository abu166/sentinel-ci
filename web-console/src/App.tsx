import WorkerTelemetryPanel from "./components/telemetry/WorkerTelemetryPanel";
import {
  observeWorkerSamples,
  WorkerHistory,
} from "./components/telemetry/series";
import OverviewPage from "./pages/OverviewPage";
import HealingStudioPage from "./pages/HealingStudioPage";
import {
  WorkspaceSidebar,
  WorkspaceCommandBar,
  TelemetryRibbon,
  workspaceRoutes,
} from "./components/layout/WorkspaceChrome";
import RepositoryEditor from "./pages/RepositoryEditor";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  Activity,
  ArrowRight,
  Box,
  Check,
  Command,
  Cpu,
  GitBranch,
  GitPullRequest,
  Layers3,
  LayoutDashboard,
  LockKeyhole,
  Play,
  Plus,
  RefreshCw,
  Search,
  ShieldCheck,
  Sparkles,
  Terminal as TerminalIcon,
  Workflow,
  X,
} from "lucide-react";
import {
  Badge,
  Commit,
  DAG,
  RepairPane,
  Select,
  Sparkline,
  Terminal,
} from "./components";
import {
  collection,
  normalizeLivePatch,
  orderQueueSnapshot,
  fuzzyScore,
  duration,
  getToken,
  Log,
  memory,
  Metrics,
  Patch,
  Pipeline,
  previewLogs,
  previewMetrics,
  previewPatch,
  previewPipelines,
  previewQueues,
  previewRepos,
  previewWorkers,
  Pull,
  Queue,
  Repository,
  request,
  Role,
  relativeAge,
  Sample,
  scenarios,
  sandboxPolicy,
  setToken,
  sha,
  topologicalSteps,
  Worker,
} from "./model";
const routes = workspaceRoutes;
function currentRoute() {
  return routes.some((r) => r.path === location.pathname)
    ? location.pathname
    : "/overview";
}
export default function App() {
  const [route, setRoute] = useState(currentRoute),
    [pipelines, setPipelines] = useState(previewPipelines),
    [selected, setSelected] = useState(previewPipelines[0].id),
    [detail, setDetail] = useState<Pipeline>({
      ...previewPipelines[0],
      logs: previewLogs,
      patches: [previewPatch],
    }),
    [patches, setPatches] = useState<Patch[]>([previewPatch]),
    [patch, setPatch] = useState<Patch | undefined>(previewPatch),
    [logs, setLogs] = useState(previewLogs),
    [metrics, setMetrics] = useState(previewMetrics),
    [repos, setRepos] = useState(previewRepos),
    [workers, setWorkers] = useState(previewWorkers),
    [queues, setQueues] = useState(previewQueues),
    [outbox, setOutbox] = useState<Record<string, number>>({}),
    [pulls, setPulls] = useState<Pull[]>([]);
  const [offline, setOffline] = useState(true),
    [role, setRole] = useState<Role>("viewer"),
    [authenticated, setAuthenticated] = useState(false),
    [busy, setBusy] = useState(false),
    [notice, setNotice] = useState(""),
    [filter, setFilter] = useState(""),
    [branch, setBranch] = useState(""),
    [status, setStatus] = useState(""),
    [author, setAuthor] = useState(""),
    [scenario, setScenario] = useState("off-by-one"),
    [selectedStep, setSelectedStep] = useState(""),
    [socketState, setSocketState] = useState("preview"),
    [sourceLocation, setSourceLocation] = useState<{
      file: string;
      line: number;
    }>(),
    [palette, setPalette] = useState(false),
    [paletteSearch, setPaletteSearch] = useState(""),
    [paletteIndex, setPaletteIndex] = useState(0),
    [loginOpen, setLoginOpen] = useState(false),
    [username, setUsername] = useState(""),
    [password, setPassword] = useState(""),
    [registerOpen, setRegisterOpen] = useState(false),
    [newRepoName, setNewRepoName] = useState(""),
    [newRepoURL, setNewRepoURL] = useState("");
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [matrixInspect, setMatrixInspect] = useState(false);
  const [queueSnapshotLoaded, setQueueSnapshotLoaded] = useState(false);
  const [workerHistory, setWorkerHistory] = useState<WorkerHistory>(
    () => new Map(),
  );
  const loaded = useRef(false);
  const observedSamples = useRef<Sample[]>([]);
  const paletteLauncher = useRef<HTMLElement | null>(null);
  const openPalette = useCallback(() => {
    paletteLauncher.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    setPaletteSearch("");
    setPaletteIndex(0);
    setPalette(true);
  }, []);
  useEffect(() => {
    if (!palette) return;
    return () => {
      paletteLauncher.current?.focus();
    };
  }, [palette]);

  const canWrite = authenticated && role !== "viewer" && !offline;
  const navigate = useCallback(
    (path: string, preserveSelection = false) => {
      if (
        path === "/healing-studio" &&
        !preserveSelection &&
        !detail.patches?.length &&
        !["HEALED", "FAILED", "HEALING"].includes(detail.status)
      ) {
        const recent = patches.find(
          (p) =>
            p.verification_status === "VERIFIED" &&
            pipelines.some((run) => run.id === p.pipeline_id),
        );
        if (recent) {
          setSelected(recent.pipeline_id);
          setSelectedStep("");
          setPatch(recent);
        }
      }
      history.pushState({}, "", path);
      setRoute(path);
      setPalette(false);
      setFilter("");
    },
    [detail.patches, detail.status, patches, pipelines],
  );
  useEffect(() => {
    if (location.pathname === "/") history.replaceState({}, "", "/overview");
    const pop = () => setRoute(currentRoute());
    window.addEventListener("popstate", pop);
    return () => window.removeEventListener("popstate", pop);
  }, []);
  const refresh = useCallback(async () => {
    try {
      const [p, m] = await Promise.all([
        request<unknown>("/pipelines"),
        request<Metrics & Record<string, unknown>>("/metrics/overview"),
      ]);
      const rows = collection<Pipeline>(p, "pipelines");
      setPipelines(rows);
      const rawSeries = Array.isArray(m.timeseries) ? m.timeseries : [];
      observedSamples.current = [
        ...observedSamples.current,
        {
          timestamp: new Date().toISOString(),
          queue_depth: m.queue_depth,
          cpu_percent: m.worker_cpu_percent,
          memory_bytes: m.worker_memory_bytes,
        },
      ].slice(-36);
      setMetrics({
        ...m,
        p95_build_duration_ms:
          m.p95_build_duration_ms ??
          Number(m.p95_build_duration_seconds ?? m.p95_build_seconds ?? 0) *
            1000,
        timeseries: [
          ...rawSeries.map((sample) => ({
            ...sample,
            build_duration_ms:
              sample.build_duration_ms ??
              Number(
                (sample as unknown as Record<string, unknown>)
                  .build_duration_seconds ?? 0,
              ) * 1000,
          })),
          ...observedSamples.current,
        ],
      });
      setOffline(false);
      setSelected((old) =>
        rows.some((x) => x.id === old) ? old : (rows[0]?.id ?? ""),
      );
      const secondary = await Promise.allSettled([
        request<unknown>("/repos"),
        request<unknown>("/workers"),
        request<unknown>("/queue"),
        request<unknown>("/patches"),
        request<unknown>("/pulls"),
      ]);
      if (secondary[0].status === "fulfilled")
        setRepos(collection<Repository>(secondary[0].value, "repositories"));
      if (secondary[1].status === "fulfilled") {
        const liveWorkers = collection<
          Worker & { host?: string; heartbeat?: string }
        >(secondary[1].value, "workers").map((w) => ({
          ...w,
          hostname: w.hostname ?? w.host,
          heartbeat_at: w.heartbeat_at ?? w.heartbeat ?? w.last_heartbeat,
        }));
        setWorkers(liveWorkers);
        setWorkerHistory((previous) =>
          observeWorkerSamples(previous, liveWorkers, Date.now()),
        );
      }
      if (secondary[2].status === "fulfilled") {
        const data = secondary[2].value as { outbox?: Record<string, number> };
        setQueues(orderQueueSnapshot(collection<Queue>(data, "queues")));
        setQueueSnapshotLoaded(true);
        setOutbox(data.outbox ?? {});
      } else {
        setQueues(orderQueueSnapshot([]));
        setQueueSnapshotLoaded(true);
      }
      if (secondary[3].status === "fulfilled")
        setPatches(collection<Patch>(secondary[3].value, "patches"));
      if (secondary[4].status === "fulfilled")
        setPulls(collection<Pull>(secondary[4].value, "pulls"));
    } catch (error) {
      setOffline(true);
      if (loaded.current)
        setNotice(
          `Connection interrupted: ${String(error)}. Showing last observed data.`,
        );
    } finally {
      loaded.current = true;
    }
  }, []);
  async function login(
    next: Role,
    credentials?: { username: string; password: string },
  ) {
    setBusy(true);
    try {
      const result = await request<{ token: string; role: Role }>(
        "/auth/login",
        {
          method: "POST",
          body: JSON.stringify(credentials ?? { demo: true, role: next }),
        },
      );
      setToken(result.token);
      setRole(result.role);
      setAuthenticated(true);
      setLoginOpen(false);
      setNotice("");
      await refresh();
    } catch (error) {
      setNotice(`Session unavailable: ${String(error)}`);
      if (!credentials && /401|403/.test(String(error))) setLoginOpen(true);
    } finally {
      setQueueSnapshotLoaded(true);
      setBusy(false);
    }
  }
  useEffect(() => {
    void login("admin");
    return () => setToken("");
  }, []);
  useEffect(() => {
    if (!authenticated) return;
    const timer = setInterval(() => void refresh(), 5000);
    return () => clearInterval(timer);
  }, [authenticated, refresh]);
  useEffect(() => {
    if (offline || !selected) return;
    let active = true;
    const load = async () => {
      try {
        const d = await request<Pipeline>(`/pipelines/${selected}`);
        if (!active) return;
        setDetail(d);
        setPatch(d.patches?.[0]);
        if (d.logs)
          setLogs((previous) => {
            const keys = new Set(
              d.logs?.map((l) => `${l.step_id}:${l.sequence_num}`),
            );
            return [
              ...(d.logs ?? []),
              ...previous.filter(
                (l) => !keys.has(`${l.step_id}:${l.sequence_num}`),
              ),
            ].slice(-8000);
          });
      } catch (error) {
        if (active) setNotice(String(error));
      }
    };
    setLogs([]);
    void load();
    const timer = setInterval(() => void load(), 2000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [selected, offline]);
  useEffect(() => {
    if (offline || !selected || !authenticated) return;
    let stopped = false,
      socket: WebSocket | undefined,
      retry: ReturnType<typeof setTimeout>;
    const connect = () => {
      setSocketState("connecting");
      socket = new WebSocket(
        `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/ws/pipelines/${selected}/logs?token=${encodeURIComponent(getToken())}`,
      );
      socket.onopen = () => setSocketState("live");
      socket.onmessage = (e) => {
        try {
          const event = JSON.parse(String(e.data)) as Log;
          if (event.event === "healer.result" && event.patch) {
            const livePatch = normalizeLivePatch(event.patch);
            if (livePatch) setPatch(livePatch);
            void refresh();
          }
          if (typeof event.content !== "string") return;
          event.sequence_num = Number.isFinite(event.sequence_num)
            ? event.sequence_num
            : Date.now();
          setLogs((old) =>
            old.some(
              (l) =>
                l.step_id === event.step_id &&
                l.sequence_num === event.sequence_num,
            )
              ? old
              : [...old, event].slice(-8000),
          );
        } catch {
          setNotice("Malformed log event ignored.");
        }
      };
      socket.onerror = () => socket?.close();
      socket.onclose = () => {
        if (!stopped) {
          setSocketState("reconnecting");
          retry = setTimeout(connect, 2000);
        }
      };
    };
    connect();
    return () => {
      stopped = true;
      clearTimeout(retry);
      socket?.close();
    };
  }, [selected, offline, authenticated, role, refresh]);
  function selectPipeline(p: Pipeline) {
    setSelected(p.id);
    setSelectedStep("");
    setSourceLocation(undefined);
    setDetail(p);
    if (offline) {
      setLogs(previewLogs);
      setPatch(p.status === "HEALED" ? previewPatch : undefined);
    } else {
      setLogs([]);
      setPatch(p.patches?.[0]);
    }
  }
  async function simulate(id = scenario) {
    if (!canWrite) {
      setNotice("An authenticated Admin or Maintainer session is required.");
      return;
    }
    setBusy(true);
    try {
      const result = await request<{ id: string; pipeline_id?: string }>(
        `/pipelines/${selected}/simulate-failure`,
        { method: "POST", body: JSON.stringify({ scenario: id }) },
      );
      setSelected(result.pipeline_id ?? result.id);
      setSelectedStep("");
      navigate("/healing-studio", true);
      setNotice(
        `${scenarios.find((s) => s.id === id)?.name ?? id} queued for isolated execution.`,
      );
      await refresh();
    } catch (error) {
      setNotice(String(error));
    } finally {
      setBusy(false);
    }
  }
  async function patchAction(action: string) {
    if (!patch || !canWrite) return;
    setBusy(true);
    try {
      const result = await request<{
        pipeline_id?: string;
        id?: string;
        url?: string;
      }>(`/patches/${patch.id}/${action === "race" ? "verify" : action}`, {
        method: "POST",
        body: JSON.stringify(
          action === "verify" || action === "race"
            ? { race: action === "race" }
            : {},
        ),
      });
      setNotice(
        action === "publish"
          ? `Git branch and pull request recorded${result.id ? `: ${result.id.slice(0, 8)}` : ""}.`
          : action === "reject"
            ? "Patch rejected."
            : action === "verify" || action === "race"
              ? action === "race"
                ? "Race-detector verification queued."
                : "Regression verification queued."
              : "Patch approved and patched execution queued.",
      );
      if (action === "reject")
        setPatch((p) => (p ? { ...p, approval_status: "REJECTED" } : p));
      if (result.pipeline_id) {
        setSelected(result.pipeline_id);
        setSelectedStep("");
      }
      await refresh();
    } catch (error) {
      setNotice(String(error));
    } finally {
      setBusy(false);
    }
  }
  const commands = [
    ...routes.map((r) => ({
      group: "Navigation",
      label: `Go to ${r.name}`,
      hint: r.path,
      run: () => navigate(r.path),
    })),
    ...scenarios.map((s) => ({
      group: "Repair scenarios",
      label: `Simulate ${s.name}`,
      hint: s.repo,
      run: () => {
        setPalette(false);
        setScenario(s.id);
        void simulate(s.id);
      },
    })),
    ...[
      { id: "deadlock", name: "Deadlock" },
      { id: "sql-injection", name: "SQL Injection" },
    ].map((extra) => ({
      group: "Repair scenarios",
      label: `Simulate ${extra.name}`,
      hint: "Extended repair scenario",
      run: () => {
        setPalette(false);
        void simulate(extra.id);
      },
    })),
    ...pipelines.map((p) => ({
      group: "Pipelines",
      label: `${p.repo_name ?? p.repo_id} · ${sha(p.commit_sha)}`,
      hint: p.branch,
      run: () => {
        selectPipeline(p);
        navigate("/healing-studio");
      },
    })),
  ]
    .map((command) => ({
      ...command,
      score: fuzzyScore(paletteSearch, `${command.label} ${command.hint}`),
    }))
    .filter((command) => command.score !== null)
    .sort(
      (a, b) =>
        ["Navigation", "Repair scenarios", "Pipelines"].indexOf(a.group) -
          ["Navigation", "Repair scenarios", "Pipelines"].indexOf(b.group) ||
        (b.score ?? 0) - (a.score ?? 0),
    );
  useEffect(() => {
    if (palette)
      document
        .querySelector<HTMLElement>(`[data-command-index="${paletteIndex}"]`)
        ?.scrollIntoView?.({ block: "nearest" });
  }, [palette, paletteIndex]);
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        if (palette) setPalette(false);
        else openPalette();
      }
      if (e.ctrlKey && e.key.toLowerCase() === "b") {
        e.preventDefault();
        setSidebarCollapsed((v) => !v);
      }
      if (e.key === "Escape") {
        setPalette(false);
        setLoginOpen(false);
      }
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, [palette, openPalette]);
  function onLocation(file: string, line: number) {
    setSourceLocation({ file, line });
    if (
      !patch ||
      (!patch.file_path.endsWith(file) && !file.endsWith(patch.file_path))
    ) {
      const found = patches.find(
        (p) =>
          p.pipeline_id === selected &&
          (p.file_path.endsWith(file) || file.endsWith(p.file_path)),
      );
      if (found) setPatch(found);
    }
    navigate("/healing-studio");
  }
  const visible = pipelines.filter(
    (p) =>
      `${p.repo_name} ${p.branch} ${p.commit_sha} ${p.commit_message}`
        .toLowerCase()
        .includes(filter.toLowerCase()) &&
      (!branch || p.branch === branch) &&
      (!status || p.status === status) &&
      (!author || p.triggered_by === author),
  );
  const steps = topologicalSteps(detail.steps ?? []);
  const evidencePatch = detail.patches?.[0];
  let tail = steps.at(-1)?.id;
  for (const phase of [
    {
      id: "ast",
      name: "AST Fault Localizer",
      stage: "heal",
      status: evidencePatch
        ? "HEALED"
        : detail.status === "HEALING"
          ? "RUNNING"
          : "PENDING",
    },
    {
      id: "verify",
      name: "Sandbox Patch Verify",
      stage: "verify",
      status:
        evidencePatch?.verification_status === "VERIFIED"
          ? "PASSED"
          : evidencePatch?.verification_status === "REJECTED"
            ? "FAILED"
            : "PENDING",
    },
    {
      id: "publish",
      name: "PR Publisher",
      stage: "publish",
      status: pulls.some((p) => p.pipeline_id === detail.id)
        ? "PASSED"
        : "PENDING",
    },
  ]) {
    if (
      !steps.some((s) => s.stage === phase.stage || s.name === phase.name) &&
      tail
    ) {
      const id = `${detail.id}:${phase.id}`;
      steps.push({
        id,
        name: phase.name,
        stage: phase.stage,
        command: phase.name,
        status: phase.status as Pipeline["status"],
        depends_on: [tail],
      });
      tail = id;
    }
  }
  const title = routes.find((r) => r.path === route)?.name ?? "Overview";
  const selectedPipeline = pipelines.find((p) => p.id === selected) ?? detail;
  const pullsView = (
    <div className="pulls-list">
      {pulls.length ? (
        pulls.map((p) => (
          <article key={p.id}>
            <GitPullRequest size={15} />
            <div>
              <strong>
                {p.title ?? p.branch ?? p.head_branch ?? "Repair pull request"}
              </strong>
              <code>
                {p.branch ?? p.head_branch} · {sha(p.commit_sha ?? "")}
              </code>
              <small>
                {p.provider ?? "local git"} · {p.status ?? p.state ?? "OPEN"}
              </small>
              {p.url && /^https?:\/\//.test(p.url) && (
                <a href={p.url} target="_blank" rel="noreferrer">
                  Open pull request ↗
                </a>
              )}
            </div>
          </article>
        ))
      ) : (
        <div className="empty">No pull requests have been published.</div>
      )}
    </div>
  );
  const workbench = (
    <div className="ide-workbench">
      <div className="execution-strip">
        <span className="execution-context">
          <GitBranch size={12} />
          <code>{detail.branch}</code>
          <Commit value={detail.commit_sha} />
          <Badge status={detail.status} />
        </span>
        <span className="execution-author">
          <code>{detail.triggered_by}</code>
          <button
            onClick={() => {
              setSelectedStep("");
              setPatch(detail.patches?.[0]);
            }}
          >
            all steps
          </button>
        </span>
      </div>
      <DAG
        steps={steps}
        selected={selectedStep}
        onSelect={(s) => {
          setSelectedStep(s.id);
          const found =
            detail.patches?.find((p) => p.step_id === s.id) ??
            (["heal", "verify", "publish"].includes(s.stage)
              ? detail.patches?.[0]
              : undefined);
          setPatch(found);
        }}
      />
      <div className="ide-bottom">
        <Terminal
          logs={logs}
          selectedStep={
            selectedStep.includes(":") ? (patch?.step_id ?? "") : selectedStep
          }
          onLocation={onLocation}
          state={offline ? "offline preview" : socketState}
        />
        <RepairPane
          patch={patch}
          location={sourceLocation}
          onAction={(a) => void patchAction(a)}
          disabled={!canWrite}
          busy={busy}
          pulls={pullsView}
          published={pulls.some((p) => p.pipeline_id === patch?.pipeline_id)}
          onSourceLocation={onLocation}
          onPatchSelected={(next) => {
            setPatch(next);
            setPatches((old) => old.map((p) => (p.id === next.id ? next : p)));
            setDetail((old) => ({
              ...old,
              patches: old.patches?.map((p) => (p.id === next.id ? next : p)),
            }));
          }}
        />
      </div>
    </div>
  );
  return (
    <div
      className={`workbench-app ${sidebarCollapsed ? "sidebar-collapsed" : ""}`}
    >
      <WorkspaceSidebar
        route={route}
        collapsed={sidebarCollapsed}
        onCollapse={() => setSidebarCollapsed((v) => !v)}
        onNavigate={navigate}
        onCommands={openPalette}
        offline={offline}
        workers={workers.length}
        queue={metrics.queue_depth}
        patchCount={
          patches.filter((p) => p.verification_status === "VERIFIED").length
        }
      />
      <main>
        <WorkspaceCommandBar
          title={title}
          offline={offline}
          role={role}
          busy={busy}
          canWrite={canWrite && !!selected}
          scenario={scenario}
          onScenario={setScenario}
          onSimulate={() => void simulate()}
          onCommands={openPalette}
          onLogin={(next) => void login(next)}
          onSession={() => setLoginOpen(true)}
        />
        <TelemetryRibbon metrics={metrics} offline={offline} />
        {notice && (
          <div className="notice" role="status">
            <Activity size={12} />
            <span>{notice}</span>
            <button aria-label="Dismiss notice" onClick={() => setNotice("")}>
              <X size={13} />
            </button>
          </div>
        )}
        <div className={`route-view ${route.slice(1)}`}>
          {route === "/overview" && (
            <OverviewPage
              visible={visible}
              selected={selected}
              total={pipelines.length}
              offline={offline}
              filter={filter}
              onFilter={setFilter}
              onSelect={selectPipeline}
              selectedName={selectedPipeline.repo_name ?? "Execution detail"}
              onOpenStudio={() => navigate("/healing-studio")}
            >
              {workbench}
            </OverviewPage>
          )}
          {route === "/healing-studio" && (
            <HealingStudioPage
              pipelines={pipelines}
              selected={selected}
              onSelect={selectPipeline}
            >
              {workbench}
            </HealingStudioPage>
          )}
          {route === "/pipelines" && (
            <div className="matrix-layout">
              <div className="view-heading">
                <div>
                  <h1>Execution matrix</h1>
                  <span>{visible.length} matching runs</span>
                </div>
                <label className="inline-search">
                  <Search size={12} />
                  <input
                    aria-label="Search execution matrix"
                    placeholder="Search commit or repository…"
                    value={filter}
                    onChange={(e) => setFilter(e.target.value)}
                  />
                </label>
              </div>
              <div className="filters">
                <Select
                  label="Branch"
                  value={branch}
                  onChange={setBranch}
                  options={[
                    { value: "", label: "All branches" },
                    ...Array.from(new Set(pipelines.map((p) => p.branch))).map(
                      (v) => ({ value: v, label: v }),
                    ),
                  ]}
                />
                <Select
                  label="Status"
                  value={status}
                  onChange={setStatus}
                  options={[
                    { value: "", label: "All statuses" },
                    ...[
                      "PASSED",
                      "HEALED",
                      "FAILED",
                      "RUNNING",
                      "HEALING",
                      "PENDING",
                    ].map((v) => ({ value: v, label: v })),
                  ]}
                />
                <Select
                  label="Author"
                  value={author}
                  onChange={setAuthor}
                  options={[
                    { value: "", label: "All authors" },
                    ...Array.from(
                      new Set(pipelines.map((p) => p.triggered_by)),
                    ).map((v) => ({ value: v, label: v })),
                  ]}
                />
                <span className="muted">
                  Step width reflects recorded duration
                </span>
              </div>
              <div
                className="status-filter-pills"
                aria-label="Quick status filters"
              >
                {[
                  { id: "", name: "All runs" },
                  { id: "FAILED", name: "Failed" },
                  { id: "RUNNING", name: "Running" },
                  { id: "HEALED", name: "Healed" },
                  { id: "PASSED", name: "Passed" },
                ].map((item) => (
                  <button
                    key={item.id}
                    className={status === item.id ? "active" : ""}
                    aria-pressed={status === item.id}
                    onClick={() => setStatus(item.id)}
                  >
                    <i className={`filter-dot ${item.id.toLowerCase()}`} />
                    {item.name}
                    <span>
                      {
                        pipelines.filter(
                          (p) => !item.id || p.status === item.id,
                        ).length
                      }
                    </span>
                  </button>
                ))}
              </div>
              <div className="matrix-scroll">
                <table className="execution-matrix">
                  <colgroup>
                    <col className="matrix-repo-col" />
                    <col className="matrix-commit-col" />
                    <col className="matrix-author-col" />
                    <col className="matrix-status-col" />
                    <col />
                    <col className="matrix-duration-col" />
                    <col className="matrix-inspect-col" />
                  </colgroup>
                  <thead>
                    <tr>
                      <th>Execution / repository</th>
                      <th>Commit</th>
                      <th>Author</th>
                      <th>Status</th>
                      <th>Step waterfall</th>
                      <th>Duration</th>
                      <th />
                    </tr>
                  </thead>
                  <tbody>
                    {visible.map((p) => {
                      const ss = p.steps ?? (p.id === detail.id ? steps : []),
                        total = ss.reduce(
                          (n, s) => n + (s.duration_ms ?? 0),
                          0,
                        );
                      return (
                        <tr
                          key={p.id}
                          className={p.id === selected ? "selected" : ""}
                        >
                          <td>
                            <strong>
                              {p.repo_name ?? p.repo_id.slice(0, 8)}
                            </strong>
                            <code>{p.branch}</code>
                          </td>
                          <td>
                            <span className="commit-message">
                              {p.commit_message ?? "Commit execution"}
                            </span>
                            <Commit value={p.commit_sha} />
                          </td>
                          <td>
                            <code>{p.triggered_by}</code>
                          </td>
                          <td>
                            <Badge status={p.status} />
                          </td>
                          <td>
                            <div className="waterfall">
                              {ss.map((s) => (
                                <button
                                  key={s.id}
                                  style={{
                                    flexGrow: Math.max(1, s.duration_ms ?? 1),
                                  }}
                                  className={s.status.toLowerCase()}
                                  title={`${s.name} · ${duration(s.duration_ms)} · exit ${s.exit_code ?? "—"}`}
                                  onClick={() => {
                                    selectPipeline(p);
                                    setSelectedStep(s.id);
                                    setMatrixInspect(true);
                                  }}
                                >
                                  <span>{s.name}</span>
                                </button>
                              ))}
                              {!ss.length && (
                                <span className="muted">
                                  select to load step timings
                                </span>
                              )}
                            </div>
                          </td>
                          <td>
                            <code>
                              {total
                                ? duration(total)
                                : p.started_at && p.finished_at
                                  ? duration(
                                      new Date(p.finished_at).getTime() -
                                        new Date(p.started_at).getTime(),
                                    )
                                  : "—"}
                            </code>
                          </td>
                          <td>
                            <button
                              aria-label={`Inspect ${sha(p.commit_sha)} execution`}
                              onClick={() => {
                                selectPipeline(p);
                                setMatrixInspect(true);
                              }}
                            >
                              <TerminalIcon size={12} />
                            </button>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
              {matrixInspect && (
                <aside
                  className="matrix-inspector"
                  role="complementary"
                  aria-label="Execution inspection drawer"
                >
                  <div className="drawer-heading">
                    <div>
                      <TerminalIcon size={15} />
                      <strong>Execution inspector</strong>
                      <Commit value={detail.commit_sha} />
                    </div>
                    <button
                      aria-label="Close execution inspector"
                      onClick={() => setMatrixInspect(false)}
                    >
                      <X size={15} />
                    </button>
                  </div>
                  <Terminal
                    logs={logs}
                    selectedStep={
                      selectedStep.includes(":")
                        ? (patch?.step_id ?? "")
                        : selectedStep
                    }
                    onLocation={onLocation}
                    state={offline ? "offline preview" : socketState}
                  />
                </aside>
              )}
            </div>
          )}
          {route === "/repositories" && (
            <div className="registry-layout">
              <div className="view-heading">
                <div>
                  <h1>Repository registry</h1>
                  <span>
                    {repos.length} connected repositories · webhook and repair
                    policies
                  </span>
                </div>
                <button
                  disabled={!canWrite}
                  onClick={() => setRegisterOpen(true)}
                >
                  <Plus size={12} />
                  Register repository
                </button>
              </div>
              <div className="repository-grid">
                {repos.map((r) => (
                  <RepositoryEditor
                    key={r.id}
                    repo={r}
                    disabled={!canWrite || busy}
                    onSave={async (values) => {
                      setBusy(true);
                      try {
                        await request(`/repos/${r.id}`, {
                          method: "PATCH",
                          body: JSON.stringify(values),
                        });
                        setNotice(`${r.name} configuration saved.`);
                        await refresh();
                      } catch (error) {
                        setNotice(String(error));
                        throw error;
                      } finally {
                        setBusy(false);
                      }
                    }}
                  />
                ))}
              </div>
            </div>
          )}
          {route === "/workers" && (
            <div className="infrastructure-layout">
              <div className="view-heading">
                <div>
                  <h1>Worker nodes</h1>
                  <span>
                    Individual runner routines · measured resource use ·
                    heartbeat freshness
                  </span>
                </div>
                <span className="policy-label">
                  <ShieldCheck size={13} />
                  Per-step sandbox policy
                </span>
              </div>
              <div className="infra-summary">
                <span>
                  <Box size={16} />
                  <b>{workers.length}</b>routines online
                </span>
                <span>
                  <Cpu size={16} />
                  <b>
                    {metrics.worker_cpu_percent == null
                      ? "—"
                      : `${metrics.worker_cpu_percent.toFixed(1)}%`}
                  </b>
                  CPU utilization
                </span>
                <span>
                  <Layers3 size={16} />
                  <b>{memory(metrics.worker_memory_bytes)}</b>resident memory
                </span>
              </div>
              <WorkerTelemetryPanel history={workerHistory} offline={offline} />
              <div
                className="worker-table-region"
                role="region"
                aria-label="Worker routine telemetry"
                tabIndex={0}
              >
                <table className="worker-table">
                  <colgroup>
                    <col style={{ width: 180 }} />
                    <col style={{ width: 190 }} />
                    <col style={{ width: 90 }} />
                    <col style={{ width: 100 }} />
                    <col style={{ width: 150 }} />
                    <col />
                  </colgroup>
                  <thead>
                    <tr>
                      <th>Worker / routine</th>
                      <th>Heartbeat</th>
                      <th>State</th>
                      <th>CPU</th>
                      <th>RAM / watermark</th>
                      <th>Sandbox security policy</th>
                    </tr>
                  </thead>
                  <tbody>
                    {workers.map((w, i) => (
                      <tr key={w.id ?? w.worker_id ?? i}>
                        <td>
                          <strong>
                            {w.hostname ??
                              w.id ??
                              w.worker_id ??
                              `routine-${i}`}
                          </strong>
                          <code>routine {w.routine ?? i}</code>
                        </td>
                        <td>
                          <code>
                            {w.heartbeat_at ??
                              w.last_heartbeat ??
                              "not reported"}
                          </code>
                        </td>
                        <td>
                          <Badge status={w.status ?? "ONLINE"} />
                        </td>
                        <td>
                          <code>
                            {w.cpu_percent == null
                              ? "—"
                              : `${w.cpu_percent.toFixed(1)}%`}
                          </code>
                          <div className="usage-track">
                            <i
                              style={{
                                width: `${Math.min(100, w.cpu_percent ?? 0)}%`,
                              }}
                            />
                          </div>
                        </td>
                        <td>
                          <code>{memory(w.memory_bytes)}</code>
                          <small>peak {memory(w.peak_rss_bytes)}</small>
                        </td>
                        <td>
                          <span className="security-state">
                            <ShieldCheck size={12} />
                            {w.sandbox_status ?? "not reported"}
                          </span>
                          <code>{sandboxPolicy(w.sandbox_policy)}</code>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <div className="section-heading">
                <h2>Selected execution resource trace</h2>
                <Commit value={detail.commit_sha} />
              </div>
              <div
                className="resource-trace-region"
                role="region"
                aria-label="Step resource telemetry"
                tabIndex={0}
              >
                <table className="resource-trace-table">
                  <colgroup>
                    <col style={{ width: 180 }} />
                    <col style={{ width: 100 }} />
                    <col style={{ width: 130 }} />
                    <col style={{ width: 110 }} />
                    <col style={{ width: 110 }} />
                    <col />
                  </colgroup>
                  <thead>
                    <tr>
                      <th>Step</th>
                      <th>Wall time</th>
                      <th>Peak RSS</th>
                      <th>User CPU</th>
                      <th>System CPU</th>
                      <th>Sandbox</th>
                    </tr>
                  </thead>
                  <tbody>
                    {steps.map((s) => (
                      <tr key={s.id}>
                        <td>{s.name}</td>
                        <td>
                          <code>{duration(s.duration_ms)}</code>
                        </td>
                        <td>
                          <code>
                            {memory(s.peak_rss_bytes ?? s.memory_peak_bytes)}
                          </code>
                        </td>
                        <td>
                          <code>
                            {s.cpu_user_seconds != null
                              ? `${s.cpu_user_seconds.toFixed(3)}s`
                              : s.cpu_user_ms != null
                                ? `${(s.cpu_user_ms / 1000).toFixed(3)}s`
                                : "—"}
                          </code>
                        </td>
                        <td>
                          <code>
                            {s.cpu_system_seconds != null
                              ? `${s.cpu_system_seconds.toFixed(3)}s`
                              : s.cpu_system_ms != null
                                ? `${(s.cpu_system_ms / 1000).toFixed(3)}s`
                                : "—"}
                          </code>
                        </td>
                        <td>
                          <code>
                            {sandboxPolicy(
                              s.sandbox_policy ?? s.sandbox_status,
                            )}
                          </code>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
          {route === "/queue" && (
            <div className="infrastructure-layout">
              <div className="view-heading">
                <div>
                  <h1>Job queue</h1>
                  <span>
                    RabbitMQ delivery topology · transactional outbox · retries
                    and quarantine
                  </span>
                </div>
                <code className="exchange">exchange: sentinel / topic</code>
              </div>
              <div className="queue-topology">
                <div className="topology-node">
                  Gateway transaction<small>pipeline + outbox event</small>
                </div>
                <ArrowRight size={17} />
                <div className="topology-node">
                  Outbox relay<small>confirmed delivery · backoff</small>
                </div>
                <ArrowRight size={17} />
                <div className="topology-node">
                  RabbitMQ<small>durable topic routing</small>
                </div>
                <ArrowRight size={17} />
                <div className="topology-node">
                  Runner lease<small>idempotent claim</small>
                </div>
              </div>
              <div
                className="queue-table-region"
                role="region"
                aria-label="Queue delivery metrics"
                tabIndex={0}
              >
                <table className="queue-table">
                  <colgroup>
                    <col style={{ width: 250 }} />
                    <col style={{ width: 90 }} />
                    <col style={{ width: 100 }} />
                    <col style={{ width: 110 }} />
                    <col />
                  </colgroup>
                  <thead>
                    <tr>
                      <th>Queue / routing key</th>
                      <th>Ready</th>
                      <th>In flight</th>
                      <th>Consumers</th>
                      <th>Policy</th>
                    </tr>
                  </thead>
                  <tbody>
                    {queues.map((q) => (
                      <tr key={q.name}>
                        <td>
                          <span
                            className={`queue-icon ${q.name.includes("dead") ? "dead" : ""}`}
                          >
                            <Layers3 size={14} />
                          </span>
                          <code>{q.name}</code>
                        </td>
                        <td>
                          <code>
                            {!queueSnapshotLoaded || q.status === "UNREPORTED"
                              ? "—"
                              : (q.messages_ready ?? q.messages ?? 0)}
                          </code>
                        </td>
                        <td>
                          <code>
                            {!queueSnapshotLoaded || q.status === "UNREPORTED"
                              ? "—"
                              : (q.messages_unacknowledged ?? "—")}
                          </code>
                        </td>
                        <td>
                          <code>
                            {!queueSnapshotLoaded || q.status === "UNREPORTED"
                              ? "—"
                              : (q.consumers ?? "—")}
                          </code>
                        </td>
                        <td>
                          {q.name.includes("dead") ? (
                            <span className="amber">
                              Quarantined · operator review required
                            </span>
                          ) : q.name.includes("retry") ? (
                            "Exponential backoff"
                          ) : (
                            "Durable · explicit acknowledgement"
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <div className="section-heading">
                <h2>Transactional outbox</h2>
                <span>Database source of truth</span>
              </div>
              <div className="outbox-counts">
                {["pending", "published", "dead"].map((k) => (
                  <div key={k}>
                    <span>{k.toUpperCase()}</span>
                    <code>{outbox[k] ?? (offline ? 0 : "—")}</code>
                  </div>
                ))}
              </div>
              <div className="queue-note">
                <ShieldCheck size={15} />
                <p>
                  Committed state and dispatch intent share one database
                  transaction. Broker confirmation controls publication status.
                  Dead-lettered events require investigation before replay.
                </p>
              </div>
            </div>
          )}
        </div>
        <footer className="statusbar">
          <span className="statusbar-left">
            <span className="statusbar-connection">
              <i className={`status-dot ${offline ? "" : "green"}`} />
              {offline
                ? "Reference preview · no execution claims"
                : "Gateway connected"}
            </span>
            <span className="divider" />
            <span>RBAC: {role}</span>
          </span>
          <span>
            <Command size={11} />K command palette
            <span className="divider" />
            SentinelCI / workbench v2
          </span>
        </footer>
      </main>
      {palette && (
        <div
          className="modal-backdrop"
          onMouseDown={(e) => {
            if (e.target === e.currentTarget) setPalette(false);
          }}
        >
          <div
            className="command-palette"
            role="dialog"
            aria-modal="true"
            aria-label="Command palette"
            onKeyDown={(e) => {
              if (e.key !== "Tab") return;
              const elements = Array.from(
                e.currentTarget.querySelectorAll<HTMLElement>(
                  'button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])',
                ),
              );
              const first = elements[0],
                last = elements.at(-1);
              if (!first || !last) return;
              if (
                e.shiftKey &&
                (document.activeElement === first ||
                  !e.currentTarget.contains(document.activeElement))
              ) {
                e.preventDefault();
                last.focus();
              } else if (!e.shiftKey && document.activeElement === last) {
                e.preventDefault();
                first.focus();
              }
            }}
          >
            <label>
              <Search size={17} />
              <input
                autoFocus
                placeholder="Navigate, simulate a failure, or switch pipeline…"
                aria-label="Search commands"
                value={paletteSearch}
                onChange={(e) => {
                  setPaletteSearch(e.target.value);
                  setPaletteIndex(0);
                }}
                onKeyDown={(e) => {
                  if (e.key === "ArrowDown") {
                    e.preventDefault();
                    setPaletteIndex((i) =>
                      Math.min(commands.length - 1, i + 1),
                    );
                  }
                  if (e.key === "ArrowUp") {
                    e.preventDefault();
                    setPaletteIndex((i) => Math.max(0, i - 1));
                  }
                  if (e.key === "Enter") {
                    e.preventDefault();
                    commands[paletteIndex]?.run();
                  }
                }}
              />
              <kbd>esc</kbd>
            </label>
            <div className="command-results">
              {commands.map((c, i) => (
                <div
                  key={`${c.group}-${c.label}-${i}`}
                  className="command-result-item"
                >
                  {(i === 0 || commands[i - 1].group !== c.group) && (
                    <div
                      role="heading"
                      aria-level={3}
                      className="command-group-heading"
                    >
                      {c.group}
                    </div>
                  )}
                  <button
                    data-command-index={i}
                    className={i === paletteIndex ? "selected" : ""}
                    key={`${c.label}-${i}`}
                    onMouseEnter={() => setPaletteIndex(i)}
                    onClick={c.run}
                  >
                    <span>{c.label}</span>
                    <code>{c.hint}</code>
                    {i === paletteIndex && <span>↵</span>}
                  </button>
                </div>
              ))}
              {!commands.length && (
                <div className="empty">
                  No matching commands. Available failure scenarios are listed
                  in the simulation selector.
                </div>
              )}
            </div>
            <div className="palette-foot">
              ↑ ↓ navigate <span>↵ select · esc close</span>
            </div>
          </div>
        </div>
      )}
      {registerOpen && (
        <div className="modal-backdrop">
          <form
            className="login-dialog"
            role="dialog"
            aria-modal="true"
            aria-label="Register repository"
            onSubmit={(e) => {
              e.preventDefault();
              setBusy(true);
              void request("/repos", {
                method: "POST",
                body: JSON.stringify({
                  name: newRepoName,
                  repo_url: newRepoURL,
                  default_branch: "main",
                }),
              })
                .then(async () => {
                  setRegisterOpen(false);
                  setNewRepoName("");
                  setNewRepoURL("");
                  setNotice("Repository registered.");
                  await refresh();
                })
                .catch((error) => setNotice(String(error)))
                .finally(() => setBusy(false));
            }}
          >
            <div className="pane-title">
              <span>
                <GitBranch size={14} />
                Register repository
              </span>
              <button
                type="button"
                aria-label="Close repository registration"
                onClick={() => setRegisterOpen(false)}
              >
                <X size={14} />
              </button>
            </div>
            <label>
              Repository name
              <input
                aria-label="Repository name"
                value={newRepoName}
                onChange={(e) => setNewRepoName(e.target.value)}
                required
              />
            </label>
            <label>
              HTTPS Git URL
              <input
                aria-label="Repository Git URL"
                type="url"
                value={newRepoURL}
                onChange={(e) => setNewRepoURL(e.target.value)}
                required
                pattern="https://.*"
              />
            </label>
            <button className="primary" disabled={busy} type="submit">
              Register repository
            </button>
          </form>
        </div>
      )}
      {loginOpen && (
        <div className="modal-backdrop">
          <form
            className="login-dialog"
            role="dialog"
            aria-modal="true"
            aria-label="Session settings"
            onSubmit={(e) => {
              e.preventDefault();
              void login("admin", { username, password });
            }}
          >
            <div className="pane-title">
              <span>
                <LockKeyhole size={14} />
                Session & access
              </span>
              <button
                type="button"
                aria-label="Close session settings"
                onClick={() => setLoginOpen(false)}
              >
                <X size={14} />
              </button>
            </div>
            <h2>Authenticate to SentinelCI</h2>
            <p>
              Demo sessions are available only when enabled by the gateway
              deployment.
            </p>
            <label>
              Username
              <input
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                autoComplete="username"
                required
              />
            </label>
            <label>
              Password
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="current-password"
                required
              />
            </label>
            <button className="primary" type="submit" disabled={busy}>
              Sign in
            </button>
            <div className="demo-login-buttons">
              <button type="button" onClick={() => void login("admin")}>
                Demo Admin
              </button>
              <button type="button" onClick={() => void login("viewer")}>
                Read-only Viewer
              </button>
            </div>
          </form>
        </div>
      )}
    </div>
  );
}
