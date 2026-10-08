// @vitest-environment jsdom
import { act } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { render } from "../test/render";
import { useStore } from "../state/store";
import { useShareHydration, loadPendingShare } from "./useShareHydration";
import { useHashRoute } from "./useHashRoute";
import fixtures from "./testdata/shares.json";

let unmount: (() => Promise<void>) | undefined;
function Routes() { useShareHydration(); useHashRoute(); return null; }
beforeEach(() => {
  useStore.setState(useStore.getInitialState(), true);
  localStorage.clear();
  vi.stubGlobal("fetch", vi.fn());
});
afterEach(async () => { await unmount?.(); unmount = undefined; vi.unstubAllGlobals(); });

it("holds foreign short links for consent and strips before routing, even in StrictMode", async () => {
  window.history.replaceState(null, "", "/play/#/s/Abc12345");
  unmount = await render(<Routes />);
  expect(fetch).not.toHaveBeenCalled();
  expect(location.hash).toBe("#/");
  expect(useStore.getState()).toMatchObject({ pendingShare: { kind: "id", id: "Abc12345" }, loadedFromShare: false, leftTab: "tailnet", view: "matrix" });
  await unmount(); unmount = undefined;
  unmount = await render(<Routes />);
  expect(fetch).not.toHaveBeenCalled();
  expect(useStore.getState().loadedFromShare).toBe(false);
});
it("does not decode foreign tokens before consent; attached route wins when accepted", async () => {
  window.history.replaceState(null, "", `/play/#/graph/#s=${fixtures[1].token}`);
  unmount = await render(<Routes />);
  expect(useStore.getState().policy).not.toBe(fixtures[1].decoded.policy);
  expect(useStore.getState().pendingShare).toEqual({ kind: "token", token: fixtures[1].token, route: "#/graph" });
  expect(location.hash).toBe("#/graph");
  const pending = useStore.getState().pendingShare!;
  await act(async () => loadPendingShare(pending));
  expect(useStore.getState()).toMatchObject({ policy: fixtures[1].decoded.policy, view: "graph", leftTab: "tailnet", loadedFromShare: true });
  expect(location.hash).toBe("#/graph");
  expect(fetch).not.toHaveBeenCalled();
});
it("automatically loads this browser's own short link once", async () => {
  localStorage.setItem("tailground.ownShares", JSON.stringify(["Abc12345"]));
  vi.mocked(fetch).mockResolvedValue({ ok: true, text: async () => fixtures[1].token } as Response);
  window.history.replaceState(null, "", "/play/#/s/Abc12345");
  unmount = await render(<Routes />);
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(useStore.getState()).toMatchObject({ policy: fixtures[1].decoded.policy, loadedFromShare: true, pendingShare: null, leftTab: "learn", activeLesson: "intro" });
  expect(location.hash).toBe("#/tutorials/intro");
});
it("handles a later hashchange without loading a foreign link", async () => {
  window.history.replaceState(null, "", "/play/#/graph");
  unmount = await render(<Routes />);
  await act(async () => {
    window.history.replaceState(null, "", "/play/#/s/Other123");
    window.dispatchEvent(new HashChangeEvent("hashchange"));
  });
  expect(useStore.getState()).toMatchObject({ pendingShare: { kind: "id", id: "Other123" }, view: "matrix", loadedFromShare: false });
  expect(fetch).not.toHaveBeenCalled();
});
it("marks invalid accepted links as failed without replacing the working policy", async () => {
  const policy = useStore.getState().policy;
  await loadPendingShare({ kind: "token", token: "invalid", route: null });
  expect(useStore.getState()).toMatchObject({ policy, shareFailed: true, loadedFromShare: false });
});
