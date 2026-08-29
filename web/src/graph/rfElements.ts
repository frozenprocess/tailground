// Turns the graph model + layout + focus state into React Flow nodes and edges.

import type { Edge, Node } from "@xyflow/react";
import type { EngineResult } from "../engine/types";
import { pairKey } from "../engine/keys";
import type { Positioned } from "./layout";
import type { GraphModel } from "./model";
import type { VisibleEdge } from "./visibility";
import type { EdgeArrows, NodePorts, PortState } from "./nodes";
import { EDGE_DASH, EDGE_STYLES, FADE_OPACITY, REST_OPACITY, type EdgeKind } from "./style";

/** Gap between the outermost tailnet nodes and the tailnet frame. */
const TAILNET_FRAME_PAD = 28;

/**
 * P2-4 — the tailnet frame: bounding box of every local top-level node (group
 * containers, loose devices, route/service targets), excluding the internet
 * zone and the external-tailnet frames. Drawn behind the graph in flow
 * coordinates via <ViewportPortal>, not an ELK node.
 */
export function tailnetFrame(model: GraphModel, positioned: Positioned) {
  let x0 = Infinity,
    y0 = Infinity,
    x1 = -Infinity,
    y1 = -Infinity;
  for (const n of model.nodes) {
    if (n.parentId || n.kind === "extframe" || n.kind === "internet") continue;
    const p = positioned.pos[n.id];
    if (!p) continue;
    const sz = positioned.size[n.id] ?? { width: n.width, height: n.height };
    x0 = Math.min(x0, p.x);
    y0 = Math.min(y0, p.y);
    x1 = Math.max(x1, p.x + sz.width);
    y1 = Math.max(y1, p.y + sz.height);
  }
  if (!Number.isFinite(x0)) return null;
  const pad = TAILNET_FRAME_PAD;
  return { x: x0 - pad, y: y0 - pad, w: x1 - x0 + pad * 2, h: y1 - y0 + pad * 2 };
}

/**
 * Per node: the left (ingress) port lights up where arrows end, the right
 * (egress) port where they start. A two-way edge marks both of its ends
 * `both`; a port is faded only if every edge on it is. Tethers are undirected
 * (no arrowheads) and get no port.
 */
export function portsOf(vis: VisibleEdge[]): Map<string, NodePorts> {
  const m = new Map<string, NodePorts>();
  const mark = (id: string, side: keyof NodePorts, mode: PortState["mode"], faded: boolean) => {
    const np = m.get(id) ?? {};
    const cur = np[side];
    np[side] = {
      mode: cur && cur.mode !== mode ? "both" : mode,
      faded: cur ? cur.faded && faded : faded,
    };
    m.set(id, np);
  };
  for (const e of vis) {
    if (e.kind === "tether") continue;
    mark(e.source, "right", e.bidir ? "both" : "out", e.faded);
    mark(e.target, "left", e.bidir ? "both" : "in", e.faded);
  }
  return m;
}

export interface NodeFocus {
  focusedActor: number | null;
  /** Actors with an allow edge to / from the focused one. */
  connected: Set<number>;
  reach: { out: number; in: number } | null;
  /** Actors "hide isolated" removes. */
  hidden: Set<number>;
  /** Actors ringed by the group highlight. */
  inGroup: Set<number>;
  ports: Map<string, NodePorts>;
}

export function rfNodes(model: GraphModel, positioned: Positioned, f: NodeFocus): Node[] {
  const { focusedActor, connected } = f;
  return model.nodes
    .filter((n) => n.actorId == null || !f.hidden.has(n.actorId))
    .map((n) => {
      const p = positioned.pos[n.id] ?? { x: 0, y: 0 };
      const sz = positioned.size[n.id];
      const focused = n.actorId != null && n.actorId === focusedActor;
      const container = n.kind === "group" || n.kind === "extframe";
      const dimmed =
        focusedActor != null &&
        !container &&
        !focused &&
        !(n.actorId != null && connected.has(n.actorId)) &&
        !(n.kind === "internet" && connected.size > 0);
      return {
        id: n.id,
        type: n.kind,
        position: p,
        parentId: n.parentId,
        extent: n.parentId ? ("parent" as const) : undefined,
        selectable: n.kind === "device",
        draggable: false,
        data: {
          label: n.label,
          sub: n.sub,
          variant: n.targetVariant,
          tagged: !!n.sub && n.kind === "device" && isTagged(n.sub),
          focused,
          dimmed,
          inGroup: n.actorId != null && f.inGroup.has(n.actorId),
          reach: focused && n.kind === "device" ? f.reach : undefined,
          ports: f.ports.get(n.id),
          actorId: n.actorId ?? null,
        },
        style:
          container && sz
            ? { width: sz.width, height: sz.height }
            : { width: n.width, height: n.height },
        zIndex: container ? 0 : 1,
      };
    });
}

/** The drawn edges, styled from the style table. `portDesc` names the probe
 *  ("tcp:443", "any port") in the hover text. */
export function rfEdges(
  vis: VisibleEdge[],
  model: GraphModel,
  result: EngineResult | null,
  portDesc: string,
): Edge[] {
  const nameOf = (id: number) =>
    model.nodes.find((n) => n.actorId === id)?.label ?? String(id);
  // Spell out the direction — the arrowhead says it, the tooltip names it.
  const tipFor = (e: VisibleEdge) => {
    if (!result) return e.tip;
    const one = (src: number, dst: number) =>
      `${nameOf(src)} → ${nameOf(dst)} · ${e.tip ?? edgeTip(result, src, dst, e.kind, portDesc)}`;
    if (e.kind === "tether") return e.tip;
    return e.bidir ? `${one(e.src, e.dst)}\n${one(e.dst, e.src)}` : one(e.src, e.dst);
  };

  return vis.map((e) => {
    const s = EDGE_STYLES[e.kind];
    const opacity = e.faded ? FADE_OPACITY : e.highlighted ? 1 : REST_OPACITY;
    return {
      id: e.id,
      source: e.source,
      target: e.target,
      type: "tip",
      animated: false,
      selectable: e.kind !== "deny" && e.kind !== "tether",
      style: {
        stroke: s.stroke,
        strokeWidth: e.highlighted ? s.width + 1 : s.width,
        strokeDasharray: s.dashed ? EDGE_DASH : undefined,
        opacity,
      },
      data: {
        kind: e.kind,
        tip: tipFor(e),
        arrows: (e.kind === "tether" ? "none" : e.bidir ? "both" : "end") satisfies EdgeArrows,
      },
    };
  });
}

function isTagged(sub: string): boolean {
  return sub.startsWith("tag:");
}

/** Hover text for an allow / blocked edge — proto·port, any grant strings, and
 *  the posture reason when the pair is downgraded. */
function edgeTip(
  result: EngineResult,
  src: number,
  dst: number,
  kind: EdgeKind,
  portDesc: string,
): string {
  const key = pairKey(src, dst);
  if (kind === "posture") {
    const why = result.posture?.denials?.[key];
    return why ? `blocked by posture — ${why}` : "blocked by posture";
  }
  if (kind === "deny") return `denied on ${portDesc}`;
  if (kind === "leaves") return "egresses to the internet via an exit node";
  const parts = [kind === "reaches" ? "reaches this target" : `allow · ${portDesc}`];
  const grants = result.grants?.[key];
  if (grants?.length) parts.push(`grant: ${grants.join(", ")}`);
  return parts.join(" · ");
}
