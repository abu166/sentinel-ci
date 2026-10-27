import { useState, useEffect, useRef } from "react";
import {
  Activity,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Command,
  GitBranch,
  Layers3,
  LayoutDashboard,
  LockKeyhole,
  LogOut,
  PanelLeftClose,
  Play,
  Search,
  ShieldCheck,
  Sparkles,
  Workflow,
  Cpu,
} from "lucide-react";
import { duration, memory, Metrics, Role, scenarios } from "../../model";
import { Sparkline } from "../../components";
export const workspaceRoutes = [
  { path: "/overview", name: "Overview", icon: LayoutDashboard },
  { path: "/pipelines", name: "Pipelines", icon: Workflow },
  { path: "/repositories", name: "Repositories", icon: GitBranch },
  { path: "/healing-studio", name: "Healing studio", icon: Sparkles },
  { path: "/workers", name: "Worker nodes", icon: Cpu },
  { path: "/queue", name: "Job queue", icon: Layers3 },
];
interface NavigationProps {
  route: string;
  collapsed: boolean;
  onCollapse: () => void;
  onNavigate: (path: string) => void;
  onCommands: () => void;
  offline: boolean;
  workers: number;
  queue: number;
  patchCount: number;
}
export function WorkspaceSidebar({
  route,
  collapsed,
  onCollapse,
  onNavigate,
  onCommands,
  offline,
  workers,
  queue,
  patchCount,
}: NavigationProps) {
  return (
    <aside className={`sidebar ${collapsed ? "is-collapsed" : ""}`}>
      <a
        className="brand"
        href="/overview"
        aria-label="SentinelCI home"
        onClick={(e) => {
          e.preventDefault();
          onNavigate("/overview");
        }}
      >
        <span className="brand-symbol">
          <ShieldCheck size={19} />
        </span>
        <strong>
          Sentinel<span>CI</span>
        </strong>
        <span className="brand-version">2.0</span>
      </a>
      <button
        className="workspace-switcher"
        onClick={onCommands}
        title="Open workspace commands"
      >
        <span className="workspace-monogram">S</span>
        <span>
          Sentinel Engineering<small>Engineering workspace</small>
        </span>
        <ChevronDown size={13} />
      </button>
      <div className="nav-section">Workspace</div>
      <nav aria-label="Workspace navigation">
        {workspaceRoutes.slice(0, 4).map((r) => (
          <a
            title={collapsed ? r.name : undefined}
            aria-label={r.name}
            aria-current={route === r.path ? "page" : undefined}
            href={r.path}
            key={r.path}
            className={route === r.path ? "active" : ""}
            onClick={(e) => {
              e.preventDefault();
              onNavigate(r.path);
            }}
          >
            <r.icon size={17} strokeWidth={1.75} />
            <span className="nav-name">{r.name}</span>
            {r.path === "/healing-studio" && patchCount > 0 && (
              <span className="nav-count">{patchCount}</span>
            )}
          </a>
        ))}
      </nav>
      <div className="nav-section runtime-section">Infrastructure</div>
      <nav aria-label="Infrastructure navigation">
        {workspaceRoutes.slice(4).map((r) => (
          <a
            title={collapsed ? r.name : undefined}
            aria-label={r.name}
            aria-current={route === r.path ? "page" : undefined}
            href={r.path}
            key={r.path}
            className={route === r.path ? "active" : ""}
            onClick={(e) => {
              e.preventDefault();
              onNavigate(r.path);
            }}
          >
            <r.icon size={17} strokeWidth={1.75} />
            <span className="nav-name">{r.name}</span>
            <span className="nav-count">
              {r.path === "/workers" ? workers : queue}
            </span>
          </a>
        ))}
      </nav>
      <div className="sidebar-bottom">
        <button
          className="sidebar-command"
          aria-label="Open command palette"
          onClick={onCommands}
        >
          <Command size={15} />
          <span>Command menu</span>
          <kbd>⌘ K</kbd>
        </button>
        <div className={`connection ${offline ? "offline" : ""}`}>
          <i />
          <span>
            {offline ? "Offline reference preview" : "Connected to gateway"}
          </span>
          <small>
            {offline ? "Read-only reference data" : "Live execution events"}
          </small>
        </div>
        <button
          className="collapse-sidebar"
          aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          onClick={onCollapse}
        >
          {collapsed ? (
            <ChevronRight size={16} />
          ) : (
            <PanelLeftClose size={16} />
          )}
          <span>Collapse sidebar</span>
          <kbd>⌃ B</kbd>
        </button>
      </div>
    </aside>
  );
}
interface CommandBarProps {
  title: string;
  offline: boolean;
  role: Role;
  busy: boolean;
  canWrite: boolean;
  scenario: string;
  onScenario: (scenario: string) => void;
  onSimulate: () => void;
  onCommands: () => void;
  onLogin: (role: Role) => void;
  onSession: () => void;
}
export function WorkspaceCommandBar({
  title,
  offline,
  role,
  busy,
  canWrite,
  scenario,
  onScenario,
  onSimulate,
  onCommands,
  onLogin,
  onSession,
}: CommandBarProps) {
  const [profile, setProfile] = useState(false);
  const profileButton = useRef<HTMLButtonElement>(null);
  const closeMenu = () => {
    setProfile(false);
    profileButton.current?.focus();
  };
  useEffect(() => {
    if (!profile) return;
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        closeMenu();
      }
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, [profile]);
  return (
    <header className="header">
      <div className="breadcrumbs">
        <span className="breadcrumb-workspace">Workspace</span>
        <ChevronRight size={12} />
        <span className="breadcrumb-organization">Sentinel Engineering</span>
        <ChevronRight className="breadcrumb-organization" size={12} />
        <strong>{title}</strong>
      </div>
      <button
        className="global-search"
        aria-label="Search workspace commands"
        onClick={onCommands}
      >
        <Search size={14} />
        <span>Search anything…</span>
        <kbd>⌘ K</kbd>
      </button>
      <div className="header-actions">
        <span className={`connection-pill ${offline ? "disconnected" : ""}`}>
          <i />
          {offline ? "Preview" : "Connected"}
        </span>
        <div className="simulate-control">
          <select
            aria-label="Failure simulation scenario"
            value={scenario}
            onChange={(e) => onScenario(e.target.value)}
          >
            {scenarios.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name}
              </option>
            ))}
          </select>
          <button
            className="primary"
            disabled={busy || !canWrite}
            onClick={onSimulate}
          >
            <Play size={11} fill="currentColor" />
            Simulate build
          </button>
        </div>
        <div className="profile-container">
          <button
            ref={profileButton}
            aria-label="Open user menu"
            aria-haspopup="menu"
            aria-expanded={profile}
            className="profile-trigger"
            onClick={() => setProfile((p) => !p)}
          >
            <span className="avatar">
              {{ admin: "A", maintainer: "M", viewer: "V" }[role]}
            </span>
            <ChevronDown size={11} />
          </button>
          {profile && (
            <>
              <button
                className="menu-dismiss"
                tabIndex={-1}
                aria-label="Close user menu"
                onClick={closeMenu}
              />
              <div
                className="profile-menu"
                role="menu"
                aria-label="Session roles"
              >
                <div className="profile-menu-heading">
                  <strong>
                    {
                      {
                        admin: "Administrator",
                        maintainer: "Maintainer",
                        viewer: "Read-only viewer",
                      }[role]
                    }
                  </strong>
                  <span>Demo workspace session</span>
                </div>
                <button
                  role="menuitemradio"
                  aria-checked={role === "admin"}
                  aria-label="Demo Admin"
                  onClick={() => {
                    closeMenu();
                    onLogin("admin");
                  }}
                >
                  <ShieldCheck size={14} />
                  Demo Admin
                  {role === "admin" && (
                    <span className="menu-current">Active</span>
                  )}
                </button>
                <button
                  role="menuitemradio"
                  aria-checked={role === "viewer"}
                  aria-label="Read-only Viewer"
                  onClick={() => {
                    closeMenu();
                    onLogin("viewer");
                  }}
                >
                  <LockKeyhole size={14} />
                  Read-only Viewer
                  {role === "viewer" && (
                    <span className="menu-current">Active</span>
                  )}
                </button>
                <div className="menu-rule" />
                <button
                  role="menuitem"
                  onClick={() => {
                    closeMenu();
                    onSession();
                  }}
                >
                  <Activity size={14} />
                  Session settings
                </button>
              </div>
            </>
          )}
        </div>
      </div>
    </header>
  );
}
export function TelemetryRibbon({
  metrics,
  offline,
}: {
  metrics: Metrics;
  offline: boolean;
}) {
  const series = metrics.timeseries ?? [];
  return (
    <section className="telemetry-ribbon" aria-label="Live telemetry">
      <div className="telemetry-value">
        <span>Pipeline runs</span>
        <strong>{metrics.total_pipelines.toLocaleString()}</strong>
      </div>
      <div className="telemetry-value">
        <span>P95 duration</span>
        <code>{duration(metrics.p95_build_duration_ms)}</code>
        <Sparkline
          values={series
            .filter((s) => s.build_duration_ms !== undefined)
            .map((s) => s.build_duration_ms!)}
          label="Observed build duration history"
        />
      </div>
      <div className="telemetry-value">
        <span>Queue depth</span>
        <code>{metrics.queue_depth}</code>
        <Sparkline
          values={series
            .filter((s) => s.queue_depth !== undefined)
            .map((s) => s.queue_depth!)}
          label="Observed queue depth history"
        />
      </div>
      <div className="telemetry-value">
        <span>Worker CPU</span>
        <code>
          {metrics.worker_cpu_percent === undefined
            ? "—"
            : `${metrics.worker_cpu_percent.toFixed(1)}%`}
        </code>
      </div>
      <div className="telemetry-value">
        <span>Memory</span>
        <code>{memory(metrics.worker_memory_bytes)}</code>
      </div>
      <div className="telemetry-value">
        <span>Healing rate</span>
        <code className="green">
          {`${metrics.healing_success_rate.toFixed(1)}%`}
        </code>
      </div>
      <span className="sampling">
        <i />
        {offline ? "Reference" : "Live"}
      </span>
    </section>
  );
}
