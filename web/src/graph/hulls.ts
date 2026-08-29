
import type { GraphModel } from "./model";
import type { Positioned } from "./layout";

export interface Hull {
  key: string;
  label: string;
  /** SVG path `d`, in flow coordinates. */
  path: string;
  labelX: number;
  labelY: number;
}

const PAD = 26; // outward inflation from the member bounding corners
const CORNER = 18; // rounded-corner radius

type Pt = [number, number];
type Rect = { x: number; y: number; w: number; h: number };

/** Absolute position of every node id (ELK gives child coords relative to the
 *  parent container). */
function absOrigins(model: GraphModel, pos: Positioned["pos"]): Map<string, Pt> {
  const parentOf = new Map<string, string>();
  for (const n of model.nodes) if (n.parentId) parentOf.set(n.id, n.parentId);
  const cache = new Map<string, Pt | null>();
  const abs = (id: string): Pt | null => {
    if (cache.has(id)) return cache.get(id)!;
    const p = pos[id];
    if (!p) return (cache.set(id, null), null);
    const par = parentOf.get(id);
    let out: Pt | null = [p.x, p.y];
    if (par) {
      const pp = abs(par);
      out = pp ? [pp[0] + p.x, pp[1] + p.y] : null;
    }
    cache.set(id, out);
    return out;
  };
  const m = new Map<string, Pt>();
  for (const n of model.nodes) {
    const a = abs(n.id);
    if (a) m.set(n.id, a);
  }
  return m;
}

export function computeHulls(model: GraphModel, positioned: Positioned): Hull[] {
  try {
    const origin = absOrigins(model, positioned.pos);
    const byKey = new Map<string, Rect[]>();
    for (const n of model.nodes) {
      if (n.kind !== "device" || !n.groups?.length) continue;
      const o = origin.get(n.id);
      if (!o) continue;
      const sz = positioned.size[n.id] ?? { width: n.width, height: n.height };
      const rect: Rect = { x: o[0], y: o[1], w: sz.width, h: sz.height };
      for (const k of n.groups) {
        const arr = byKey.get(k) ?? byKey.set(k, []).get(k)!;
        arr.push(rect);
      }
    }

    const hulls: Hull[] = [];
    for (const [key, rects] of byKey) {
      const pts: Pt[] = [];
      for (const r of rects) {
        pts.push([r.x, r.y], [r.x + r.w, r.y], [r.x + r.w, r.y + r.h], [r.x, r.y + r.h]);
      }
      const hull = inflate(convexHull(pts), PAD);
      if (hull.length < 3) continue;
      // Label at the top-left-most vertex, lifted clear of the border.
      let anchor = hull[0];
      for (const p of hull) if (p[1] * 2 + p[0] < anchor[1] * 2 + anchor[0]) anchor = p;
      hulls.push({
        key,
        label: key,
        path: roundedPath(hull, CORNER),
        labelX: anchor[0] + 6,
        labelY: anchor[1] - 7,
      });
    }
    // Deterministic paint order so overlaps are stable across renders.
    hulls.sort((a, b) => a.key.localeCompare(b.key));
    return hulls;
  } catch {
    return [];
  }
}

/** Andrew's monotone chain. Returns the hull in order; < 3 unique points come
 *  back as-is. */
function convexHull(points: Pt[]): Pt[] {
  const pts = [...points].sort((a, b) => a[0] - b[0] || a[1] - b[1]);
  const uniq: Pt[] = [];
  for (const p of pts) {
    const last = uniq[uniq.length - 1];
    if (!last || last[0] !== p[0] || last[1] !== p[1]) uniq.push(p);
  }
  if (uniq.length < 3) return uniq;
  const cross = (o: Pt, a: Pt, b: Pt) =>
    (a[0] - o[0]) * (b[1] - o[1]) - (a[1] - o[1]) * (b[0] - o[0]);
  const lower: Pt[] = [];
  for (const p of uniq) {
    while (lower.length >= 2 && cross(lower[lower.length - 2], lower[lower.length - 1], p) <= 0)
      lower.pop();
    lower.push(p);
  }
  const upper: Pt[] = [];
  for (let i = uniq.length - 1; i >= 0; i--) {
    const p = uniq[i];
    while (upper.length >= 2 && cross(upper[upper.length - 2], upper[upper.length - 1], p) <= 0)
      upper.pop();
    upper.push(p);
  }
  lower.pop();
  upper.pop();
  return lower.concat(upper);
}

/** Offset a convex polygon outward by `d`, winding-agnostic (each edge is moved
 *  along the normal that points away from the centroid, then adjacent offset
 *  edges are intersected). */
function inflate(poly: Pt[], d: number): Pt[] {
  const n = poly.length;
  if (n < 3) return poly;
  const cx = poly.reduce((s, p) => s + p[0], 0) / n;
  const cy = poly.reduce((s, p) => s + p[1], 0) / n;
  const lines: [Pt, Pt][] = [];
  for (let i = 0; i < n; i++) {
    const p1 = poly[i];
    const p2 = poly[(i + 1) % n];
    const dx = p2[0] - p1[0];
    const dy = p2[1] - p1[1];
    const len = Math.hypot(dx, dy) || 1;
    let nx = -dy / len;
    let ny = dx / len;
    const mx = (p1[0] + p2[0]) / 2;
    const my = (p1[1] + p2[1]) / 2;
    if ((mx - cx) * nx + (my - cy) * ny < 0) {
      nx = -nx;
      ny = -ny;
    }
    lines.push([
      [p1[0] + nx * d, p1[1] + ny * d],
      [p2[0] + nx * d, p2[1] + ny * d],
    ]);
  }
  const out: Pt[] = [];
  for (let i = 0; i < n; i++) {
    const a = lines[(i - 1 + n) % n];
    const b = lines[i];
    out.push(lineIntersect(a[0], a[1], b[0], b[1]) ?? poly[i]);
  }
  return out;
}

function lineIntersect(a: Pt, b: Pt, c: Pt, e: Pt): Pt | null {
  const d = (b[0] - a[0]) * (e[1] - c[1]) - (b[1] - a[1]) * (e[0] - c[0]);
  if (Math.abs(d) < 1e-6) return null;
  const t = ((c[0] - a[0]) * (e[1] - c[1]) - (c[1] - a[1]) * (e[0] - c[0])) / d;
  return [a[0] + t * (b[0] - a[0]), a[1] + t * (b[1] - a[1])];
}

/** An SVG path around `poly` with quadratic-bezier rounded corners. */
function roundedPath(poly: Pt[], r: number): string {
  const n = poly.length;
  if (n < 3) return "";
  const sub = (p: Pt, q: Pt): Pt => [p[0] - q[0], p[1] - q[1]];
  const dist = (p: Pt, q: Pt) => Math.hypot(p[0] - q[0], p[1] - q[1]);
  let d = "";
  for (let i = 0; i < n; i++) {
    const prev = poly[(i - 1 + n) % n];
    const cur = poly[i];
    const next = poly[(i + 1) % n];
    const dPrev = dist(prev, cur) || 1;
    const dNext = dist(next, cur) || 1;
    const rp = Math.min(r, dPrev / 2);
    const rn = Math.min(r, dNext / 2);
    const vp = sub(prev, cur);
    const vn = sub(next, cur);
    const p1: Pt = [cur[0] + (vp[0] / dPrev) * rp, cur[1] + (vp[1] / dPrev) * rp];
    const p2: Pt = [cur[0] + (vn[0] / dNext) * rn, cur[1] + (vn[1] / dNext) * rn];
    d += `${i === 0 ? "M" : "L"} ${p1[0].toFixed(1)} ${p1[1].toFixed(1)} `;
    d += `Q ${cur[0].toFixed(1)} ${cur[1].toFixed(1)} ${p2[0].toFixed(1)} ${p2[1].toFixed(1)} `;
  }
  return d + "Z";
}
