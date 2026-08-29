// ELK layout, run once per skeleton signature and cached so that toggling
// focus / emphasize never triggers a relayout.
//
// The graph is edge-free at rest — flows only draw on hover — so a flow-shaped
// layout buys nothing and ELK's layered algorithm columns badly (it stacks
// disconnected components, and grouped mode makes every cluster one). So the
// default is "rectpacking": cluster boxes + loose nodes tiled into a ~16:10
// rectangle, no edge influence. Only external-tailnet scenarios keep "layered"
// — they need its periphery partition and are always small.

import ELK, { type ElkNode } from "elkjs/lib/elk.bundled.js";
import type { GraphModel, GraphNode } from "./model";

const elk = new ELK();

export interface Positioned {
  /** node id → absolute-ish position (child positions are relative to parent). */
  pos: Record<string, { x: number; y: number }>;
  /** group id → measured size */
  size: Record<string, { width: number; height: number }>;
}

const GROUP_PAD = 24;
const GROUP_HEADER = 22;

const cache = new Map<string, Positioned>();

export async function layoutGraph(model: GraphModel): Promise<Positioned> {
  const hit = cache.get(model.signature);
  if (hit) return hit;

  const isContainer = (n: GraphNode) => n.kind === "group" || n.kind === "extframe";
  const groups = model.nodes.filter(isContainer);
  const loose = model.nodes.filter((n) => !isContainer(n) && !n.parentId);
  // Partition externals into their own band so ELK keeps them on the periphery,
  // not threaded through the local graph (P2-3). Only when externals exist —
  // no behaviour change for the common case.
  const hasExt = model.nodes.some((n) => n.kind === "extframe");
  const partitionOf = (n: GraphNode) => (n.kind === "extframe" ? 1 : 0);
  const childrenByGroup = new Map<string, GraphNode[]>();
  for (const n of model.nodes) {
    if (n.parentId) {
      const arr = childrenByGroup.get(n.parentId) ?? [];
      arr.push(n);
      childrenByGroup.set(n.parentId, arr);
    }
  }

  const pack = !hasExt;

  const elkGraph: ElkNode = {
    id: "root",
    layoutOptions: pack
      ? {
          "elk.algorithm": "rectpacking",
          "elk.aspectRatio": "1.6",
          "elk.spacing.nodeNode": "30",
          "elk.padding": "[top=12,left=12,bottom=12,right=12]",
        }
      : {
          "elk.algorithm": "layered",
          "elk.direction": "RIGHT",
          "elk.hierarchyHandling": "INCLUDE_CHILDREN",
          "elk.layered.spacing.nodeNodeBetweenLayers": "70",
          "elk.spacing.nodeNode": "26",
          "elk.aspectRatio": "1.7",
          "elk.padding": "[top=12,left=12,bottom=12,right=12]",
          ...(hasExt ? { "elk.partitioning.activate": "true" } : {}),
        },
    children: [
      ...groups.map<ElkNode>((g) => ({
        id: g.id,
        layoutOptions: {
          "elk.padding": `[top=${GROUP_PAD + GROUP_HEADER},left=${GROUP_PAD},bottom=${GROUP_PAD},right=${GROUP_PAD}]`,
          "elk.spacing.nodeNode": pack ? "16" : "20",
          // The packed root has no hierarchy handling, so each cluster lays
          // itself out — box tiles its handful of devices compactly.
          ...(pack ? { "elk.algorithm": "box", "elk.aspectRatio": "1.3" } : {}),
          ...(hasExt ? { "elk.partitioning.partition": String(partitionOf(g)) } : {}),
        },
        children: (childrenByGroup.get(g.id) ?? []).map<ElkNode>((c) => ({
          id: c.id,
          width: c.width,
          height: c.height,
        })),
      })),
      ...loose.map<ElkNode>((n) => ({
        id: n.id,
        width: n.width,
        height: n.height,
        ...(hasExt ? { layoutOptions: { "elk.partitioning.partition": "0" } } : {}),
      })),
    ],
    edges: pack
      ? []
      : model.skeleton.map((e) => ({
          id: e.id,
          sources: [e.source],
          targets: [e.target],
        })),
  };

  const res = await elk.layout(elkGraph);

  const pos: Positioned["pos"] = {};
  const size: Positioned["size"] = {};
  const walk = (n: ElkNode) => {
    if (n.id !== "root") {
      pos[n.id] = { x: n.x ?? 0, y: n.y ?? 0 };
      if (n.width && n.height) size[n.id] = { width: n.width, height: n.height };
    }
    n.children?.forEach(walk);
  };
  walk(res);

  const out = { pos, size };
  cache.set(model.signature, out);
  if (cache.size > 24) cache.delete(cache.keys().next().value!);
  return out;
}
