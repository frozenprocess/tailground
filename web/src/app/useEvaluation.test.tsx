// @vitest-environment jsdom
import { act } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { render } from "../test/render";
import { useStore } from "../state/store";
import type { EngineResult } from "../engine/types";
import type { EngineStatus, EvalOutcome } from "../engine/client";
import { useEvaluation } from "./useEvaluation";

const client = vi.hoisted(() => ({ getStatus: vi.fn(), whenSettled: vi.fn(), evaluate: vi.fn(), onStatusChange: undefined as ((s: EngineStatus) => void) | undefined }));
vi.mock("../engine/client", () => ({ getEngineClient: () => client }));
const result: EngineResult = { matrices: { https: {} }, actors: [], engine: { headscaleCommit: "test", tailscaleVersion: "test" } };
let unmount: (() => Promise<void>) | undefined;
function Evaluation() { useEvaluation(); return null; }
beforeEach(() => {
  vi.useFakeTimers(); vi.clearAllMocks();
  useStore.setState(useStore.getInitialState(), true);
  client.getStatus.mockReturnValue("ready");
  client.whenSettled.mockResolvedValue("ready");
  client.evaluate.mockResolvedValue({ result, ms: 7 });
});
afterEach(async () => { await unmount?.(); unmount = undefined; vi.useRealTimers(); });
async function tick(ms: number) { await act(async () => { await vi.advanceTimersByTimeAsync(ms); }); }
it("debounces rapid edits and publishes only the final request under StrictMode", async () => {
  unmount = await render(<Evaluation />);
  await tick(199); expect(client.evaluate).not.toHaveBeenCalled();
  await act(async () => useStore.getState().setPolicy("first"));
  await tick(100);
  await act(async () => useStore.getState().setPolicy("latest"));
  await tick(199); expect(client.evaluate).not.toHaveBeenCalled();
  await tick(1);
  expect(client.evaluate).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ policy: "latest", snapshot: useStore.getState().snapshot, probes: useStore.getState().probes, sshUsers: useStore.getState().sshUsers }));
  expect(useStore.getState()).toMatchObject({ result, evalMs: 7, evaluating: false, engineStatus: "ready" });
});
it("waits for worker readiness and drops an obsolete readiness waiter", async () => {
  const waiters: Array<(s: EngineStatus) => void> = [];
  client.getStatus.mockReturnValue("loading");
  client.whenSettled.mockImplementation(() => new Promise<EngineStatus>(resolve => waiters.push(resolve)));
  unmount = await render(<Evaluation />); await tick(200);
  await act(async () => useStore.getState().setPolicy("latest")); await tick(200);
  expect(client.evaluate).not.toHaveBeenCalled();
  await act(async () => { client.onStatusChange?.("ready"); for (const resolve of waiters) resolve("ready"); });
  expect(client.evaluate).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ policy: "latest" }));
  expect(useStore.getState().engineStatus).toBe("ready");
});
it("does not publish an old result after inputs change", async () => {
  const outcomes: Array<(o: EvalOutcome) => void> = [];
  client.evaluate.mockImplementation(() => new Promise<EvalOutcome>(resolve => outcomes.push(resolve)));
  unmount = await render(<Evaluation />); await tick(200);
  await act(async () => useStore.getState().setPolicy("latest")); await tick(200);
  await act(async () => outcomes[0]({ result, ms: 1 }));
  expect(useStore.getState().result).toBeNull();
  await act(async () => outcomes[1]({ result, ms: 2 }));
  expect(useStore.getState()).toMatchObject({ result, evalMs: 2, evaluating: false });
});
it("cancels the debounce on unmount", async () => {
  unmount = await render(<Evaluation />); await tick(100); await unmount(); unmount = undefined; await tick(200);
  expect(client.evaluate).not.toHaveBeenCalled();
});
it("does not evaluate if worker initialization fails", async () => {
  client.whenSettled.mockResolvedValue("error");
  unmount = await render(<Evaluation />); await tick(200);
  expect(client.evaluate).not.toHaveBeenCalled();
});
