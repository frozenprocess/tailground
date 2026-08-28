export type Grouping = "flat" | "user" | "tag";
export type ImpactView = "matrix" | "graph";
/**
 * The single value a URL fragment / share link carries. It is *derived* from the
 * two orthogonal UI axes: `leftTab` for the left panel, and
 * `view` itself ("matrix" | "graph") for the right — see `canonicalView`.
 */
export type MainView = ImpactView | "tutorials" | "about";
/**
 * Which lens the left workspace column shows. The policy editor no longer lives
 * here — it is a permanent pane at the top of the right column.
 * The About reference is a modal, `aboutOpen`, not a tab.
 */
export type LeftTab = "tailnet" | "learn";
/**
 * The single visible pane in the stacked mobile layout (below `MOBILE_QUERY`).
 * Unifies the two desktop axes plus the always-on editor into one selector, so
 * the bottom nav can drive them all. `setMobilePane` mirrors the choice back
 * onto `leftTab` / `view` so the desktop layout stays consistent on resize.
 */
export type MobilePane = "policy" | "tailnet" | "matrix" | "graph" | "learn";

/**
 * Collapse the two UI axes into the single `MainView` a URL fragment / share
 * link carries. The Learn lens wins the URL when it is active — a lesson + the
 * graph is a valid in-session state, but the link points at the lesson.
 */
export function canonicalView(s: { leftTab: LeftTab; view: MainView }): MainView {
  if (s.leftTab === "learn") return "tutorials";
  return s.view === "graph" ? "graph" : "matrix";
}

/** Shared URL/share mapping for the impact pane; Learn/About use matrix underneath. */
export function impactView(view: MainView): ImpactView {
  return view === "graph" ? "graph" : "matrix";
}
