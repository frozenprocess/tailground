// The one style table. Edge builders in model.ts read it; Legend.tsx renders it.
// Nothing else should hard-code an edge colour.

export type EdgeKind =
  | "within" // allow, both ends share an identity cluster
  | "boundary" // allow, ends in different clusters
  | "leaves" // allow to the internet zone (autogroup:internet egress)
  | "reaches" // allow to a Tailground-layer target (route / service VIP / shared node)
  | "tether" // structural: a target node hangs off its serving machine (router / backend)
  | "ssh" // an SSH accept/check path
  | "posture" // would be allowed, but the src device fails a posture gate
  | "deny"; // denied (only drawn in emphasize / all mode)

export interface EdgeStyle {
  label: string;
  stroke: string;
  width: number;
  dashed?: boolean;
  animated?: boolean;
  /** When emphasize mode is on, edges with fade=true drop to this opacity. */
  fadeWhenEmphasized?: boolean;
}

// Colours mirror peacha's action palette: green = allowed, red = denied,
// blue = leaves the tailnet, grey = structural. An allow is an allow in a
// tailnet — crossing a group boundary isn't its own state — so same-group and
// cross-group allows are the same green, separated only by width. SSH (purple)
// and posture (orange) have no peacha equivalent and keep their own accent.
export const EDGE_STYLES: Record<EdgeKind, EdgeStyle> = {
  within: {
    label: "allow (same group)",
    stroke: "var(--flow-allow)",
    width: 1,
    fadeWhenEmphasized: true,
  },
  boundary: {
    label: "allow (crosses group)",
    stroke: "var(--flow-allow)",
    width: 1.75,
  },
  leaves: {
    label: "leaves tailnet",
    stroke: "var(--flow-egress)",
    width: 1.75,
    dashed: true,
  },
  reaches: {
    label: "reaches route / service / shared node",
    stroke: "var(--flow-allow)",
    width: 1.75,
  },
  tether: {
    label: "served by (router / backend)",
    stroke: "var(--flow-struct)",
    width: 1,
    dashed: true,
    fadeWhenEmphasized: true,
  },
  ssh: {
    label: "SSH accept / check",
    stroke: "var(--color-purple)",
    width: 1.5,
    dashed: true,
  },
  posture: {
    label: "blocked by posture",
    stroke: "var(--color-check)",
    width: 1.5,
    dashed: true,
  },
  deny: {
    label: "denied (emphasize / all)",
    stroke: "var(--flow-deny)",
    width: 1,
    dashed: true,
    fadeWhenEmphasized: true,
  },
};

export const FADE_OPACITY = 0.14;
/** A drawn edge that is neither faded nor highlighted (highlighted is 1). */
export const REST_OPACITY = 0.85;

/** strokeDasharray of a `dashed` edge on the canvas … */
export const EDGE_DASH = "5 4";
/** … and of its shorter legend swatch. */
export const LEGEND_DASH = "4 3";

export interface NodeStyleToken {
  label: string;
  swatch: string;
}

export const NODE_LEGEND: NodeStyleToken[] = [
  { label: "user device", swatch: "var(--color-paper)" },
  { label: "tagged device", swatch: "var(--color-linen)" },
  { label: "identity-cluster region (owner / tag; devices sit in several)", swatch: "transparent" },
  { label: "internet", swatch: "var(--color-deny-bg)" },
  { label: "route / service / shared node", swatch: "var(--color-allow-bg)" },
];
