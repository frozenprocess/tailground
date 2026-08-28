import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { EngineRequest, EngineResult } from "./types";

class WorkerFake {
  static current: WorkerFake;
  onmessage?: (e: { data: unknown }) => void;
  onerror?: () => void;
  postMessage = vi.fn();
  terminate = vi.fn();
  constructor(readonly url: string) { WorkerFake.current = this; }
  emit(data: unknown) { this.onmessage?.({ data }); }
}
const request: EngineRequest = { policy: "{}", snapshot: { users: [], devices: [] }, probes: [] };
const result: EngineResult = { matrices: {}, actors: [], engine: { headscaleCommit: "test", tailscaleVersion: "test" } };
beforeEach(() => { vi.resetModules(); vi.stubGlobal("Worker", WorkerFake); });
afterEach(() => vi.unstubAllGlobals());

it("creates one versioned worker and waits for readiness", async () => {
  const { getEngineClient } = await import("./client");
  const client = getEngineClient();
  expect(getEngineClient()).toBe(client);
  expect(WorkerFake.current.url).toMatch(/worker\.js\?v=/);
  expect(client.getStatus()).toBe("loading");
  const settled = vi.fn();
  void client.whenSettled().then(settled);
  await Promise.resolve();
  expect(settled).not.toHaveBeenCalled();
  WorkerFake.current.emit({ type: "ready" });
  await Promise.resolve();
  expect(settled).toHaveBeenCalledWith("ready");
  expect(await client.whenSettled()).toBe("ready");
});
it("posts every request but only resolves the latest, regardless of response order", async () => {
  const { getEngineClient } = await import("./client");
  const client = getEngineClient();
  const older = vi.fn(), latest = vi.fn();
  void client.evaluate(request).then(older);
  void client.evaluate({ ...request, policy: "new" }).then(latest);
  expect(WorkerFake.current.postMessage.mock.calls).toEqual([[{ id: 1, request }], [{ id: 2, request: { ...request, policy: "new" } }]]);
  WorkerFake.current.emit({ type: "result", id: 2, result, ms: 4 });
  WorkerFake.current.emit({ type: "result", id: 1, result, ms: 2 });
  await Promise.resolve();
  expect(older).not.toHaveBeenCalled();
  expect(latest).toHaveBeenCalledExactlyOnceWith({ result, ms: 4 });
});
it.each(["message", "event"])("reports a worker %s error without settling evaluations", async (kind) => {
  const { getEngineClient } = await import("./client");
  const client = getEngineClient();
  const pending = vi.fn();
  void client.evaluate(request).then(pending);
  const settled = client.whenSettled();
  if (kind === "message") WorkerFake.current.emit({ type: "error", message: "failed" });
  else WorkerFake.current.onerror?.();
  expect(await settled).toBe("error");
  expect(client.getStatus()).toBe("error");
  expect(pending).not.toHaveBeenCalled();
  client.dispose();
  expect(WorkerFake.current.terminate).toHaveBeenCalledOnce();
});
