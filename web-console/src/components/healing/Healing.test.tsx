import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { ASTCallGraphData, previewPatch, setToken } from "../../model";
import ASTCallGraph, { layoutFunctions } from "./ASTCallGraph";
import CandidateVotingPanel, { RepairCandidate } from "./CandidateVotingPanel";
const graph: ASTCallGraphData = {
  root_id: "a.py:1:process",
  provenance: "Python ast",
  nodes: [
    {
      id: "a.py:1:process",
      name: "process",
      file_path: "a.py",
      start_line: 1,
      end_line: 3,
      complexity: 2,
    },
    {
      id: "a.py:5:helper",
      name: "helper",
      file_path: "a.py",
      start_line: 5,
      end_line: 7,
      complexity: 1,
    },
    {
      id: "a.py:9:unrelated",
      name: "unrelated",
      file_path: "a.py",
      start_line: 9,
      end_line: 10,
      complexity: 1,
    },
  ],
  edges: [
    { source: "a.py:1:process", target: "a.py:5:helper", kind: "calls" },
    { source: "a.py:5:helper", target: "a.py:5:helper", kind: "calls" },
  ],
};
const candidate = (
  id: string,
  overrides: Partial<RepairCandidate> = {},
): RepairCandidate => ({
  id,
  patch_id: previewPatch.id,
  rank: 1,
  category: "boundary-off-by-one",
  variant: `actual-${id}`,
  file_path: "a.py",
  original_code: "return items[len(items)]",
  patched_code: "return items[-1]",
  unified_diff: "-old\n+new",
  ast_cost: 2,
  test_success: true,
  score: 0.98,
  status: "VERIFIED",
  selected: false,
  test_output: "[failing command]\nOK\n[regression]\nOK",
  ...overrides,
});
const response = (data: unknown, status = 200) => ({
  ok: status < 400,
  status,
  json: async () => data,
  text: async () => JSON.stringify(data),
});
beforeEach(() => setToken("test-maintainer-token"));
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  setToken("");
});
describe("parser-derived AST graph", () => {
  it("renders only reported call edges, including a real recursive edge", () => {
    render(<ASTCallGraph graph={graph} onInspect={vi.fn()} />);
    expect(document.querySelectorAll(".ast-call-edge")).toHaveLength(2);
    expect(
      screen.getByText(/Python ast · 3 functions · 2 call edges/),
    ).toBeInTheDocument();
    expect(
      layoutFunctions(graph).find((n) => n.name === "helper")!.x,
    ).toBeGreaterThan(
      layoutFunctions(graph).find((n) => n.name === "process")!.x,
    );
  });
  it("reports missing graph metadata instead of making links from nearby function names", () => {
    render(<ASTCallGraph onInspect={vi.fn()} />);
    expect(screen.getByText(/no recorded call graph/)).toBeInTheDocument();
    expect(document.querySelector(".ast-call-edge")).not.toBeInTheDocument();
  });
  it("selects functions with keyboard and inspects their exact source location", () => {
    const inspect = vi.fn(),
      select = vi.fn();
    render(
      <ASTCallGraph graph={graph} onInspect={inspect} onSelect={select} />,
    );
    const helper = screen.getByRole("button", {
      name: "Select function helper at a.py:5",
    });
    fireEvent.keyDown(helper, { key: "Enter" });
    expect(helper).toHaveAttribute("aria-pressed", "true");
    expect(select).toHaveBeenCalledWith(
      expect.objectContaining({ name: "helper", start_line: 5 }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Inspect source" }));
    expect(inspect).toHaveBeenCalledWith("a.py", 5);
  });
  it("zooms with controls and keyboard then resets the actual SVG viewport", () => {
    render(<ASTCallGraph graph={graph} onInspect={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Zoom in AST graph" }));
    expect(screen.getByLabelText("AST graph zoom")).toHaveTextContent("120%");
    fireEvent.keyDown(
      screen.getByRole("group", { name: "Interactive AST call graph" }),
      { key: "+" },
    );
    expect(screen.getByLabelText("AST graph zoom")).toHaveTextContent("144%");
    fireEvent.click(
      screen.getByRole("button", { name: "Reset AST graph viewport" }),
    );
    expect(screen.getByLabelText("AST graph zoom")).toHaveTextContent("100%");
  });
  it("pans by pointer without inventing edges or triggering a node selection", () => {
    vi.stubGlobal("PointerEvent", MouseEvent);
    render(<ASTCallGraph graph={graph} onInspect={vi.fn()} />);
    const svg = screen.getByRole("group", {
      name: "Interactive AST call graph",
    });
    vi.spyOn(svg, "getBoundingClientRect").mockReturnValue({
      width: 440,
      height: 225,
      top: 0,
      left: 0,
      right: 440,
      bottom: 225,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    });
    fireEvent.pointerDown(svg, { clientX: 10, clientY: 10 });
    fireEvent.pointerMove(svg, { clientX: 55, clientY: 30 });
    expect(
      document.querySelector("[data-ast-transform]")?.getAttribute("transform"),
    ).not.toBe("translate(0 0) scale(1)");
    fireEvent.pointerUp(svg);
    expect(document.querySelectorAll(".ast-call-edge")).toHaveLength(2);
  });
});
describe("recorded candidate voting and permissions", () => {
  function setupCandidates() {
    const rows = [
      candidate("first", { selected: true }),
      candidate("second", { rank: 2, score: 0.91 }),
      candidate("rejected", {
        rank: 3,
        status: "REJECTED",
        test_success: false,
        score: 0.1,
      }),
    ];
    const fetchMock = vi.fn(async (url: string, options?: RequestInit) =>
      url.endsWith("/select-candidate")
        ? response({
            selected_candidate_id: "second",
            patch: {
              ...previewPatch,
              patched_code: "return items[-1]",
              selected_candidate_id: "second",
            },
          })
        : response({ candidates: rows, selected_candidate_id: "first" }),
    );
    vi.stubGlobal("fetch", fetchMock);
    return { rows, fetchMock };
  }
  it("shows actual score, test result, AST cost, and recorded stdout for at most three candidates", async () => {
    setupCandidates();
    render(
      <CandidateVotingPanel
        patch={previewPatch}
        disabled={false}
        published={false}
        onSelected={vi.fn()}
      />,
    );
    await screen.findByText("actual-second");
    expect(document.querySelectorAll(".repair-candidate")).toHaveLength(3);
    expect(screen.getByText("0.910")).toBeInTheDocument();
    expect(screen.getByText("Failed")).toBeInTheDocument();
    const second = document.querySelectorAll(
      ".repair-candidate",
    )[1] as HTMLElement;
    fireEvent.click(within(second).getByText("Test evidence"));
    expect(within(second).getByText(/\[regression\]/)).toBeInTheDocument();
  });
  it("selects only a verified passing alternative and updates selected flags after the authoritative response", async () => {
    const { fetchMock } = setupCandidates(),
      selected = vi.fn();
    render(
      <CandidateVotingPanel
        patch={previewPatch}
        disabled={false}
        published={false}
        onSelected={selected}
      />,
    );
    await screen.findByText("actual-second");
    const cards = document.querySelectorAll(".repair-candidate");
    expect(
      within(cards[0] as HTMLElement).getByRole("button", {
        name: "Select candidate",
      }),
    ).toBeDisabled();
    expect(
      within(cards[2] as HTMLElement).getByRole("button", {
        name: "Select candidate",
      }),
    ).toBeDisabled();
    fireEvent.click(
      within(cards[1] as HTMLElement).getByRole("button", {
        name: "Select candidate",
      }),
    );
    await waitFor(() =>
      expect(selected).toHaveBeenCalledWith(
        expect.objectContaining({ selected_candidate_id: "second" }),
      ),
    );
    const post = fetchMock.mock.calls.find((call) =>
      call[0].endsWith("/select-candidate"),
    )!;
    expect(JSON.parse(String(post[1]?.body))).toEqual({
      candidate_id: "second",
    });
    expect((post[1]?.headers as Record<string, string>).Authorization).toBe(
      "Bearer test-maintainer-token",
    );
    expect(document.querySelectorAll(".repair-candidate")[1]).toHaveClass(
      "selected",
    );
    expect(document.querySelectorAll(".repair-candidate")[0]).not.toHaveClass(
      "selected",
    );
  });
  it("keeps Viewer evidence readable while preventing all candidate mutation requests", async () => {
    const { fetchMock } = setupCandidates();
    render(
      <CandidateVotingPanel
        patch={previewPatch}
        disabled={true}
        published={false}
        onSelected={vi.fn()}
      />,
    );
    await screen.findByText("actual-second");
    for (const button of screen.getAllByRole("button", {
      name: "Select candidate",
    }))
      expect(button).toBeDisabled();
    fireEvent.click(
      screen.getAllByRole("button", { name: "Select candidate" })[1],
    );
    expect(
      fetchMock.mock.calls.some((call) =>
        call[0].endsWith("/select-candidate"),
      ),
    ).toBe(false);
    expect(screen.getByText(/Read-only role/)).toBeInTheDocument();
  });
  it.each([
    { approval_status: "APPROVED", published: false },
    { approval_status: "REJECTED", published: false },
    { approval_status: "PENDING", published: true },
  ])(
    "locks candidate choice after approval/rejection/publication: %j",
    async ({ approval_status, published }) => {
      setupCandidates();
      render(
        <CandidateVotingPanel
          patch={{ ...previewPatch, approval_status }}
          disabled={false}
          published={published}
          onSelected={vi.fn()}
        />,
      );
      await screen.findByText("actual-second");
      expect(
        screen
          .getAllByRole("button", { name: "Select candidate" })
          .every((button) => (button as HTMLButtonElement).disabled),
      ).toBe(true);
      expect(screen.getByText(/locked after/)).toBeInTheDocument();
    },
  );
  it("surfaces a rejected server selection without pretending the patch changed", async () => {
    const selected = vi.fn();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string) =>
        url.endsWith("/select-candidate")
          ? response({ error: "candidate does not belong to patch" }, 409)
          : response({ candidates: [candidate("second")] }),
      ),
    );
    render(
      <CandidateVotingPanel
        patch={previewPatch}
        disabled={false}
        published={false}
        onSelected={selected}
      />,
    );
    await screen.findByText("actual-second");
    fireEvent.click(screen.getByRole("button", { name: "Select candidate" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "candidate does not belong to patch",
    );
    expect(selected).not.toHaveBeenCalled();
  });
  it("does not fabricate candidate cards for a legacy repair with no recorded variants", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(response({ candidates: [] })),
    );
    render(
      <CandidateVotingPanel
        patch={previewPatch}
        disabled={false}
        published={false}
        onSelected={vi.fn()}
      />,
    );
    expect(await screen.findByText(/No candidate records/)).toBeInTheDocument();
    expect(document.querySelector(".repair-candidate")).not.toBeInTheDocument();
  });
});
