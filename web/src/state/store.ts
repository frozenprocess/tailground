import { create } from "zustand";
import type { EngineResult, Probe, Snapshot } from "../engine/types";
import { defaultProbes, DEFAULT_SSH_USERS } from "../engine/types";
import { splitSSHKey } from "../engine/keys";
import { DEFAULT_PRESET, DEFAULT_TEMPLATE } from "./presets";
import type { EngineStatus } from "../engine/client";
import { readThemePref, resolveTheme, persistThemePref, type ThemePref } from "./theme";
import { clamp, readLayout, persistLayout, type LayoutState } from "./layout";
import type { PendingShare, SharedState } from "../app/share";

import { impactView, type Grouping, type ImpactView, type MainView, type LeftTab, type MobilePane } from "./navigation";
export { canonicalView } from "./navigation";
export type { Grouping, ImpactView, MainView, LeftTab, MobilePane } from "./navigation";

export type { ThemePref } from "./theme";
export type { LayoutState } from "./layout";

/** A src→dst selection the user clicked in the matrix. */
export interface PairSelection {
  src: number;
  dst: number;
}

interface AppState {
  // Inputs
  policy: string;
  snapshot: Snapshot;
  probes: Probe[];
  sshUsers: string[];

  // Engine output
  result: EngineResult | null;
  evalMs: number;
  engineStatus: EngineStatus;
  evaluating: boolean;

  // UI — two orthogonal axes. The left panel (`leftTab`) and the right panel (`view`, only ever "matrix" | "graph")
  // move independently; `canonicalView` collapses them for the URL / share link.
  view: ImpactView;
  leftTab: LeftTab;
  /** The one visible pane on the mobile layout — see `MobilePane`. */
  mobilePane: MobilePane;
  activeProbe: string;
  grouping: Grouping;
  selectedPair: PairSelection | null;

  // Graph
  graphEmphasize: boolean;
  graphHideIsolated: boolean;
  /** Graph only: reachability is the union over every configured L4 probe, so a
   *  grant on a port you're not currently viewing still shows an edge. */
  graphAnyProbe: boolean;
  /** Graph only: a `group:` name to highlight — every device whose owner
   *  resolves into that group is ringed. Session-only, not shared. */
  graphHighlightGroup: string | null;
  /** The device the graph is focused on (node click / matrix row hover). */
  focusedActor: number | null;

  // Tutorials
  activeLesson: string | null;
  completedLessons: Set<string>;

  /** True until dismissed, when the scenario was hydrated from a #s= link. */
  loadedFromShare: boolean;
  shareFailed: boolean;
  /** A share link awaiting the visitor's consent; nothing from it is loaded until accepted. */
  pendingShare: PendingShare | null;

  // Theme
  themePref: ThemePref;
  themeResolved: "light" | "dark";

  // Workspace layout (L-D)
  layout: LayoutState;

  // Actions
  setPolicy: (policy: string) => void;
  setSnapshot: (snapshot: Snapshot) => void;
  patchSnapshot: (patch: Partial<Snapshot>) => void;
  loadTemplate: (policy: string) => void;
  loadPreset: (snapshot: Snapshot) => void;
  /** Load a matched policy + snapshot (a Scenario). */
  loadScenario: (policy: string, snapshot: Snapshot) => void;
  setProbes: (probes: Probe[]) => void;
  setSSHUsers: (users: string[]) => void;

  setResult: (result: EngineResult, ms: number) => void;
  setEngineStatus: (s: EngineStatus) => void;
  setEvaluating: (b: boolean) => void;

  setView: (v: ImpactView) => void;
  setLeftTab: (t: LeftTab) => void;
  aboutOpen: boolean;
  setAboutOpen: (open: boolean) => void;
  /** A lesson "Show me" step moved the view; offer a way back to the lesson. */
  tourNote: boolean;
  setTourNote: (b: boolean) => void;
  setMobilePane: (p: MobilePane) => void;
  /** Apply a MainView (from the URL or a share link) to the orthogonal axes. */
  applyRoute: (v: MainView, lesson: string | null) => void;
  setActiveProbe: (p: string) => void;
  setGrouping: (g: Grouping) => void;
  selectPair: (p: PairSelection | null) => void;

  setGraphEmphasize: (b: boolean) => void;
  setGraphHideIsolated: (b: boolean) => void;
  setGraphAnyProbe: (b: boolean) => void;
  setGraphHighlightGroup: (g: string | null) => void;
  setFocusedActor: (id: number | null) => void;

  setActiveLesson: (slug: string | null) => void;
  markLessonComplete: (slug: string) => void;

  hydrateFromShare: (s: SharedState) => void;
  dismissShareBanner: () => void;
  failShare: () => void;
  setPendingShare: (p: PendingShare | null) => void;
  /** Discard the working scenario (and any loaded share) and show the built-in default playground. */
  startFresh: () => void;

  setThemePref: (pref: ThemePref) => void;
  /** Re-derive themeResolved from the OS (called on a live prefers-color-scheme change). */
  syncResolvedTheme: () => void;

  setLayoutCols: (cols: number[]) => void;
  setLayoutRows: (rows: number[]) => void;
  setStripFraction: (frac: number) => void;
  toggleMaximized: (id: string) => void;
  setPanelCollapsed: (id: string, collapsed: boolean) => void;
}

export const useStore = create<AppState>((set) => ({
  policy: DEFAULT_TEMPLATE.policy,
  snapshot: DEFAULT_PRESET.snapshot,
  probes: defaultProbes(),
  sshUsers: DEFAULT_SSH_USERS,

  result: null,
  evalMs: 0,
  engineStatus: "loading",
  evaluating: false,

  view: "graph",
  leftTab: "learn",
  aboutOpen: false,
  tourNote: false,
  mobilePane: "learn",
  activeProbe: "https",
  grouping: "user",
  selectedPair: null,

  graphEmphasize: true,
  graphHideIsolated: false,
  graphAnyProbe: true,
  graphHighlightGroup: null,
  focusedActor: null,

  activeLesson: null,
  completedLessons: new Set<string>(),
  loadedFromShare: false,
  shareFailed: false,
  pendingShare: null,

  themePref: readThemePref(),
  themeResolved: resolveTheme(readThemePref()),

  layout: readLayout(),

  setPolicy: (policy) => set({ policy }),
  setSnapshot: (snapshot) => set({ snapshot }),
  patchSnapshot: (patch) => set((s) => ({ snapshot: { ...s.snapshot, ...patch } })),
  loadTemplate: (policy) => set({ policy, selectedPair: null }),
  loadPreset: (snapshot) => set({ snapshot, selectedPair: null }),
  loadScenario: (policy, snapshot) =>
    set({ policy, snapshot, selectedPair: null, focusedActor: null }),
  setProbes: (probes) => set({ probes }),
  setSSHUsers: (sshUsers) => set({ sshUsers }),

  setResult: (result, evalMs) =>
    set((s) => {
      // Keep the active probe valid as the probe / SSH-user sets change.
      const names = Object.keys(result.matrices);
      const sshUsers = new Set(
        Object.keys(result.ssh ?? {}).map((k) => splitSSHKey(k)[0]),
      );
      const stillValid =
        names.includes(s.activeProbe) ||
        (s.activeProbe.startsWith("ssh:") && sshUsers.has(s.activeProbe.slice(4)));
      const activeProbe = stillValid ? s.activeProbe : (names[0] ?? "");
      return { result, evalMs, activeProbe, evaluating: false };
    }),
  setEngineStatus: (engineStatus) => set({ engineStatus }),
  setEvaluating: (evaluating) => set({ evaluating }),

  // The mobile selector tracks whichever axis the user last touched, so nothing
  // jumps when the viewport crosses the breakpoint.
  setView: (view) =>
    set((s) => ({
      view,
      mobilePane:
        view === "graph" ? "graph" : view === "matrix" ? "matrix" : s.mobilePane,
    })),
  setLeftTab: (leftTab) =>
    set({ leftTab, mobilePane: leftTab === "tailnet" ? "tailnet" : "learn" }),
  setAboutOpen: (aboutOpen) => set({ aboutOpen }),
  setTourNote: (tourNote) => set({ tourNote }),
  setMobilePane: (mobilePane) =>
    set(() => {
      switch (mobilePane) {
        case "tailnet":
          return { mobilePane, leftTab: "tailnet" };
        case "learn":
          return { mobilePane, leftTab: "learn" };
        case "graph":
          return { mobilePane, view: "graph" };
        case "matrix":
          return { mobilePane, view: "matrix" };
        default:
          return { mobilePane }; // "policy" has no desktop axis of its own
      }
    }),
  applyRoute: (v, lesson) =>
    set(() => {
      if (v === "tutorials")
        return { leftTab: "learn", activeLesson: lesson, mobilePane: "learn" };
      // About is a modal over whatever is showing (`#/about` deep link).
      if (v === "about") return { aboutOpen: true };
      // matrix / graph: the impact lens, and leave Learn if we're in it.
      return {
        view: impactView(v),
        leftTab: "tailnet",
        activeLesson: null,
        mobilePane: impactView(v),
      };
    }),
  setActiveProbe: (activeProbe) => set({ activeProbe }),
  setGrouping: (grouping) => set({ grouping }),
  selectPair: (selectedPair) => set({ selectedPair }),

  setGraphEmphasize: (graphEmphasize) => set({ graphEmphasize }),
  setGraphHideIsolated: (graphHideIsolated) => set({ graphHideIsolated }),
  setGraphAnyProbe: (graphAnyProbe) => set({ graphAnyProbe }),
  setGraphHighlightGroup: (graphHighlightGroup) => set({ graphHighlightGroup }),
  setFocusedActor: (focusedActor) => set({ focusedActor }),

  setActiveLesson: (activeLesson) => set({ activeLesson }),
  markLessonComplete: (slug) =>
    set((s) => {
      if (s.completedLessons.has(slug)) return s;
      const next = new Set(s.completedLessons);
      next.add(slug);
      return { completedLessons: next };
    }),

  hydrateFromShare: (sh) => {
    const v = sh.view ?? "matrix";
    const learn = v === "tutorials";
    set({
      policy: sh.policy,
      snapshot: sh.snapshot,
      probes: sh.probes?.length ? sh.probes : defaultProbes(),
      sshUsers: sh.sshUsers?.length ? sh.sshUsers : DEFAULT_SSH_USERS,
      view: impactView(v),
      leftTab: v === "tutorials" ? "learn" : "tailnet",
      mobilePane: learn ? "learn" : impactView(v),
      activeProbe: sh.activeProbe ?? "https",
      grouping: sh.grouping ?? "user",
      activeLesson: sh.activeLesson ?? null,
      selectedPair: null,
      focusedActor: null,
      loadedFromShare: true,
    });
  },
  dismissShareBanner: () => set({ loadedFromShare: false, shareFailed: false }),
  failShare: () => set({ shareFailed: true }),
  setPendingShare: (pendingShare) => set({ pendingShare }),
  startFresh: () =>
    set({
      policy: DEFAULT_TEMPLATE.policy,
      snapshot: DEFAULT_PRESET.snapshot,
      probes: defaultProbes(),
      sshUsers: DEFAULT_SSH_USERS,
      activeProbe: "https",
      grouping: "user",
      selectedPair: null,
      focusedActor: null,
      activeLesson: null,
      loadedFromShare: false,
      shareFailed: false,
      pendingShare: null,
      // The playground showing that scenario, not the Learn index.
      view: "matrix",
      leftTab: "tailnet",
      mobilePane: "matrix",
    }),

  setThemePref: (pref) => {
    persistThemePref(pref);
    set({ themePref: pref, themeResolved: resolveTheme(pref) });
  },
  syncResolvedTheme: () =>
    set((s) => ({ themeResolved: resolveTheme(s.themePref) })),

  setLayoutCols: (cols) =>
    set((s) => {
      if (cols.length !== 2) return s;
      const layout = { ...s.layout, cols: cols as [number, number] };
      persistLayout(layout);
      return { layout };
    }),
  setLayoutRows: (rows) =>
    set((s) => {
      if (rows.length !== 2) return s;
      const layout = { ...s.layout, rows: rows as [number, number] };
      persistLayout(layout);
      return { layout };
    }),
  setStripFraction: (frac) =>
    set((s) => {
      const layout = { ...s.layout, strip: clamp(frac, 0.15, 0.75) };
      persistLayout(layout);
      return { layout };
    }),
  toggleMaximized: (id) =>
    set((s) => {
      const layout = { ...s.layout, maximized: s.layout.maximized === id ? null : id };
      persistLayout(layout);
      return { layout };
    }),
  setPanelCollapsed: (id, collapsed) =>
    set((s) => {
      const layout = { ...s.layout, collapsed: { ...s.layout.collapsed, [id]: collapsed } };
      persistLayout(layout);
      return { layout };
    }),
}));
