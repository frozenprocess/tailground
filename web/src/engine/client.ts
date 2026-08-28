// Typed wrapper over the WASM engine worker. One worker per client; requests are
// posted immediately; only the latest request's promise resolves. Worker errors
// update status without settling pending evaluations. The app owns the debounce.

import type { EngineRequest, EngineResult } from "./types";

type WorkerMsg =
  | { type: "ready" }
  | { type: "error"; message: string }
  | { type: "result"; id: number; result: EngineResult; ms: number };

export interface EvalOutcome {
  result: EngineResult;
  ms: number;
}

export type EngineStatus = "loading" | "ready" | "error";

class EngineClient {
  private worker: Worker;
  private nextId = 1;
  private pending = new Map<number, (o: EvalOutcome) => void>();
  private status: EngineStatus = "loading";
  private readyWaiters: Array<(s: EngineStatus) => void> = [];
  private lastRequestedId = 0;

  onStatusChange?: (s: EngineStatus) => void;

  constructor() {
    // worker.js, wasm_exec.js and engine.wasm are served from the app base as
    // static assets (built by `make wasm`). Classic worker: it uses importScripts.
    // `?v=` is the build's engine hash: a new engine gets new URLs, so no cache
    // (browser or CDN) can pair a fresh page with a stale engine. The worker
    // passes it on to the files it loads.
    const v = import.meta.env.VITE_ENGINE_VERSION as string;
    const url = `${import.meta.env.BASE_URL}worker.js?v=${v}`;
    this.worker = new Worker(url);
    this.worker.onmessage = (e: MessageEvent<WorkerMsg>) => this.handle(e.data);
    this.worker.onerror = () => this.setStatus("error");
  }

  private setStatus(s: EngineStatus) {
    this.status = s;
    this.onStatusChange?.(s);
    if (s !== "loading") {
      this.readyWaiters.forEach((w) => w(s));
      this.readyWaiters = [];
    }
  }

  private handle(msg: WorkerMsg) {
    switch (msg.type) {
      case "ready":
        this.setStatus("ready");
        break;
      case "error":
        this.setStatus("error");
        break;
      case "result": {
        const resolve = this.pending.get(msg.id);
        if (resolve) {
          this.pending.delete(msg.id);
          resolve({ result: msg.result, ms: msg.ms });
        }
        break;
      }
    }
  }

  getStatus(): EngineStatus {
    return this.status;
  }

  whenSettled(): Promise<EngineStatus> {
    if (this.status !== "loading") return Promise.resolve(this.status);
    return new Promise((res) => this.readyWaiters.push(res));
  }

  /**
   * Evaluate a request. If a newer evaluate() is called before this resolves,
   * the older promise never resolves (the caller should always await the latest).
   */
  evaluate(request: EngineRequest): Promise<EvalOutcome> {
    const id = this.nextId++;
    this.lastRequestedId = id;
    return new Promise((resolve) => {
      this.pending.set(id, (o) => {
        if (id === this.lastRequestedId) resolve(o);
      });
      this.worker.postMessage({ id, request });
    });
  }

  dispose() {
    this.worker.terminate();
    this.pending.clear();
  }
}

// One engine worker for the lifetime of the page. Created on first use so tests
// and SSR don't spawn a worker at import time.
let singleton: EngineClient | null = null;

export function getEngineClient(): EngineClient {
  if (!singleton) singleton = new EngineClient();
  return singleton;
}
