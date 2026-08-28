import { beforeEach, expect, it } from "vitest";
import { canonicalView, useStore, type MainView, type MobilePane } from "./store";
import { DEFAULT_PRESET, DEFAULT_TEMPLATE } from "./presets";
import { defaultProbes, DEFAULT_SSH_USERS, type EngineResult } from "../engine/types";

beforeEach(() => useStore.setState(useStore.getInitialState(), true));
const result: EngineResult = { matrices: { https: {} }, actors: [], engine: { headscaleCommit: "test", tailscaleVersion: "test" } };
it.each([['learn', 'graph', 'tutorials'], ['learn', 'matrix', 'tutorials'], ['tailnet', 'graph', 'graph'], ['tailnet', 'matrix', 'matrix']] as const)("canonical view %s + %s = %s", (leftTab, view, expected) => {
  expect(canonicalView({ leftTab, view })).toBe(expected);
});
it.each<[MainView, object]>([
  ["graph", { view: "graph", leftTab: "tailnet", mobilePane: "graph", activeLesson: null }],
  ["matrix", { view: "matrix", leftTab: "tailnet", mobilePane: "matrix", activeLesson: null }],
  ["tutorials", { view: "graph", leftTab: "learn", mobilePane: "learn", activeLesson: "intro" }],
  ["about", { view: "graph", leftTab: "learn", mobilePane: "learn", aboutOpen: true }],
])("applies route %s without losing the independent pane", (view, expected) => {
  useStore.getState().applyRoute(view, "intro");
  expect(useStore.getState()).toMatchObject(expected);
});
it.each<[MobilePane, object]>([
  ["tailnet", { leftTab: "tailnet", view: "graph" }], ["learn", { leftTab: "learn", view: "graph" }],
  ["graph", { leftTab: "learn", view: "graph" }], ["matrix", { leftTab: "learn", view: "matrix" }],
  ["policy", { leftTab: "learn", view: "graph" }],
])("mirrors mobile pane %s", (pane, expected) => {
  useStore.getState().setMobilePane(pane);
  expect(useStore.getState()).toMatchObject({ ...expected, mobilePane: pane });
});
it("hydrates missing optional fields and resets selections", () => {
  useStore.setState({ selectedPair: { src: 1, dst: 2 }, focusedActor: 1 });
  useStore.getState().hydrateFromShare({ policy: "{}", snapshot: { users: [], devices: [] }, probes: [] });
  expect(useStore.getState()).toMatchObject({ policy: "{}", probes: defaultProbes(), sshUsers: DEFAULT_SSH_USERS, view: "matrix", leftTab: "tailnet", mobilePane: "matrix", activeProbe: "https", grouping: "user", activeLesson: null, selectedPair: null, focusedActor: null, loadedFromShare: true });
});
it.each(["matrix", "graph", "tutorials", "about"] as const)("hydrates the existing %s wire view", (view) => {
  useStore.getState().hydrateFromShare({ policy: "{}", snapshot: { users: [], devices: [] }, probes: [], view });
  expect(useStore.getState()).toMatchObject({ view: view === "graph" ? "graph" : "matrix", leftTab: view === "tutorials" ? "learn" : "tailnet", mobilePane: view === "tutorials" ? "learn" : view === "graph" ? "graph" : "matrix" });
});
it("starts fresh without resetting unrelated preferences or lesson progress", () => {
  useStore.setState({ shareFailed: true, loadedFromShare: true, pendingShare: { kind: "id", id: "Abc12345" }, activeLesson: "intro", focusedActor: 1, selectedPair: { src: 1, dst: 2 }, graphAnyProbe: false, completedLessons: new Set(["intro"]) });
  useStore.getState().startFresh();
  expect(useStore.getState()).toMatchObject({ policy: DEFAULT_TEMPLATE.policy, snapshot: DEFAULT_PRESET.snapshot, probes: defaultProbes(), sshUsers: DEFAULT_SSH_USERS, shareFailed: false, loadedFromShare: false, pendingShare: null, activeLesson: null, focusedActor: null, selectedPair: null, view: "matrix", leftTab: "tailnet", mobilePane: "matrix", graphAnyProbe: false });
  expect(useStore.getState().completedLessons).toEqual(new Set(["intro"]));
});
it("keeps a valid SSH probe, falls back after removal and handles empty results", () => {
  useStore.setState({ activeProbe: "ssh:root", evaluating: true });
  useStore.getState().setResult({ ...result, ssh: { "root@1->2": "accept" } }, 12);
  expect(useStore.getState()).toMatchObject({ activeProbe: "ssh:root", evalMs: 12, evaluating: false });
  useStore.getState().setResult(result, 10);
  expect(useStore.getState().activeProbe).toBe("https");
  useStore.getState().setResult({ ...result, matrices: {} }, 5);
  expect(useStore.getState().activeProbe).toBe("");
});
it("preserves distinct template, preset and scenario reset semantics", () => {
  for (const action of ["template", "preset", "scenario"]) {
    useStore.setState({ selectedPair: { src: 1, dst: 2 }, focusedActor: 1 });
    const s = useStore.getState();
    if (action === "template") s.loadTemplate("{}");
    else if (action === "preset") s.loadPreset({ users: [], devices: [] });
    else s.loadScenario("{}", { users: [], devices: [] });
    expect(useStore.getState().selectedPair).toBeNull();
    expect(useStore.getState().focusedActor).toBe(action === "scenario" ? null : 1);
  }
});
