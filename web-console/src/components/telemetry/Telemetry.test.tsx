import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { Worker } from "../../model";
import CanvasSparkline, { drawTelemetry } from "./CanvasSparkline";
import WorkerTelemetryPanel from "./WorkerTelemetryPanel";
import {
  MAX_TELEMETRY_HOSTS,
  MAX_TELEMETRY_SAMPLES,
  metricSegments,
  observeWorkerSamples,
  TelemetrySample,
} from "./series";
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
const worker = (overrides: Partial<Worker> = {}): Worker => ({
  id: "runner-a:0",
  hostname: "runner-a",
  metrics_scope: "controller_host",
  sampled_at: "2026-10-27T10:00:00Z",
  cpu_percent: 0,
  memory_bytes: 1048576,
  ...overrides,
});
const sample = (
  timestamp: number,
  cpuPercent: number | null,
  memoryBytes: number | null,
): TelemetrySample => ({
  timestamp,
  cpuPercent,
  memoryBytes,
  timestampSource: "runner",
});
function context() {
  return {
    setTransform: vi.fn(),
    clearRect: vi.fn(),
    setLineDash: vi.fn(),
    beginPath: vi.fn(),
    moveTo: vi.fn(),
    lineTo: vi.fn(),
    stroke: vi.fn(),
    arc: vi.fn(),
    fill: vi.fn(),
    fillText: vi.fn(),
    strokeStyle: "",
    lineWidth: 0,
    lineJoin: "",
    fillStyle: "",
    font: "",
    textAlign: "",
  };
}
describe("real bounded host observations", () => {
  it("deduplicates routines reporting the same controller host without fabricating history", () => {
    const history = observeWorkerSamples(
      new Map(),
      [worker(), worker({ id: "runner-a:1" })],
      Date.now(),
    );
    expect(history.size).toBe(1);
    const series = [...history.values()][0];
    expect(series.samples).toHaveLength(1);
    expect(series.samples[0]).toMatchObject({
      cpuPercent: 0,
      memoryBytes: 1048576,
      timestampSource: "runner",
    });
    expect(series.scope).toBe("controller_host");
  });
  it("ignores repeated or out-of-order runner snapshots and appends actual later samples", () => {
    const first = observeWorkerSamples(new Map(), [worker()], 1);
    const duplicate = observeWorkerSamples(
      first,
      [worker({ cpu_percent: 99 })],
      2,
    );
    expect([...duplicate.values()][0].samples).toHaveLength(1);
    const next = observeWorkerSamples(
      duplicate,
      [worker({ sampled_at: "2026-10-27T10:00:05Z", cpu_percent: 8 })],
      3,
    );
    expect([...next.values()][0].samples.map((s) => s.cpuPercent)).toEqual([
      0, 8,
    ]);
  });
  it("preserves unknown samples as gaps and treats real zero as measured zero", () => {
    const history = observeWorkerSamples(
      new Map(),
      [worker({ cpu_percent: null, memory_bytes: null })],
      1,
    );
    expect([...history.values()][0].samples[0]).toMatchObject({
      cpuPercent: null,
      memoryBytes: null,
    });
    const segments = metricSegments(
      [sample(0, 0, 1048576), sample(1, null, null), sample(2, 5, 2097152)],
      "cpu",
    );
    expect(segments).toEqual([
      [{ timestamp: 0, value: 0 }],
      [{ timestamp: 2, value: 5 }],
    ]);
    expect(metricSegments([sample(0, 0, 1048576)], "memory")).toEqual([
      [{ timestamp: 0, value: 1 }],
    ]);
  });
  it("uses explicit client observation time only when runner timestamps are absent", () => {
    const history = observeWorkerSamples(
      new Map(),
      [worker({ sampled_at: undefined })],
      12345,
    );
    expect([...history.values()][0].samples[0]).toMatchObject({
      timestamp: 12345,
      timestampSource: "client_observation",
    });
  });
  it("bounds both points and retained host identities", () => {
    let history = new Map();
    for (let n = 0; n < MAX_TELEMETRY_SAMPLES + 20; n++)
      history = observeWorkerSamples(
        history,
        [worker({ sampled_at: new Date(n * 5000).toISOString() })],
        n * 5000,
      );
    expect([...history.values()][0].samples).toHaveLength(
      MAX_TELEMETRY_SAMPLES,
    );
    history = observeWorkerSamples(
      new Map(),
      Array.from({ length: MAX_TELEMETRY_HOSTS + 8 }, (_, n) =>
        worker({ id: `runner-${n}:0`, hostname: `runner-${n}` }),
      ),
      0,
    );
    expect(history.size).toBe(MAX_TELEMETRY_HOSTS);
  });
  it("keeps independent worker-process scopes separate even when a hostname is shared", () => {
    const history = observeWorkerSamples(
      new Map(),
      [
        worker({ metrics_scope: "worker_process" }),
        worker({ id: "runner-a:1", metrics_scope: "worker_process" }),
      ],
      1,
    );
    expect(history.size).toBe(2);
  });
});
describe("Canvas renderer", () => {
  it("draws an actual single sample as a point, not an invented zero-to-value line", () => {
    const ctx = context(),
      canvas = document.createElement("canvas");
    vi.spyOn(canvas, "getContext").mockReturnValue(
      ctx as unknown as CanvasRenderingContext2D,
    );
    drawTelemetry(
      canvas,
      [sample(10, 25, 1048576)],
      "cpu",
      300,
      68,
      2,
      "#818cf8",
      "#85858f",
    );
    expect(canvas.width).toBe(600);
    expect(canvas.height).toBe(136);
    expect(ctx.setTransform).toHaveBeenCalledWith(2, 0, 0, 2, 0, 0);
    expect(ctx.arc).toHaveBeenCalledTimes(1);
    expect(ctx.lineTo).toHaveBeenCalledTimes(1);
  });
  it("does not draw null readings as zero-valued datapoints", () => {
    const ctx = context(),
      canvas = document.createElement("canvas");
    vi.spyOn(canvas, "getContext").mockReturnValue(
      ctx as unknown as CanvasRenderingContext2D,
    );
    drawTelemetry(
      canvas,
      [sample(10, null, null)],
      "cpu",
      300,
      68,
      1,
      "#818cf8",
      "#85858f",
    );
    expect(ctx.clearRect).toHaveBeenCalled();
    expect(ctx.arc).not.toHaveBeenCalled();
    expect(ctx.lineTo).not.toHaveBeenCalled();
  });
  it("draws disconnected segments around missing observations and labels binary memory units", () => {
    const ctx = context(),
      canvas = document.createElement("canvas");
    vi.spyOn(canvas, "getContext").mockReturnValue(
      ctx as unknown as CanvasRenderingContext2D,
    );
    drawTelemetry(
      canvas,
      [sample(0, 1, 1048576), sample(1, null, null), sample(2, 3, 2097152)],
      "memory",
      300,
      68,
      1,
      "#818cf8",
      "#85858f",
    );
    expect(ctx.arc).toHaveBeenCalledTimes(2);
    expect(
      ctx.fillText.mock.calls.some((call) => String(call[0]).includes("MiB")),
    ).toBe(true);
  });
  it("redraws HiDPI backing pixels on ResizeObserver changes without animation loops", () => {
    const ctx = context();
    vi.stubGlobal("CanvasRenderingContext2D", function () {});
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(
      ctx as unknown as CanvasRenderingContext2D,
    );
    let width = 300;
    vi.spyOn(
      HTMLCanvasElement.prototype,
      "getBoundingClientRect",
    ).mockImplementation(() => ({
      width,
      height: 68,
      top: 0,
      left: 0,
      right: width,
      bottom: 68,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    }));
    let callback: () => void = () => {};
    const disconnect = vi.fn();
    vi.stubGlobal(
      "ResizeObserver",
      class {
        constructor(fn: () => void) {
          callback = fn;
        }
        observe = vi.fn();
        disconnect = disconnect;
      },
    );
    vi.stubGlobal("devicePixelRatio", 2);
    const raf = vi.fn();
    vi.stubGlobal("requestAnimationFrame", raf);
    const view = render(
      <CanvasSparkline
        samples={[sample(0, 1, 1)]}
        metric="cpu"
        label="Host CPU observations"
      />,
    );
    const canvas = screen.getByRole("img", {
      name: "Host CPU observations",
    }) as HTMLCanvasElement;
    expect(canvas.width).toBe(600);
    width = 400;
    callback();
    expect(canvas.width).toBe(800);
    expect(raf).not.toHaveBeenCalled();
    view.unmount();
    expect(disconnect).toHaveBeenCalled();
  });
  it("shows an empty live collection honestly and never seeds a chart with reference points", () => {
    render(<WorkerTelemetryPanel history={new Map()} offline={true} />);
    expect(screen.getByLabelText("Latest host CPU")).toHaveTextContent("—");
    expect(screen.getByLabelText("Latest host memory")).toHaveTextContent("—");
    expect(screen.getByText(/no synthetic points/)).toBeInTheDocument();
  });
  it("switches between actual host histories and names the controller attribution scope", () => {
    const history = observeWorkerSamples(
      new Map(),
      [
        worker(),
        worker({
          id: "runner-b:0",
          hostname: "runner-b",
          cpu_percent: 33,
          memory_bytes: 2097152,
        }),
      ],
      1,
    );
    render(<WorkerTelemetryPanel history={history} offline={false} />);
    expect(screen.getByLabelText("Latest host CPU")).toHaveTextContent("0.0%");
    fireEvent.change(screen.getByLabelText("Select telemetry host"), {
      target: { value: "controller_host:runner-b" },
    });
    expect(screen.getByLabelText("Latest host CPU")).toHaveTextContent("33.0%");
    expect(screen.getByLabelText("Latest host memory")).toHaveTextContent(
      "2.0 MiB",
    );
    expect(screen.getByText(/not per-routine attribution/)).toBeInTheDocument();
  });
});
