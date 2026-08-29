// Builds the graph from an engine Result: device + group + internet nodes, a
// stable "skeleton" edge set for layout, and a classified display-edge set.

import type { Actor, EngineResult, Verdict } from "../engine/types";
import { pairKey, splitSSHKey } from "../engine/keys";
import type { Grouping, PairSelection } from "../state/store";
import type { EdgeKind } from "./style";
import { internetEgress } from "./internet";
import { addTargets } from "./targets";

export const INTERNET_ID = -1;

export type GraphNodeKind = "device" | "group" | "internet" | "target" | "extframe";
export type TargetVariant = "route" | "service" | "external";

export interface GraphNode {
  id: string; // "dev:1" | "grp:alice@x" | "net" | "tgt:route:10.0.0.0/24@3"
  kind: GraphNodeKind;
  label: string;
  sub?: string;
  actorId?: number;
  parentId?: string;
  /** Every identity cluster a device belongs to (owner email, or each tag).
   *  `parentId` is the primary (stable, lexically first) one used for ELK
   *  layout; the full set drives the hull regions (P2-5). */
  groups?: string[];
  width: number;
  height: number;
  targetVariant?: TargetVariant;
}

export interface ClassifiedEdge {
  id: string;
  source: string; // node id
  target: string; // node id
  src: number; // actor id
  dst: number; // actor id or INTERNET_ID
  kind: EdgeKind;
  allow: boolean;
  /** Overrides the hover text GraphView would otherwise derive. Set on
   *  `reaches` edges so the route/service tooltip can name the serving machine
   *  (P2-2: `via` is an annotation, not a hop). */
  tip?: string;
}

export interface GraphModel {
  nodes: GraphNode[];
  /** The edges the layout is computed from — allow edges plus structural
   *  `tether` links (target → serving machine). Never filtered by UI toggles,
   *  so the layout stays stable while the user toggles focus / emphasize. */
  skeleton: ClassifiedEdge[];
  edges: ClassifiedEdge[];
  hasInternet: boolean;
  signature: string; // changes only when a relayout is actually needed
}

/** The display-edge id for a device→device pair, or a device's internet
 *  egress (`"net"`). pairOfEdgeId reads the pair back. */
const deviceEdgeId = (src: number, dst: number | "net") => `e:${src}-${dst}`;

/** A display edge from device `src` to device `dst`, or to the internet node. */
function deviceEdge(src: number, dst: number | "net", kind: EdgeKind, allow: boolean): ClassifiedEdge {
  return {
    id: deviceEdgeId(src, dst),
    source: `dev:${src}`,
    target: dst === "net" ? "net" : `dev:${dst}`,
    src,
    dst: dst === "net" ? INTERNET_ID : dst,
    kind,
    allow,
  };
}

/** The device pair a device→device edge id names; null for every other edge
 *  (internet egress, route / service / shared-node reach, tethers). */
export function pairOfEdgeId(id: string): PairSelection | null {
  if (!id.startsWith("e:")) return null;
  const [a, b] = id.slice(2).split("-");
  if (!a || !b || b === "net") return null;
  return { src: Number(a), dst: Number(b) };
}

const DEVICE_W = 158;
const DEVICE_H = 46;

/**
 * Every identity cluster an actor belongs to. A machine's identity is its owner
 * email XOR its tag set — never a catch-all bucket:
 *   - "user":  user-owned → [owner email];  tag-owned → [] (floats, no cluster)
 *   - "tag":   one entry per tag;           user-owned → [] (floats)
 * Multi-tag machines legitimately belong to several clusters at once.
 */
function groupKeysOf(a: Actor, grouping: Grouping): string[] {
  if (grouping === "flat") return [];
  if (grouping === "user") return a.kind === "user" && a.user ? [a.user] : [];
  return a.tags ?? [];
}

/** The one cluster used as the ELK parent — stable across renders (lexically
 *  first) so multi-tag machines don't jump between boxes. */
function primaryGroupKey(a: Actor, grouping: Grouping): string | null {
  const keys = groupKeysOf(a, grouping);
  if (keys.length === 0) return null;
  return keys.reduce((lo, k) => (k < lo ? k : lo));
}

/**
 * Resolve a `group:` name to the set of owner emails in it (lower-cased),
 * following nested `group:` members. Cycles and unknown groups resolve to
 * whatever is reachable — never throw. `autogroup:` / other tokens are skipped
 * (they don't name concrete owners). Used by the graph's group-highlight
 * overlay (P2-6 / G-F).
 */
export function resolveGroupOwners(
  groups: Record<string, string[]>,
  name: string,
): Set<string> {
  const out = new Set<string>();
  const seen = new Set<string>();
  const visit = (g: string) => {
    if (seen.has(g)) return;
    seen.add(g);
    for (const m of groups[g] ?? []) {
      if (m.startsWith("group:")) visit(m);
      else if (m.includes("@")) out.add(m.toLowerCase());
    }
  };
  visit(name.startsWith("group:") ? name : `group:${name}`);
  return out;
}

/** Union several probe matrices: a cell is "allow" if allowed on ANY of them. */
function unionCells(result: EngineResult, probes: string[]): Record<string, Verdict> {
  const out: Record<string, Verdict> = {};
  for (const p of probes) {
    const m = result.matrices[p];
    if (!m) continue;
    for (const [k, v] of Object.entries(m)) {
      if (v === "allow" || !(k in out)) out[k] = v;
    }
  }
  return out;
}

export function buildGraphModel(
  result: EngineResult,
  grouping: Grouping,
  probe: string,
  /** When set (non-empty), reachability is the UNION over these probes — an
   *  edge is drawn if the pair is allowed on ANY of them ("any port" view). */
  unionProbes?: string[],
): GraphModel {
  const actors = [...result.actors].sort((a, b) => a.name.localeCompare(b.name));
  const anyPort = !!unionProbes && unionProbes.length > 0;
  const cells = anyPort
    ? unionCells(result, unionProbes!)
    : (result.matrices[probe] ?? {});
  const nodes: GraphNode[] = [];

  const keysOf = new Map(actors.map((a) => [a.id, groupKeysOf(a, grouping)] as const));
  const primaryOf = new Map(
    actors.map((a) => [a.id, primaryGroupKey(a, grouping)] as const),
  );

  // Identity-cluster container nodes — one per distinct owner / tag. No
  // catch-all bucket: a machine with no cluster simply floats.
  if (grouping !== "flat") {
    const keys = new Set<string>();
    for (const a of actors) for (const k of keysOf.get(a.id)!) keys.add(k);
    for (const k of [...keys].sort((x, y) => x.localeCompare(y))) {
      nodes.push({ id: `grp:${k}`, kind: "group", label: k, width: 0, height: 0 });
    }
  }

  // Device nodes.
  for (const a of actors) {
    const keys = keysOf.get(a.id)!;
    const primary = primaryOf.get(a.id);
    nodes.push({
      id: `dev:${a.id}`,
      kind: "device",
      label: a.name,
      sub: a.kind === "user" ? a.user : a.tags?.join(", "),
      actorId: a.id,
      parentId: primary ? `grp:${primary}` : undefined,
      groups: keys.length ? keys : undefined,
      width: DEVICE_W,
      height: DEVICE_H,
    });
  }

  const egress = internetEgress(result);
  const hasInternet = egress.size > 0;
  if (hasInternet) {
    nodes.push({
      id: "net",
      kind: "internet",
      label: "internet",
      actorId: INTERNET_ID,
      width: 120,
      height: 46,
    });
  }

  const clusterSets = new Map(
    actors.map((a) => [a.id, new Set(keysOf.get(a.id))] as const),
  );
  const sharesCluster = (x: number, y: number) => {
    const gx = clusterSets.get(x)!;
    for (const k of clusterSets.get(y)!) if (gx.has(k)) return true;
    return false;
  };
  const sshPair = sshPairs(result);

  const skeleton: ClassifiedEdge[] = [];
  const edges: ClassifiedEdge[] = [];

  const denials = result.posture?.denials ?? {};

  for (const src of actors) {
    for (const dst of actors) {
      if (src.id === dst.id) continue;
      const key = pairKey(src.id, dst.id);
      const allow = cells[key] === "allow";
      const ssh = sshPair.get(key);
      if (!allow && !ssh) {
        // Denied pair: a display edge only, never in the skeleton.
        edges.push(deviceEdge(src.id, dst.id, denials[key] ? "posture" : "deny", false));
        continue;
      }
      // In flat mode there are no clusters, so no within / boundary distinction.
      // Otherwise a pair is "within" if it shares ANY identity cluster.
      const crosses = grouping === "flat" || !sharesCluster(src.id, dst.id);
      const kind: EdgeKind = ssh && !allow ? "ssh" : crosses ? "boundary" : "within";
      const e = deviceEdge(src.id, dst.id, kind, allow);
      edges.push(e);
      if (allow) skeleton.push(e);
    }
    if (egress.has(src.id)) {
      const e = deviceEdge(src.id, "net", "leaves", true);
      edges.push(e);
      skeleton.push(e);
    }
  }

  // Multi-cluster membership is shown by the hull regions (P2-5, `hulls.ts`) —
  // `GraphNode.groups` carries every cluster a device belongs to. `primaryOf`
  // stays the ELK parent only.

  addTargets(result, cells, nodes, skeleton, edges);

  const signature = JSON.stringify({
    g: grouping,
    p: anyPort ? "any" : probe,
    n: nodes.map((n) => n.id),
    s: skeleton.map((e) => `${e.source}>${e.target}`),
  });

  return { nodes, skeleton, edges, hasInternet, signature };
}

/** "src->dst" for every pair that has any SSH accept/check verdict. */
function sshPairs(result: EngineResult): Map<string, "accept" | "check"> {
  const m = new Map<string, "accept" | "check">();
  for (const [k, v] of Object.entries(result.ssh ?? {})) {
    if (v !== "accept" && v !== "check") continue;
    const [, pair] = splitSSHKey(k);
    if (!pair) continue;
    if (v === "accept" || !m.has(pair)) m.set(pair, v);
  }
  return m;
}
