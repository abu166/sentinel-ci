import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  cleanup,
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import App from "./App";
import { DAG, RepairPane, Terminal } from "./components";
import {
  ansiSegments,
  collection,
  normalizeLivePatch,
  orderQueueSnapshot,
  duration,
  fuzzyScore,
  unifiedRows,
  relativeAge,
  memory,
  previewLogs,
  previewMetrics,
  previewPatch,
  previewPipelines,
  previewQueues,
  previewRepos,
  previewSteps,
  previewWorkers,
  request,
  setToken,
  sha,
  sandboxPolicy,
  stripAnsi,
  topologicalSteps,
} from "./model";
class Socket {
  static instances: Socket[] = [];
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onerror: (() => void) | null = null;
  onclose: (() => void) | null = null;
  close = vi.fn();
  constructor() {
    Socket.instances.push(this);
    queueMicrotask(() => this.onopen?.());
  }
}
let calls: { url: string; body?: Record<string, unknown>; method?: string }[] =
  [];
const livePolicy = {
  backend: "docker",
  isolated: true,
  memory_mb: 768,
  network: "none",
  pids_limit: 128,
  cpus: 1,
  read_only_root: true,
  secrets_scrubbed: true,
};
function setup() {
  const pipeline = {
    ...previewPipelines[0],
    steps: previewPipelines[0].steps?.map((step) => ({
      ...step,
      sandbox_policy: livePolicy,
      cpu_user_ms: 120,
      cpu_system_ms: 40,
    })),
    patches: [previewPatch],
    logs: previewLogs,
  };
  vi.stubGlobal("WebSocket", Socket);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, options?: RequestInit) => {
      const body = options?.body ? JSON.parse(String(options.body)) : undefined;
      calls.push({ url, body, method: options?.method });
      let data: unknown;
      if (url.endsWith("/auth/login"))
        data = { token: "signed-session", role: body.role ?? "admin" };
      else if (url.endsWith("/pipelines"))
        data = [pipeline, ...previewPipelines.slice(1)];
      else if (url.endsWith("/simulate-failure"))
        data = { pipeline_id: pipeline.id, id: pipeline.id };
      else if (url.includes("/pipelines/")) data = pipeline;
      else if (url.endsWith("/metrics/overview")) data = previewMetrics;
      else if (url.endsWith("/workers"))
        data = [
          {
            ...previewWorkers[0],
            hostname: undefined,
            host: "runner-live-01",
            heartbeat: "2026-10-27T10:00:00Z",
            sandbox_policy: livePolicy,
          },
        ];
      else if (url.endsWith("/queue"))
        data = {
          queues: previewQueues,
          outbox: { pending: 2, published: 42, dead: 0 },
        };
      else if (url.endsWith("/patches")) data = [previewPatch];
      else if (url.endsWith("/pulls")) data = [];
      else if (url.includes("/patches/")) data = { id: "pull-1" };
      else if (url.includes("/repos"))
        data = options?.method ? previewRepos[0] : previewRepos;
      else data = {};
      return {
        ok: true,
        status: 200,
        json: async () => data,
        text: async () => JSON.stringify(data),
      };
    }),
  );
}
async function live() {
  setup();
  render(<App />);
  await waitFor(() =>
    expect(screen.getByText("Connected to gateway")).toBeInTheDocument(),
  );
}
beforeEach(() => {
  Socket.instances = [];
  calls = [];
  history.replaceState({}, "", "/overview");
  setToken("");
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});
describe("distinct navigation and data views", () => {
  it("renders dense overview, observed telemetry and connected SVG graph", async () => {
    await live();
    expect(
      screen.getByRole("heading", { name: "Executions" }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Live telemetry")).toHaveTextContent(
      "P95 duration",
    );
    expect(
      screen.getByRole("img", {
        name: "Interactive pipeline dependency graph",
      }),
    ).toBeInTheDocument();
    expect(document.querySelectorAll(".dag-connector").length).toBeGreaterThan(
      0,
    );
    expect(
      screen.queryByText("Pipeline control center"),
    ).not.toBeInTheDocument();
  });
  it.each([
    ["Pipelines", "/pipelines", "Execution matrix"],
    ["Repositories", "/repositories", "Repository registry"],
    ["Healing studio", "/healing-studio", "Healing studio"],
    ["Worker nodes", "/workers", "Worker nodes"],
    ["Job queue", "/queue", "Job queue"],
  ])("dedicated %s route", async (name, path, heading) => {
    await live();
    fireEvent.click(
      screen.getByRole("link", { name: new RegExp(`^${name}`, "i") }),
    );
    expect(location.pathname).toBe(path);
    expect(screen.getByRole("heading", { name: heading })).toBeInTheDocument();
  });
  it("supports browser navigation", async () => {
    await live();
    history.pushState({}, "", "/workers");
    fireEvent(window, new PopStateEvent("popstate"));
    expect(
      screen.getByRole("heading", { name: "Worker nodes" }),
    ).toBeInTheDocument();
  });
  it("filters executions safely to empty", async () => {
    await live();
    fireEvent.change(screen.getByLabelText("Filter executions"), {
      target: { value: "missing-repository" },
    });
    expect(document.querySelectorAll(".pipeline-item")).toHaveLength(0);
  });
  it("filters matrix author and status", async () => {
    await live();
    fireEvent.click(screen.getByRole("link", { name: "Pipelines" }));
    fireEvent.change(screen.getByLabelText("Author"), {
      target: { value: "s.kassymov" },
    });
    fireEvent.change(screen.getByLabelText("Status"), {
      target: { value: "PASSED" },
    });
    const table = document.querySelector(".execution-matrix")!;
    expect(table.textContent).toContain("s.kassymov");
    expect(table.textContent).not.toContain("abukhasym");
  });
  it("labels offline reference state and disables mutations", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("offline")));
    render(<App />);
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(
        "Session unavailable",
      ),
    );
    expect(screen.getByText("Offline reference preview")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Simulate build" }),
    ).toBeDisabled();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
});
describe("authorized execution and configuration actions", () => {
  it("switches to Viewer and includes bearer tokens", async () => {
    await live();
    fireEvent.click(screen.getByRole("button", { name: "Open user menu" }));
    fireEvent.click(
      screen.getByRole("menuitemradio", { name: "Read-only Viewer" }),
    );
    await waitFor(() =>
      expect(
        calls.filter((c) => c.url.endsWith("/auth/login")).at(-1)?.body?.role,
      ).toBe("viewer"),
    );
    expect(
      screen.getByRole("button", { name: "Simulate build" }),
    ).toBeDisabled();
    expect(
      calls.some(
        (c) => c.url.endsWith("/auth/login") && c.body?.role === "viewer",
      ),
    ).toBe(true);
    const f = fetch as ReturnType<typeof vi.fn>;
    expect(
      f.mock.calls.some(
        (c) =>
          ((c[1] as RequestInit)?.headers as Record<string, string>)
            ?.Authorization === "Bearer signed-session",
      ),
    ).toBe(true);
  });
  it("queues selected failure scenario", async () => {
    await live();
    fireEvent.change(screen.getByLabelText("Failure simulation scenario"), {
      target: { value: "data-race" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Simulate build" }));
    await waitFor(() =>
      expect(
        calls.some(
          (c) =>
            c.url.endsWith("/simulate-failure") &&
            c.body?.scenario === "data-race",
        ),
      ).toBe(true),
    );
    await waitFor(() => expect(location.pathname).toBe("/healing-studio"));
  });
  it.each([
    ["Approve & Apply", "apply"],
    ["Publish Branch / PR", "publish"],
    ["Re-run Regression", "verify"],
    ["Reject", "reject"],
  ])("dispatches %s", async (label, action) => {
    await live();
    await waitFor(() =>
      expect(screen.getByRole("button", { name: label })).toBeEnabled(),
    );
    fireEvent.click(screen.getByRole("button", { name: label }));
    await waitFor(() =>
      expect(
        calls.some((c) =>
          c.url.endsWith(`/patches/${previewPatch.id}/${action}`),
        ),
      ).toBe(true),
    );
    if (action === "verify")
      expect(calls.find((c) => c.url.endsWith("/verify"))?.body).toEqual({
        race: false,
      });
  });
  it("PATCHes repository branch and policies", async () => {
    await live();
    fireEvent.click(screen.getByRole("link", { name: "Repositories" }));
    const repo = document.querySelector(".repository") as HTMLElement;
    fireEvent.change(within(repo).getByLabelText("Default branch"), {
      target: { value: "release/v3" },
    });
    fireEvent.click(
      within(repo).getByRole("button", { name: "Save settings" }),
    );
    await waitFor(() =>
      expect(
        calls.some(
          (c) =>
            c.method === "PATCH" && c.body?.default_branch === "release/v3",
        ),
      ).toBe(true),
    );
  });
  it("registers repository through real endpoint", async () => {
    await live();
    fireEvent.click(screen.getByRole("link", { name: "Repositories" }));
    fireEvent.click(
      screen.getByRole("button", { name: "Register repository" }),
    );
    const dialog = screen.getByRole("dialog", { name: "Register repository" });
    fireEvent.change(within(dialog).getByLabelText("Repository name"), {
      target: { value: "new-service" },
    });
    fireEvent.change(within(dialog).getByLabelText("Repository Git URL"), {
      target: { value: "https://github.com/org/new-service" },
    });
    fireEvent.submit(dialog);
    await waitFor(() =>
      expect(
        calls.some(
          (c) =>
            c.url.endsWith("/repos") &&
            c.method === "POST" &&
            c.body?.name === "new-service",
        ),
      ).toBe(true),
    );
  });
});
describe("keyboard and engineering inspectors", () => {
  it("opens Ctrl+K palette and navigates with Enter", async () => {
    await live();
    fireEvent.keyDown(window, { key: "k", ctrlKey: true });
    expect(
      screen.getByRole("dialog", { name: "Command palette" }),
    ).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Search commands"), {
      target: { value: "Go to Worker nodes" },
    });
    fireEvent.keyDown(screen.getByLabelText("Search commands"), {
      key: "Enter",
    });
    expect(location.pathname).toBe("/workers");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
  it("opens Cmd+K and triggers named simulation command", async () => {
    await live();
    fireEvent.keyDown(window, { key: "k", metaKey: true });
    fireEvent.change(screen.getByLabelText("Search commands"), {
      target: { value: "Simulate Go Nil-Pointer" },
    });
    fireEvent.keyDown(screen.getByLabelText("Search commands"), {
      key: "ArrowDown",
    });
    fireEvent.keyDown(screen.getByLabelText("Search commands"), {
      key: "Enter",
    });
    await waitFor(() =>
      expect(calls.some((c) => c.body?.scenario === "nil-pointer")).toBe(true),
    );
    fireEvent.keyDown(window, { key: "k", ctrlKey: true });
    fireEvent.keyDown(window, { key: "Escape" });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
  it("filters stderr/search and opens exact source location", () => {
    const onLocation = vi.fn();
    render(
      <Terminal
        logs={previewLogs}
        selectedStep=""
        onLocation={onLocation}
        state="live"
      />,
    );
    fireEvent.click(screen.getByRole("tab", { name: /^stderr/ }));
    expect(screen.getByRole("log")).not.toHaveTextContent("checkout");
    fireEvent.change(screen.getByLabelText("Search logs"), {
      target: { value: "File" },
    });
    fireEvent.click(screen.getByRole("button", { name: /File.*pagination/ }));
    expect(onLocation).toHaveBeenCalledWith("pkg/inventory/pagination.py", 2);
  });
  it("renders token diff, unified mode and actual AST diagnostics", () => {
    render(
      <RepairPane
        patch={previewPatch}
        onAction={vi.fn()}
        disabled={false}
        busy={false}
        pulls={null}
      />,
    );
    expect(document.querySelector(".split-diff")).toBeInTheDocument();
    expect(document.querySelector(".token-changed")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Unified" }));
    expect(document.querySelector(".unified-diff")).toHaveTextContent(
      "len(items) - 1",
    );
    fireEvent.click(screen.getByRole("button", { name: "diagnostics" }));
    expect(screen.getByText("Ochiai")).toBeInTheDocument();
    expect(screen.getByText("Not reported by analyzer")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "verification" }));
    expect(screen.getByText(/Ran 8 tests/)).toBeInTheDocument();
  });
  it("selects DAG with keyboard", () => {
    const select = vi.fn();
    render(<DAG steps={previewSteps} selected="" onSelect={select} />);
    fireEvent.keyDown(
      screen.getByRole("button", { name: "Inspect Unit Test Suite" }),
      { key: "Enter" },
    );
    expect(select).toHaveBeenCalledWith(previewSteps[2]);
  });
});
describe("safe data formatting", () => {
  it("renders only hexadecimal SHAs", () => {
    expect(sha("e41b80d1234")).toBe("e41b80d");
    expect(sha("demo")).toBe("—");
    expect(sha("")).toBe("—");
  });
  it("normalizes null collections and formats durations and RAM", () => {
    expect(collection({ queues: [1] }, "queues")).toEqual([1]);
    expect(collection(null, "queues")).toEqual([]);
    expect(duration(3420)).toBe("3.42s");
    expect(duration()).toBe("—");
    expect(memory(1048576)).toBe("1.0 MiB");
    expect(memory(null)).toBe("—");
  });
  it("preserves ANSI colors", () => {
    expect(stripAnsi("\x1b[31merror\x1b[0m")).toBe("error");
    expect(ansiSegments("\x1b[31merror\x1b[0m")[0].color).toBe("#fb7185");
  });
  it("sorts dependencies and retains cyclic evidence", () => {
    expect(
      topologicalSteps([...previewSteps].reverse()).map((s) => s.id),
    ).toEqual(previewSteps.map((s) => s.id));
    expect(
      topologicalSteps([
        { ...previewSteps[0], depends_on: [previewSteps[0].id] },
      ]),
    ).toHaveLength(1);
  });
  it("surfaces denied requests", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: false,
        status: 403,
        text: async () => '{"error":"viewer cannot mutate"}',
      }),
    );
    await expect(
      request("/patches/id/apply", { method: "POST" }),
    ).rejects.toThrow("viewer cannot mutate");
  });
});

it("renders structured live sandbox policy for both worker and step telemetry", async () => {
  await live();
  fireEvent.click(screen.getByRole("link", { name: /^Worker nodes/ }));
  await waitFor(() =>
    expect(screen.getByText("runner-live-01")).toBeInTheDocument(),
  );
  expect(
    screen.getAllByText(/docker · network=none · rootfs=read-only · 768 MiB/)
      .length,
  ).toBeGreaterThan(1);
  expect(screen.getAllByText("0.120s").length).toBeGreaterThan(0);
});
it("formats structured, legacy and absent sandbox policies safely", () => {
  expect(sandboxPolicy(livePolicy)).toContain("secrets=scrubbed");
  expect(sandboxPolicy("network=none")).toBe("network=none");
  expect(sandboxPolicy(null)).toBe("Policy not reported");
  expect(sandboxPolicy({})).toBe("Policy not reported");
});

it.each([
  ["Deadlock", "deadlock"],
  ["SQL Injection", "sql-injection"],
])(
  "triggers palette-only %s scenario without expanding primary selector",
  async (name, scenario) => {
    await live();
    expect(
      within(screen.getByLabelText("Failure simulation scenario")).getAllByRole(
        "option",
      ),
    ).toHaveLength(5);
    fireEvent.keyDown(window, { key: "k", ctrlKey: true });
    fireEvent.change(screen.getByLabelText("Search commands"), {
      target: { value: `Simulate ${name}` },
    });
    fireEvent.keyDown(screen.getByLabelText("Search commands"), {
      key: "Enter",
    });
    await waitFor(() =>
      expect(
        calls.some(
          (c) =>
            c.url.endsWith("/simulate-failure") &&
            c.body?.scenario === scenario,
        ),
      ).toBe(true),
    );
  },
);

it("collapses sidebar while retaining accessible navigation", async () => {
  await live();
  fireEvent.click(screen.getByRole("button", { name: "Collapse sidebar" }));
  expect(document.querySelector(".workbench-app")).toHaveClass(
    "sidebar-collapsed",
  );
  expect(
    screen.getByRole("link", { name: "Repositories" }),
  ).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Expand sidebar" }));
  expect(document.querySelector(".workbench-app")).not.toHaveClass(
    "sidebar-collapsed",
  );
});
it("opens global search and restores avatar focus on menu Escape", async () => {
  await live();
  fireEvent.click(
    screen.getByRole("button", { name: "Search workspace commands" }),
  );
  expect(
    screen.getByRole("dialog", { name: "Command palette" }),
  ).toBeInTheDocument();
  fireEvent.keyDown(window, { key: "Escape" });
  const profile = screen.getByRole("button", { name: "Open user menu" });
  fireEvent.click(profile);
  expect(
    screen.getByRole("menu", { name: "Session roles" }),
  ).toBeInTheDocument();
  fireEvent.keyDown(window, { key: "Escape" });
  expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  expect(profile).toHaveFocus();
});
it("uses correct source gutters across diff hunks", () => {
  const rows = unifiedRows(
    "--- a/file.go\n+++ b/file.go\n@@ -10,3 +20,3 @@\n keep\n-old\n+new\n context\n@@ -45 +55 @@\n-next\n+after\n",
  );
  expect(rows[3]).toMatchObject({ oldLine: 10, newLine: 20, kind: "context" });
  expect(rows[4]).toMatchObject({
    oldLine: 11,
    newLine: undefined,
    kind: "removed",
  });
  expect(rows[5]).toMatchObject({
    oldLine: undefined,
    newLine: 21,
    kind: "added",
  });
  expect(rows[6]).toMatchObject({ oldLine: 12, newLine: 22 });
  expect(rows[8]).toMatchObject({ oldLine: 45 });
  expect(rows[9]).toMatchObject({ newLine: 55 });
});
it("copies filtered terminal output without ANSI control codes", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });
  render(
    <Terminal
      logs={[
        { sequence_num: 1, stream: "stdout", content: "\x1b[32mgreen\x1b[0m" },
        { sequence_num: 2, stream: "stderr", content: "error" },
      ]}
      selectedStep=""
      onLocation={vi.fn()}
      state="live"
    />,
  );
  fireEvent.click(screen.getByRole("tab", { name: /^stdout/ }));
  fireEvent.click(screen.getByRole("button", { name: "Copy terminal output" }));
  await waitFor(() => expect(writeText).toHaveBeenCalledWith("green"));
  expect(
    screen.getByRole("button", { name: "Copy terminal output" }),
  ).toHaveAttribute("title", "Copied");
  fireEvent.click(screen.getByRole("button", { name: "Toggle auto-scroll" }));
  expect(
    screen.getByRole("button", { name: "Toggle auto-scroll" }),
  ).toHaveAttribute("aria-pressed", "false");
});
it("reports denied clipboard access truthfully", async () => {
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText: vi.fn().mockRejectedValue(new Error("denied")) },
  });
  render(
    <Terminal
      logs={previewLogs}
      selectedStep=""
      onLocation={vi.fn()}
      state="live"
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "Copy terminal output" }));
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "Copy terminal output" }),
    ).toHaveAttribute("title", "Clipboard unavailable"),
  );
});
it("generates webhook secrets locally and saves only on submit", async () => {
  await live();
  fireEvent.click(screen.getByRole("link", { name: "Repositories" }));
  const first = previewRepos[0];
  fireEvent.click(
    screen.getByRole("button", {
      name: `Generate webhook secret for ${first.name}`,
    }),
  );
  const secret = screen.getByLabelText(
    `Webhook secret for ${first.name}`,
  ) as HTMLInputElement;
  expect(secret.value).toMatch(/^[a-f0-9]{64}$/);
  expect(calls.some((c) => c.method === "PATCH")).toBe(false);
  fireEvent.click(
    screen.getByRole("button", {
      name: `Reveal webhook secret for ${first.name}`,
    }),
  );
  expect(secret.type).toBe("text");
  fireEvent.click(
    within(document.querySelector(".repository") as HTMLElement).getByRole(
      "button",
      { name: "Save settings" },
    ),
  );
  await waitFor(() =>
    expect(
      calls.some(
        (c) =>
          c.method === "PATCH" && typeof c.body?.webhook_secret === "string",
      ),
    ).toBe(true),
  );
});
it("opens and closes matrix execution inspection drawer", async () => {
  await live();
  fireEvent.click(screen.getByRole("link", { name: "Pipelines" }));
  fireEvent.click(
    screen.getAllByRole("button", { name: /Inspect .* execution/ })[0],
  );
  expect(
    screen.getByRole("complementary", { name: "Execution inspection drawer" }),
  ).toBeInTheDocument();
  fireEvent.click(
    screen.getByRole("button", { name: "Close execution inspector" }),
  );
  expect(
    screen.queryByRole("complementary", {
      name: "Execution inspection drawer",
    }),
  ).not.toBeInTheDocument();
});
it("dispatches optional Go race verification separately", async () => {
  await live();
  fireEvent.click(screen.getByRole("button", { name: "Go Race" }));
  await waitFor(() =>
    expect(
      calls.some((c) => c.url.endsWith("/verify") && c.body?.race === true),
    ).toBe(true),
  );
});
it("formats execution ages from real timestamps", () => {
  expect(relativeAge(undefined)).toBe("Queued");
  expect(relativeAge("invalid")).toBe("Unknown");
  expect(relativeAge(new Date(0).toISOString(), 65000)).toBe("1m ago");
});
it("does not color passing unittest stderr output as an error", () => {
  render(
    <Terminal
      logs={[
        { sequence_num: 1, stream: "stderr", content: "Ran 2 tests in 0.002s" },
        { sequence_num: 2, stream: "stderr", content: "OK" },
        {
          sequence_num: 3,
          stream: "stderr",
          content: "IndexError: out of range",
        },
      ]}
      selectedStep=""
      onLocation={vi.fn()}
      state="live"
    />,
  );
  expect(screen.getByText("OK").closest(".terminal-line")).not.toHaveClass(
    "failure",
  );
  expect(
    screen.getByText("IndexError: out of range").closest(".terminal-line"),
  ).toHaveClass("failure");
});

it("groups command shortcuts without adding keyboard navigation stops", async () => {
  await live();
  fireEvent.click(
    screen.getByRole("button", { name: "Search workspace commands" }),
  );
  const dialog = screen.getByRole("dialog", { name: "Command palette" });
  for (const name of ["Navigation", "Repair scenarios", "Pipelines"])
    expect(within(dialog).getByRole("heading", { name })).toBeInTheDocument();
  fireEvent.change(screen.getByLabelText("Search commands"), {
    target: { value: "hlstd" },
  });
  expect(
    within(dialog).getByRole("button", { name: /Go to Healing studio/ }),
  ).toBeInTheDocument();
  fireEvent.keyDown(screen.getByLabelText("Search commands"), { key: "Enter" });
  expect(location.pathname).toBe("/healing-studio");
});
it("scores fuzzy subsequences and rejects unmatched search text", () => {
  expect(fuzzyScore("hlstd", "Go to Healing studio")).not.toBeNull();
  expect(fuzzyScore("healing", "Healing studio")!).toBeGreaterThan(
    fuzzyScore("hlstd", "Healing studio")!,
  );
  expect(fuzzyScore("xyz", "Healing studio")).toBeNull();
});

it("traps palette Tab and Shift+Tab and restores the originating launcher on Escape", async () => {
  await live();
  const launcher = screen.getByRole("button", {
    name: "Search workspace commands",
  });
  launcher.focus();
  fireEvent.click(launcher);
  const dialog = screen.getByRole("dialog", { name: "Command palette" });
  const first = within(dialog).getByRole("textbox", {
    name: "Search commands",
  });
  const last = within(dialog).getAllByRole("button").at(-1)!;
  last.focus();
  fireEvent.keyDown(last, { key: "Tab" });
  expect(first).toHaveFocus();
  fireEvent.keyDown(first, { key: "Tab", shiftKey: true });
  expect(last).toHaveFocus();
  fireEvent.keyDown(last, { key: "Escape" });
  await waitFor(() =>
    expect(
      screen.queryByRole("dialog", { name: "Command palette" }),
    ).not.toBeInTheDocument(),
  );
  expect(launcher).toHaveFocus();
});
it("restores palette launcher focus when its backdrop is dismissed", async () => {
  await live();
  const launcher = screen.getByRole("button", {
    name: "Search workspace commands",
  });
  launcher.focus();
  fireEvent.click(launcher);
  fireEvent.mouseDown(
    screen.getByRole("dialog", { name: "Command palette" }).parentElement!,
  );
  await waitFor(() => expect(launcher).toHaveFocus());
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});
it("shows fixed Admin and Viewer choices with accurate checked states in both sessions", async () => {
  await live();
  fireEvent.click(screen.getByRole("button", { name: "Open user menu" }));
  expect(
    screen.getByRole("menuitemradio", { name: "Demo Admin" }),
  ).toHaveAttribute("aria-checked", "true");
  fireEvent.click(
    screen.getByRole("menuitemradio", { name: "Read-only Viewer" }),
  );
  await waitFor(() =>
    expect(
      calls.filter((c) => c.url.endsWith("/auth/login")).at(-1)?.body?.role,
    ).toBe("viewer"),
  );
  fireEvent.click(screen.getByRole("button", { name: "Open user menu" }));
  expect(screen.getAllByRole("menuitemradio")).toHaveLength(2);
  expect(
    screen.getByRole("menuitemradio", { name: "Demo Admin" }),
  ).toHaveAttribute("aria-checked", "false");
  expect(
    screen.getByRole("menuitemradio", { name: "Read-only Viewer" }),
  ).toHaveAttribute("aria-checked", "true");
  expect(screen.queryByText("Switch to Admin")).not.toBeInTheDocument();
});

it("keeps infrastructure telemetry in named keyboard-scrollable regions", async () => {
  await live();
  fireEvent.click(screen.getByRole("link", { name: "Worker nodes" }));
  expect(
    screen.getByRole("region", { name: "Worker routine telemetry" }),
  ).toHaveAttribute("tabindex", "0");
  expect(
    screen.getByRole("region", { name: "Step resource telemetry" }),
  ).toHaveTextContent("Checkout");
  fireEvent.click(screen.getByRole("link", { name: "Job queue" }));
  expect(
    screen.getByRole("region", { name: "Queue delivery metrics" }),
  ).toHaveTextContent("healer.dead");
});

it("keeps broker topology positions stable while telemetry loads and appends unknown queues", () => {
  const ordered = orderQueueSnapshot([
    { name: "healer.trigger", messages: 1 },
    { name: "future.queue", messages: 2 },
  ]);
  expect(ordered.map((q) => q.name)).toEqual([
    "jobs.dispatch",
    "logs.stream",
    "healer.trigger",
    "healer.result",
    "healer.dead",
    "future.queue",
  ]);
  expect(ordered[0].status).toBe("UNREPORTED");
  expect(ordered[2].messages).toBe(1);
  expect(ordered[5].messages).toBe(2);
});

it("renders telemetry numbers and units as one text leaf and unknown CPU as plain dash", async () => {
  await live();
  fireEvent.click(screen.getByRole("link", { name: "Worker nodes" }));
  const summary = document.querySelectorAll(".infra-summary b")[1];
  expect(summary.textContent).toBe("13.6%");
  expect(summary.childNodes).toHaveLength(1);
  const resources = screen.getByRole("region", {
    name: "Step resource telemetry",
  });
  expect(resources).not.toHaveTextContent("—s");
  const measured = within(resources).getAllByText("0.120s")[0];
  expect(measured.childNodes).toHaveLength(1);
});

it("normalizes live HealResult IDs while preserving selected-candidate evidence", () => {
  const wire = {
    ...previewPatch,
    id: undefined,
    patch_id: "wire-patch",
    selected_candidate_id: "candidate-two",
    diagnostics: { complexity_before: 1, complexity_after: 2 },
  };
  expect(normalizeLivePatch(wire)).toMatchObject({
    id: "wire-patch",
    selected_candidate_id: "candidate-two",
    patched_code: previewPatch.patched_code,
    diagnostics: { complexity_before: 1, complexity_after: 2 },
  });
  expect(normalizeLivePatch({ ...previewPatch })).toMatchObject({
    id: previewPatch.id,
  });
});
it.each([
  { id: undefined, patch_id: undefined },
  { id: 42, patch_id: null },
  { id: "", patch_id: " " },
  { id: "first", patch_id: "second" },
])("ignores malformed or ambiguous live patch identity %j", (identity) => {
  expect(normalizeLivePatch({ ...previewPatch, ...identity })).toBeUndefined();
});
it("uses a live patch_id for candidate requests instead of requesting undefined", async () => {
  await live();
  const base = vi.mocked(fetch).getMockImplementation()!;
  vi.mocked(fetch).mockImplementation(async (url, options) =>
    String(url).endsWith("/patches/wire-patch/candidates")
      ? ({
          ok: true,
          status: 200,
          json: async () => ({ candidates: [] }),
          text: async () => "",
        } as Response)
      : base(url, options),
  );
  await waitFor(() => expect(Socket.instances.length).toBeGreaterThan(0));
  await act(async () => {
    Socket.instances
      .at(-1)
      ?.onmessage?.({
        data: JSON.stringify({
          event: "healer.result",
          content: "Healer verification: VERIFIED",
          stream: "stdout",
          patch: {
            ...previewPatch,
            id: undefined,
            patch_id: "wire-patch",
            selected_candidate_id: "actual-candidate",
          },
        }),
      });
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  fireEvent.click(screen.getByRole("button", { name: "candidates" }));
  await screen.findByText(/No candidate records/);
  expect(
    vi
      .mocked(fetch)
      .mock.calls.some((call) =>
        String(call[0]).endsWith("/patches/wire-patch/candidates"),
      ),
  ).toBe(true);
  expect(
    vi
      .mocked(fetch)
      .mock.calls.some((call) =>
        String(call[0]).includes("/patches/undefined/"),
      ),
  ).toBe(false);
});
it("ignores a live event missing both identities and retains the last valid patch", async () => {
  await live();
  const base = vi.mocked(fetch).getMockImplementation()!;
  vi.mocked(fetch).mockImplementation(async (url, options) =>
    String(url).endsWith(`/patches/${previewPatch.id}/candidates`)
      ? ({
          ok: true,
          status: 200,
          json: async () => ({ candidates: [] }),
          text: async () => "",
        } as Response)
      : base(url, options),
  );
  await waitFor(() => expect(Socket.instances.length).toBeGreaterThan(0));
  await act(async () => {
    Socket.instances
      .at(-1)
      ?.onmessage?.({
        data: JSON.stringify({
          event: "healer.result",
          content: "malformed result",
          patch: { ...previewPatch, id: undefined, patch_id: undefined },
        }),
      });
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  fireEvent.click(screen.getByRole("button", { name: "candidates" }));
  await screen.findByText(/No candidate records/);
  expect(
    vi
      .mocked(fetch)
      .mock.calls.some((call) =>
        String(call[0]).endsWith(`/patches/${previewPatch.id}/candidates`),
      ),
  ).toBe(true);
  expect(
    vi
      .mocked(fetch)
      .mock.calls.some((call) =>
        String(call[0]).includes("/patches/undefined/"),
      ),
  ).toBe(false);
});
