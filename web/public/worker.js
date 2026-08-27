// Engine worker: load the Go WASM policy engine once, then service evaluate
// requests off the main thread. Classic worker so importScripts() can pull in
// the Go runtime shim exactly as the pinned toolchain ships it.
//
// Protocol:
//   worker -> page  { type: "ready" }
//   worker -> page  { type: "error", message }            (fatal load failure)
//   page   -> worker { id, request }                       (eval.Request)
//   worker -> page  { type: "result", id, result, ms }    (eval.Result)

// The page loads this as worker.js?v=<engine hash>; carry the same version on
// to the files below so a new build can never run against a cached old engine.
const version = new URL(self.location.href).search;

importScripts("wasm_exec.js" + version);

const go = new Go();
let engineReady = false;

async function loadEngine() {
  const resp = await fetch("engine.wasm" + version);
  let instance;
  try {
    ({ instance } = await WebAssembly.instantiateStreaming(resp.clone(), go.importObject));
  } catch {
    // Fallback: some static servers send the wrong MIME for .wasm.
    const bytes = await resp.arrayBuffer();
    ({ instance } = await WebAssembly.instantiate(bytes, go.importObject));
  }
  go.run(instance); // resolves only when the Go program exits; it never does
  engineReady = true;
  postMessage({ type: "ready" });
}

loadEngine().catch((err) => {
  postMessage({ type: "error", message: "wasm load failed: " + err });
});

onmessage = (e) => {
  const { id, request } = e.data;
  if (!engineReady || !globalThis.tailground) {
    postMessage({
      type: "result",
      id,
      ms: 0,
      result: {
        matrices: {},
        actors: [],
        engine: { headscaleCommit: "", tailscaleVersion: "" },
        diagnostics: [{ severity: "error", code: "internal", message: "engine not ready" }],
      },
    });
    return;
  }
  const t0 = performance.now();
  let result;
  try {
    result = JSON.parse(globalThis.tailground.evaluate(JSON.stringify(request)));
  } catch (err) {
    result = {
      matrices: {},
      actors: [],
      engine: { headscaleCommit: "", tailscaleVersion: "" },
      diagnostics: [{ severity: "error", code: "internal", message: "engine call failed: " + err }],
    };
  }
  postMessage({ type: "result", id, result, ms: performance.now() - t0 });
};
