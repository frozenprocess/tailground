import { createHash } from "node:crypto";
import { existsSync, readFileSync } from "node:fs";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The engine files (worker.js, wasm_exec.js, engine.wasm) live in public/ with
// stable names, so browsers and Cloudflare would keep serving a stale engine
// to a fresh page. Their URLs carry this content hash instead (`?v=…`), which
// changes whenever any of them does — the same cache-busting the hashed
// /assets/ bundles get.
function engineVersion(): string {
  const h = createHash("sha256");
  for (const f of ["public/worker.js", "public/wasm_exec.js", "public/engine.wasm"]) {
    if (existsSync(f)) h.update(readFileSync(f));
  }
  return h.digest("hex").slice(0, 12);
}

// Base path is overridable for project-pages hosting. Default "/" works for
// local dev and any root-domain deploy; the Pages workflow sets TAILGROUND_BASE.
export default defineConfig({
  base: process.env.TAILGROUND_BASE ?? "/",
  define: {
    "import.meta.env.VITE_BUILD_DATE": JSON.stringify(
      new Date().toISOString().slice(0, 10),
    ),
    "import.meta.env.VITE_ENGINE_VERSION": JSON.stringify(engineVersion()),
  },
  plugins: [react()],
  // Short share links: the tailground-api sidecar (server/). nginx does this
  // in production; `go run ./server` serves it for dev and preview.
  server: { proxy: { "/api": "http://127.0.0.1:8081" } },
  preview: { proxy: { "/api": "http://127.0.0.1:8081" } },
  build: {
    target: "es2022",
    // engine.wasm is large; don't inline or warn about it.
    assetsInlineLimit: 0,
    // The graph chunk (React Flow + elkjs) is lazy-loaded and inherently ~1.6 MB;
    // the entry chunk stays ~680 KB.
    chunkSizeWarningLimit: 1800,
  },
  worker: {
    format: "es",
  },
});
