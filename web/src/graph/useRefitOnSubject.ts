import { useEffect, useRef } from "react";
import { useReactFlow } from "@xyflow/react";
import type { Actor } from "../engine/types";
import { useMediaQuery, MOBILE_QUERY } from "../app/useMediaQuery";
import type { Positioned } from "./layout";

/** fitView padding when a new subject's layout lands. Two passes: once RF has
 *  the nodes, again once the strip / legend below have taken their height. */
const REFIT_PADDING = 0.1;
const REFIT_FIRST_MS = 60;
const REFIT_SETTLED_MS = 350;
const REFIT_SETTLED_DURATION_MS = 150;

/**
 * The `fitView` prop only fires at init — by then the async layout hasn't
 * landed, so the graph can open scrolled off the nodes. Re-fit once a fresh
 * layout is in whenever the graph's *subject* changes: a new lesson, or a
 * different tailnet / preset (device set) loaded from the Tailnet tab. Pure
 * regroups, filter toggles and grant-only policy edits keep the device set, so
 * they keep the user's pan/zoom. On mobile every new layout re-fits.
 */
export function useRefitOnSubject(
  activeLesson: string | null,
  actors: Actor[] | undefined,
  layout: { ready: boolean; positioned: Positioned | null; layoutSig: string },
  nodeCount: number,
) {
  const isMobile = useMediaQuery(MOBILE_QUERY);
  const { fitView } = useReactFlow();
  // Starts true so the first real (async) layout always gets a fit — the
  // `fitView` prop only runs at init, before that layout has landed.
  const refitPending = useRef(true);
  const subjectSig =
    (activeLesson ?? "") +
    "|" +
    (actors ?? [])
      .map((a) => `${a.id}:${a.name}:${a.kind}:${(a.tags ?? []).join("+")}`)
      .join(",");
  const prevSubject = useRef(subjectSig);
  if (prevSubject.current !== subjectSig) {
    prevSubject.current = subjectSig;
    refitPending.current = true;
  }
  const fitTimers = useRef<number[]>([]);
  useEffect(() => () => fitTimers.current.forEach((t) => clearTimeout(t)), []);
  const { ready, positioned, layoutSig } = layout;
  useEffect(() => {
    // Only fit once the layout that matches the *current* model has landed —
    // firing against a stale layout (the window between an eval and its
    // relayout) is what left the graph parked off the new nodes.
    if (!ready || !positioned || nodeCount === 0) return;
    if (!isMobile && !refitPending.current) return;
    refitPending.current = false;
    // The timers are NOT cancelled on a plain re-render (e.g. nodeCount
    // settling from the old count to the new) — only when a newer fit
    // supersedes them or the pane unmounts — so a mid-flight relayout can't
    // strand the request.
    fitTimers.current.forEach((t) => clearTimeout(t));
    fitTimers.current = [
      window.setTimeout(() => void fitView({ padding: REFIT_PADDING, duration: 0 }), REFIT_FIRST_MS),
      window.setTimeout(
        () => void fitView({ padding: REFIT_PADDING, duration: REFIT_SETTLED_DURATION_MS }),
        REFIT_SETTLED_MS,
      ),
    ];
  }, [isMobile, ready, positioned, nodeCount, layoutSig, fitView]);
}
