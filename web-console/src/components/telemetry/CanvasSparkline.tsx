import { useEffect, useRef } from "react";
import { metricSegments, TelemetrySample } from "./series";
interface Props {
  samples: TelemetrySample[];
  metric: "cpu" | "memory";
  label: string;
}
export function drawTelemetry(
  canvas: HTMLCanvasElement,
  samples: TelemetrySample[],
  metric: "cpu" | "memory",
  width: number,
  height: number,
  dpr: number,
  color: string,
  muted: string,
) {
  const ctx = canvas.getContext("2d");
  if (!ctx || width <= 0 || height <= 0) return;
  canvas.width = Math.max(1, Math.round(width * dpr));
  canvas.height = Math.max(1, Math.round(height * dpr));
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, width, height);
  const segments = metricSegments(samples, metric),
    points = segments.flat();
  if (!points.length) return;
  const lowTime = samples[0]?.timestamp ?? points[0].timestamp,
    highTime = samples.at(-1)?.timestamp ?? lowTime;
  const upper =
    Math.max(metric === "cpu" ? 100 : 1, ...points.map((p) => p.value)) * 1.08;
  const pad = { left: 4, right: 4, top: 7, bottom: 16 };
  const x = (timestamp: number) =>
    pad.left +
    (highTime === lowTime
      ? width - pad.left - pad.right
      : ((timestamp - lowTime) / (highTime - lowTime)) *
        (width - pad.left - pad.right));
  const y = (value: number) =>
    pad.top + (1 - value / upper) * (height - pad.top - pad.bottom);
  ctx.strokeStyle = muted;
  ctx.lineWidth = 0.7;
  ctx.setLineDash([2, 4]);
  ctx.beginPath();
  ctx.moveTo(pad.left, height - pad.bottom);
  ctx.lineTo(width - pad.right, height - pad.bottom);
  ctx.stroke();
  ctx.setLineDash([]);
  ctx.strokeStyle = color;
  ctx.fillStyle = color;
  ctx.lineWidth = 1.6;
  ctx.lineJoin = "round";
  for (const segment of segments) {
    ctx.beginPath();
    segment.forEach((point, i) => {
      if (i === 0) ctx.moveTo(x(point.timestamp), y(point.value));
      else ctx.lineTo(x(point.timestamp), y(point.value));
    });
    if (segment.length > 1) ctx.stroke();
    const last = segment.at(-1)!;
    ctx.beginPath();
    ctx.arc(x(last.timestamp), y(last.value), 2.3, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.fillStyle = muted;
  ctx.font = '11px "JetBrains Mono",monospace';
  ctx.textAlign = "left";
  ctx.fillText("0", pad.left, height - 2);
  ctx.textAlign = "right";
  ctx.fillText(
    `${upper.toFixed(metric === "cpu" ? 0 : 1)}${metric === "cpu" ? "%" : " MiB"}`,
    width - pad.right,
    12,
  );
}
export default function CanvasSparkline({ samples, metric, label }: Props) {
  const canvas = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    const element = canvas.current;
    if (!element) return;
    const paint = () => {
      if (typeof CanvasRenderingContext2D === "undefined") return;
      const rect = element.getBoundingClientRect();
      const styles = getComputedStyle(element);
      drawTelemetry(
        element,
        samples,
        metric,
        rect.width || 300,
        rect.height || 68,
        Math.max(1, window.devicePixelRatio || 1),
        styles.getPropertyValue("--accent-light").trim() || "#818cf8",
        styles.getPropertyValue("--text-faint").trim() || "#85858f",
      );
    };
    paint();
    const observer =
      typeof ResizeObserver !== "undefined"
        ? new ResizeObserver(paint)
        : undefined;
    observer?.observe(element);
    window.addEventListener("resize", paint);
    return () => {
      observer?.disconnect();
      window.removeEventListener("resize", paint);
    };
  }, [samples, metric]);
  return (
    <canvas
      ref={canvas}
      className="telemetry-canvas"
      role="img"
      aria-label={label}
    >
      {label}.{" "}
      {samples.length
        ? `${samples.length} actual observed samples.`
        : "No live samples have been collected."}
    </canvas>
  );
}
