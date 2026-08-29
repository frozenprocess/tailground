import { pairKey } from "../engine/keys";
import type { PairSelection } from "../state/store";
import type { EdgeKind } from "./style";
import type { ClassifiedEdge, GraphModel } from "./model";

export interface EdgeFilterState {
  emphasize: boolean;
  /** The actor the graph is focused on — a pinned node click, or the node the
   *  pointer is currently over. Isolates that node's neighbourhood. */
  focusedActor: number | null;
  /** A focused pair — a pinned edge click, or the edge under the pointer.
   *  Isolates both endpoints and highlights the exact flow. */
  selectedPair: PairSelection | null;
}

export type VisibleEdge = ClassifiedEdge & {
  faded: boolean;
  highlighted: boolean;
  /** Both directions are allowed with the same kind — drawn as one edge with
   *  an arrowhead at each end instead of two overlapping curves. */
  bidir: boolean;
};

/**
 * Decide which classified edges are visible and whether each is de-emphasised.
 *
 * Resting state (no focus): the whole mesh, with `emphasize` fading the routine
 * within-group / structural edges (GraphView itself draws no edges at rest).
 * Focusing a node (hover or pin) drops to that node's neighbourhood — including
 * the deny / posture edges that explain why a peer is *not* reachable. Focusing
 * an edge isolates its two endpoints.
 */
export function visibleEdges(model: GraphModel, st: EdgeFilterState): VisibleEdge[] {
  const focus = st.focusedActor;
  const sel = st.selectedPair;
  const touchesSel = (e: ClassifiedEdge) =>
    sel != null &&
    (e.src === sel.src || e.dst === sel.dst || e.src === sel.dst || e.dst === sel.src);
  const isExactSel = (e: ClassifiedEdge) =>
    sel != null && e.src === sel.src && e.dst === sel.dst;
  const touchesFocus = (e: ClassifiedEdge) =>
    focus != null && (e.src === focus || e.dst === focus);

  const vis = model.edges
    .filter((e) => {
      // Denied / posture-blocked edges only appear to explain a focused subject.
      if (e.kind === "deny" || e.kind === "posture")
        return touchesFocus(e) || isExactSel(e);

      if (focus != null) return touchesFocus(e);
      if (sel != null) return touchesSel(e);
      return true; // resting: the full mesh, decluttered by `emphasize`
    })
    .map((e) => {
      const touches = touchesFocus(e);
      const highlighted = isExactSel(e) || (touches && e.kind !== "deny");

      let faded = false;
      if (focus != null && !touches) faded = true;
      else if (sel != null && focus == null && !isExactSel(e)) faded = true;
      else if (
        st.emphasize &&
        (e.kind === "within" || e.kind === "tether") &&
        !highlighted
      )
        faded = true;

      return { ...e, faded, highlighted, bidir: false };
    });
  return mergeReverse(vis, focus, isExactSel);
}

/**
 * Collapse A→B + B→A of the same kind into one `bidir` edge. The kept half is
 * the one leaving the focused node (or the exact selected flow), so its id —
 * which a click turns into a pair selection — reads from the subject outward.
 * Tethers are structural and undirected; they're left alone.
 */
function mergeReverse(
  vis: VisibleEdge[],
  focus: number | null,
  isExactSel: (e: ClassifiedEdge) => boolean,
): VisibleEdge[] {
  const byPair = new Map<string, VisibleEdge>();
  for (const e of vis) byPair.set(pairKey(e.src, e.dst), e);
  // Both halves of a merged pair are consumed — the kept one too, or when it's
  // the reverse half the loop would reach it again and emit it a second time
  // (a duplicate RF edge id, which React can't reconcile: the stale path stays
  // drawn after the focus clears).
  const merged = new Set<VisibleEdge>();
  const out: VisibleEdge[] = [];
  for (const e of vis) {
    if (merged.has(e)) continue;
    const rev = byPair.get(pairKey(e.dst, e.src));
    if (!rev || rev === e || rev.kind !== e.kind || e.kind === "tether" || merged.has(rev)) {
      out.push(e);
      continue;
    }
    const keepRev = rev.src === focus || (isExactSel(rev) && e.src !== focus);
    const [keep, drop] = keepRev ? [rev, e] : [e, rev];
    merged.add(keep).add(drop);
    out.push({
      ...keep,
      bidir: true,
      faded: keep.faded && drop.faded,
      highlighted: keep.highlighted || drop.highlighted,
    });
  }
  return out;
}

/**
 * The focused node's reach on the current probe: distinct peers it can open a
 * connection to (`out`) and distinct peers that can open one to it (`in`).
 * Allow + SSH paths only — denies and posture-blocked pairs aren't reach.
 */
export function reachCounts(model: GraphModel, actor: number): { out: number; in: number } {
  const outs = new Set<number>();
  const ins = new Set<number>();
  for (const e of model.edges) {
    if (!REACH_KINDS.has(e.kind)) continue;
    if (e.src === actor) outs.add(e.dst);
    if (e.dst === actor) ins.add(e.src);
  }
  return { out: outs.size, in: ins.size };
}

const REACH_KINDS = new Set<EdgeKind>(["within", "boundary", "leaves", "reaches", "ssh"]);
