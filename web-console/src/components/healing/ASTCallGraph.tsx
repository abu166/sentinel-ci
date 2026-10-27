import { useEffect, useMemo, useRef, useState } from "react";
import { Crosshair, GitFork, Minus, Plus, RotateCcw } from "lucide-react";
import type { ASTFunctionNode, ASTCallGraphData } from "../../model";
export type { ASTFunctionNode, ASTCallGraphData } from "../../model";
interface Props {
  graph?: ASTCallGraphData;
  onInspect: (file: string, line: number) => void;
  onSelect?: (node: ASTFunctionNode) => void;
}
export function layoutFunctions(graph: ASTCallGraphData) {
  const known = new Set(graph.nodes.map((n) => n.id));
  const levels = new Map<string, number>();
  const root =
    graph.root_id && known.has(graph.root_id)
      ? graph.root_id
      : graph.nodes[0]?.id;
  if (root) {
    levels.set(root, 0);
    const queue = [root];
    while (queue.length) {
      const from = queue.shift()!;
      for (const edge of graph.edges.filter(
        (e) => e.source === from && known.has(e.target),
      )) {
        if (!levels.has(edge.target)) {
          levels.set(edge.target, Math.min(5, (levels.get(from) ?? 0) + 1));
          queue.push(edge.target);
        }
      }
    }
  }
  const rows = new Map<number, number>();
  return [...graph.nodes]
    .sort(
      (a, b) =>
        (levels.get(a.id) ?? 0) - (levels.get(b.id) ?? 0) ||
        a.start_line - b.start_line,
    )
    .map((node) => {
      const level = levels.get(node.id) ?? 0,
        row = rows.get(level) ?? 0;
      rows.set(level, row + 1);
      return { ...node, x: 24 + level * 205, y: 38 + row * 88 };
    });
}
export default function ASTCallGraph({ graph, onInspect, onSelect }: Props) {
  const [selectedId, setSelectedId] = useState(""),
    [zoom, setZoom] = useState(1),
    [pan, setPan] = useState({ x: 0, y: 0 });
  const svg = useRef<SVGSVGElement>(null);
  const drag = useRef<{
    x: number;
    y: number;
    panX: number;
    panY: number;
  } | null>(null);
  const nodes = useMemo(() => (graph ? layoutFunctions(graph) : []), [graph]);
  const selected =
    nodes.find((n) => n.id === selectedId) ??
    nodes.find((n) => n.id === graph?.root_id) ??
    nodes[0];
  const width = Math.max(440, ...nodes.map((n) => n.x + 195)),
    height = Math.max(190, ...nodes.map((n) => n.y + 86));
  useEffect(() => {
    setSelectedId("");
    setZoom(1);
    setPan({ x: 0, y: 0 });
  }, [graph?.root_id, graph?.provenance]);
  useEffect(() => {
    const element = svg.current;
    if (!element) return;
    const wheel = (event: WheelEvent) => {
      if (!event.ctrlKey && !event.metaKey) return;
      event.preventDefault();
      setZoom((value) =>
        Math.max(0.5, Math.min(3, value * (event.deltaY < 0 ? 1.1 : 1 / 1.1))),
      );
    };
    element.addEventListener("wheel", wheel, { passive: false });
    return () => element.removeEventListener("wheel", wheel);
  }, [graph]);
  const pick = (node: ASTFunctionNode) => {
    setSelectedId(node.id);
    onSelect?.(node);
  };
  if (!graph?.nodes.length)
    return (
      <section className="ast-call-graph" aria-label="AST dependency graph">
        <div className="ast-graph-heading">
          <span>
            <GitFork size={13} />
            Function dependency graph
          </span>
        </div>
        <div className="ast-graph-empty">
          This repair has no recorded call graph. Run a new analysis to collect
          parser-derived function and helper edges.
        </div>
      </section>
    );
  return (
    <section className="ast-call-graph" aria-label="AST dependency graph">
      <div className="ast-graph-heading">
        <span>
          <GitFork size={13} />
          Function dependency graph
        </span>
        <div>
          <button
            aria-label="Zoom out AST graph"
            onClick={() => setZoom((z) => Math.max(0.5, z / 1.2))}
            disabled={zoom <= 0.5}
          >
            <Minus size={12} />
          </button>
          <output aria-label="AST graph zoom">{`${Math.round(zoom * 100)}%`}</output>
          <button
            aria-label="Zoom in AST graph"
            onClick={() => setZoom((z) => Math.min(3, z * 1.2))}
            disabled={zoom >= 3}
          >
            <Plus size={12} />
          </button>
          <button
            aria-label="Reset AST graph viewport"
            onClick={() => {
              setZoom(1);
              setPan({ x: 0, y: 0 });
            }}
          >
            <RotateCcw size={12} />
          </button>
        </div>
      </div>
      <div className="ast-graph-canvas">
        <svg
          ref={svg}
          viewBox={`0 0 ${width} ${height}`}
          role="group"
          aria-label="Interactive AST call graph"
          tabIndex={0}
          onKeyDown={(e) => {
            if (e.key === "+" || e.key === "=") {
              e.preventDefault();
              setZoom((z) => Math.min(3, z * 1.2));
            } else if (e.key === "-") {
              e.preventDefault();
              setZoom((z) => Math.max(0.5, z / 1.2));
            }
          }}
          onPointerDown={(e) => {
            if ((e.target as Element).closest("[data-function-node]")) return;
            drag.current = {
              x: e.clientX,
              y: e.clientY,
              panX: pan.x,
              panY: pan.y,
            };
            e.currentTarget.setPointerCapture?.(e.pointerId);
          }}
          onPointerMove={(e) => {
            if (!drag.current) return;
            const rect = e.currentTarget.getBoundingClientRect();
            setPan({
              x:
                drag.current.panX +
                ((e.clientX - drag.current.x) * width) / (rect.width || width),
              y:
                drag.current.panY +
                ((e.clientY - drag.current.y) * height) /
                  (rect.height || height),
            });
          }}
          onPointerUp={(e) => {
            drag.current = null;
            if (e.currentTarget.hasPointerCapture?.(e.pointerId))
              e.currentTarget.releasePointerCapture(e.pointerId);
          }}
          onPointerCancel={() => {
            drag.current = null;
          }}
        >
          <defs>
            <marker
              id="ast-call-arrow"
              markerWidth="6"
              markerHeight="6"
              refX="5"
              refY="3"
              orient="auto"
            >
              <path d="M0 0 L6 3 L0 6" fill="var(--dag-edge)" />
            </marker>
          </defs>
          <g
            data-ast-transform
            transform={`translate(${pan.x} ${pan.y}) scale(${zoom})`}
          >
            {graph.edges.map((edge, i) => {
              const from = nodes.find((n) => n.id === edge.source),
                to = nodes.find((n) => n.id === edge.target);
              if (!from || !to) return null;
              const path =
                from.id === to.id
                  ? `M${from.x + 125} ${from.y} C${from.x + 170} ${from.y - 32} ${from.x + 65} ${from.y - 32} ${from.x + 75} ${from.y}`
                  : `M${from.x + 170} ${from.y + 28} C${from.x + 198} ${from.y + 28} ${to.x - 28} ${to.y + 28} ${to.x} ${to.y + 28}`;
              return (
                <path
                  key={`${edge.source}-${edge.target}-${i}`}
                  d={path}
                  className="ast-call-edge"
                  markerEnd="url(#ast-call-arrow)"
                >
                  <title>{`${from.name} calls ${to.name}`}</title>
                </path>
              );
            })}
            {nodes.map((node, i) => (
              <g
                key={node.id}
                data-function-node={node.id}
                data-ast-node-index={i}
                className={`ast-function-node ${selected?.id === node.id ? "selected" : ""}`}
                transform={`translate(${node.x} ${node.y})`}
                role="button"
                tabIndex={0}
                aria-pressed={selected?.id === node.id}
                aria-label={`Select function ${node.name} at ${node.file_path}:${node.start_line}`}
                onClick={() => pick(node)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    pick(node);
                  } else if (
                    [
                      "ArrowRight",
                      "ArrowDown",
                      "ArrowLeft",
                      "ArrowUp",
                      "Home",
                      "End",
                    ].includes(e.key)
                  ) {
                    e.preventDefault();
                    const index =
                      e.key === "Home"
                        ? 0
                        : e.key === "End"
                          ? nodes.length - 1
                          : (i +
                              (e.key === "ArrowLeft" || e.key === "ArrowUp"
                                ? -1
                                : 1) +
                              nodes.length) %
                            nodes.length;
                    svg.current
                      ?.querySelector<SVGElement & { focus: () => void }>(
                        `[data-ast-node-index="${index}"]`,
                      )
                      ?.focus?.();
                  }
                }}
              >
                <rect width="170" height="56" rx="5" />
                <text x="10" y="21" className="ast-function-name">
                  {node.name.length > 21
                    ? `${node.name.slice(0, 20)}…`
                    : node.name}
                </text>
                <text
                  x="10"
                  y="42"
                  className="ast-function-meta"
                >{`L${node.start_line}–${node.end_line}`}</text>
                <text
                  x="160"
                  y="42"
                  textAnchor="end"
                  className="ast-function-meta"
                >{`C ${node.complexity}`}</text>
              </g>
            ))}
          </g>
        </svg>
      </div>
      <div className="ast-graph-selection">
        <div>
          <strong>{selected?.name}</strong>
          <code>
            {selected
              ? `${selected.file_path}:${selected.start_line}`
              : "No function selected"}
          </code>
        </div>
        <button
          disabled={!selected}
          onClick={() => {
            if (selected) onInspect(selected.file_path, selected.start_line);
          }}
        >
          <Crosshair size={12} />
          Inspect source
        </button>
      </div>
      <div className="ast-graph-provenance">{`${graph.provenance ?? "Parser provenance unreported"} · ${graph.nodes.length} functions · ${graph.edges.length} call edges · drag to pan, Ctrl+wheel to zoom`}</div>
    </section>
  );
}
