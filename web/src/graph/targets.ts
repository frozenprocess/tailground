import { pairKey } from "../engine/keys";
import type { EngineResult } from "../engine/types";
import type { ClassifiedEdge, GraphNode, TargetVariant } from "./model";

// Tailground-layer targets (routes / service VIPs / shared nodes) get synthetic
// negative ids well clear of INTERNET_ID and any real actor id.
const TARGET_ID_BASE = -1000;

/**
 * Add Tailground-layer target nodes (approved routes, service VIPs, shared
 * external nodes) and the "reaches" edges from the devices that can hit them on
 * the current probe. A target with no allow edge is skipped — the matrix panels
 * still list it.
 */
export function addTargets(
  result: EngineResult,
  cells: Record<string, string>,
  nodes: GraphNode[],
  skeleton: ClassifiedEdge[],
  edges: ClassifiedEdge[],
) {
  interface Spec {
    variant: TargetVariant;
    nodeId: string;
    label: string;
    sub?: string;
    column: string;
    inbound?: boolean;
    exit?: boolean;
    /** Device node ids this target hangs off (router for a route, backends for a
     *  service). Emitted as `tether` skeleton edges so ELK places the target
     *  next to its serving machine (P2-1). */
    tetherTo?: string[];
    /** Hover text for the `reaches` edge, naming the serving machine (P2-2). */
    tip?: string;
    /** External-tailnet alias — the node lives in that tailnet's frame (P2-3). */
    alias?: string;
  }
  const specs: Spec[] = [];
  const actorIds = new Set(result.actors.map((a) => a.id));

  for (const p of result.routes?.paths ?? []) {
    if (!p.approved || !p.column) continue;
    specs.push({
      variant: "route",
      nodeId: `tgt:${p.column}`,
      label: p.prefix,
      sub: `${p.exit ? "exit · " : ""}via ${p.router}`,
      column: p.column,
      exit: p.exit,
      tetherTo: actorIds.has(p.routerId) ? [`dev:${p.routerId}`] : undefined,
      tip: p.exit
        ? `internet egress · routed via ${p.router}`
        : `reaches ${p.prefix} · routed via ${p.router}`,
    });
  }
  for (const p of result.services?.paths ?? []) {
    if (!p.column) continue;
    const backends = (p.backends ?? []).filter((id) => actorIds.has(id));
    specs.push({
      variant: "service",
      nodeId: `tgt:${p.column}`,
      label: p.name,
      sub: p.vip,
      column: p.column,
      tetherTo: backends.map((id) => `dev:${id}`),
      tip: `reaches ${p.name} · VIP ${p.vip}`,
    });
  }
  for (const t of result.externalShare?.tailnets ?? []) {
    for (const n of t.nodes ?? []) {
      if (!n.column) continue;
      specs.push({
        variant: "external",
        nodeId: `tgt:${n.column}`,
        label: n.name,
        sub: [...(n.tags ?? []), ...(n.groups ?? [])].join(", ") || "shared node",
        column: n.column,
        inbound: n.inbound,
        alias: t.alias,
      });
    }
  }

  let tid = TARGET_ID_BASE;
  // External nodes are deferred so their `extframe` container can be pushed
  // first (React Flow needs a parent before its children) — P2-3.
  const extNodes: GraphNode[] = [];
  const extAliases = new Set<string>();
  for (const spec of specs) {
    const targetActorId = tid--;
    const reach: ClassifiedEdge[] = [];
    for (const a of result.actors) {
      if (cells[pairKey(a.id, spec.column)] === "allow") {
        reach.push({
          id: `re:${a.id}:${spec.nodeId}`,
          source: `dev:${a.id}`,
          target: spec.nodeId,
          src: a.id,
          dst: targetActorId,
          kind: "reaches",
          allow: true,
          tip: spec.tip,
        });
      }
      if (spec.inbound && cells[pairKey(spec.column, a.id)] === "allow") {
        reach.push({
          id: `re:${spec.nodeId}:${a.id}`,
          source: spec.nodeId,
          target: `dev:${a.id}`,
          src: targetActorId,
          dst: a.id,
          kind: "reaches",
          allow: true,
        });
      }
    }
    if (reach.length === 0) continue;

    const node: GraphNode = {
      id: spec.nodeId,
      kind: "target",
      label: spec.label,
      sub: spec.sub,
      actorId: targetActorId,
      width: 152,
      height: 44,
      targetVariant: spec.variant,
      parentId:
        spec.variant === "external" && spec.alias ? `extframe:${spec.alias}` : undefined,
    };
    if (spec.variant === "external" && spec.alias) {
      extAliases.add(spec.alias);
      extNodes.push(node);
    } else {
      nodes.push(node);
    }
    for (const e of reach) {
      edges.push(e);
      skeleton.push(e);
    }
    // Tether the target to its serving machine(s) — router for a route, each
    // backend for a service. Structural, in the skeleton so ELK places the
    // target beside its machine; rendered thin + dashed (P2-1).
    for (const devNodeId of spec.tetherTo ?? []) {
      const machineId = Number(devNodeId.slice(4));
      const te: ClassifiedEdge = {
        id: `te:${devNodeId}:${spec.nodeId}`,
        source: devNodeId,
        target: spec.nodeId,
        src: machineId,
        dst: targetActorId,
        kind: "tether",
        allow: true,
        tip:
          spec.variant === "service"
            ? `backend for ${spec.label}`
            : spec.exit
              ? "advertises this exit node"
              : `advertises ${spec.label}`,
      };
      edges.push(te);
      skeleton.push(te);
    }
  }

  // Per-alias external-tailnet frames, pushed before their child nodes.
  for (const alias of [...extAliases].sort((a, b) => a.localeCompare(b))) {
    nodes.push({ id: `extframe:${alias}`, kind: "extframe", label: alias, width: 0, height: 0 });
  }
  nodes.push(...extNodes);
}
