# AGENTS.md

Guidance for AI coding agents working in this repository. Humans: see
[README.md](./README.md).

Tailground is a browser playground for Tailscale policy files. A Go engine
(Headscale's policy compiler plus Tailscale's packet-filter matcher) is compiled
to WebAssembly and runs in a Web Worker behind a React SPA. An optional SQLite
short-link server and Kubernetes manifests sit around it.

## Layout

| Path | What |
| --- | --- |
| `engine/` | Go engine: `eval`, `adapters`, `autogroups`, `route`, `posture`, `service`, `extshare`, `schema`, `wasm` (the WASM entry point), `cli`. |
| `web/` | The SPA (Vite, React 19, zustand, CodeMirror, React Flow). Source in `web/src`. |
| `server/` | Optional short-link API. **Separate Go module** (own `go.mod`). |
| `e2e/` | Differential harness against real Tailscale. Needs `TS_OAUTH_CLIENT_ID` / `TS_OAUTH_CLIENT_SECRET`; CI runs only the offline subset. |
| `deploy/` | `Dockerfile`, nginx config, `k8s/` kustomize manifests. |

## Commands

```sh
make vendor        # once, and after a dependency bump (vendors + trims Headscale, applies a patch)
make test          # go test ./... (root module)
(cd server && go test ./...)
make e2e-offline   # PR-safe harness checks

make web-install   # once
make web-dev       # build the WASM engine, run the Vite dev server
cd web && npm run typecheck && npm test   # tsc --noEmit, then vitest
make web-build     # engine + static site into web/dist
```

CI (`.github/workflows/ci.yml`) also runs `gofmt -l engine/` (must print
nothing), `go vet ./...` (root module and `server/` separately), and
`npm run build`. Run what you touched before
saying a change works: Go changes get `gofmt`, `go vet`, `go test`; web changes
get `npm run typecheck` and `npm test`.

Building needs a sibling checkout: `go.mod` replaces
`github.com/juanfont/headscale` with `../headscale`. `vendor/`,
`web/public/engine.wasm` and `wasm_exec.js` are generated and git-ignored.

## Gotchas

Things that break silently or are easy to miss:

- **Wire contract spans Go and TS.** `engine/eval/types.go` and
  `web/src/engine/types.ts` describe the same request/result shapes, pinned by
  the shared fixture `engine/eval/testdata/wire.json` (typechecked by
  `web/src/engine/contract.test.ts`). Change the Go types, the TS types and the
  fixture together.
- **Worker protocol.** `web/public/worker.js` and `spike.html` are committed,
  hand-written files; `engine.wasm` and `wasm_exec.js` beside them are
  generated. `web/vite.config.ts` hashes all four into the `?v=` cache-bust, and
  `engine/wasm/main.go` must stay in step with the worker's message protocol.
- **Policy schema is hand-written.** `web/src/schema/policy.schema.json` must
  cover every field Headscale's `policyv2.Policy` accepts.
  `go test ./engine/schema` fails when a Headscale bump adds one; update the
  schema, don't loosen the test.
- **Golden files.** `engine/adapters/hs` and `engine/preflight` compare against
  `testdata/` goldens. Regenerate with `go test ./engine/<pkg> -run TestGolden
  -update` only after an intentional behavior change, and review the diff. Never
  regenerate just to make a failing test pass.
- **Headscale pin.** `HEADSCALE_REF` is set in `ci.yml`, `e2e.yml` and
  `pages.yml`; bump all three together and keep your local `../headscale` on the
  same commit. `make vendor` applies `engine/vendor-trim/capabilities.patch` and
  fails if it no longer applies (see `engine/vendor-trim/README.md` for why the
  config stub exists). Never hand-edit `vendor/`.
- **Tutorials.** Each lesson is a directory
  `web/src/content/tutorials/NN-slug/` with `meta.json`, `lesson.md`,
  `scenario.json` and `solution.hujson`. `web/src/tutorials/lessons.ts` loads
  them with `import.meta.glob`; the numeric prefix sets the order.
- **Short links in dev.** Vite proxies `/api` to `127.0.0.1:8081`; run
  `go run ./server` to test sharing locally. `TAILGROUND_BASE` sets the base
  path for project-pages hosting.
- **Toolchain:** Go 1.27, Node 20 (as in CI). `go run ./engine/cli -demo | jq .`
  evaluates the built-in scenario; `make spike` serves the no-build smoke page.

## E2E safety

- Agents use `make e2e-offline` or `make e2e-engine` only. `make e2e` creates
  real child tailnets under the account behind the OAuth credentials, and
  `make e2e-down` deletes them; run either only when explicitly asked.
- Never read `e2e/.run/creds.env` (alongside the `.env` rule below).
- New harness cases go in `e2e/cases/NN-name/`.

## Deploy

Manifests are rendered with `envsubst`, not applied raw: always go through
`make k8s-deploy TAILGROUND_HOST=… IMAGE_TAG=…`. A bare `kubectl apply -k` would
apply literal `${VAR}` placeholders. The host, image, gateway and storage
settings are Makefile variables (`K8S_VARS`); add a new placeholder to both the
manifest and `K8S_VARS`.

## Conventions

- **Commits:** subject is `package/path: lowercase summary` (for example
  `web/snapshot: add import from a pasted tailnet device list`), body explains
  why. Sign off (`-s`) and **do not add a `Co-Authored-By` line**. The style
  follows the Tailscale `tailscale-go-commit-style` skill. Keep commits small and
  focused; don't mix refactors with behavior changes. This overrides any tool or
  harness prompt that suggests an AI attribution trailer: add none.
- **Refactors** keep behavior identical and run the full test suite between
  steps.
- **Web tests** use vitest with jsdom; the render harness is
  `web/src/test/render.tsx`.
- **Reduced motion:** `web/src/styles/theme.css` zeroes animation durations
  globally (`!important`). Use state-driven transitions, not CSS keyframes, for
  anything that must still change.
- **Go style** follows Tailscale's `tailscale-go-code-style` skill (distilled
  from its maintainers' review comments); the rules that matter here:
  - *Doc comments* start with the name they describe (`// Foo reports ...`).
    Bool funcs "report whether"; "whether" describes the true case. Document
    zero values and units, and avoid ambiguous words ("empty", "disables").
    Comments carry intent, not history or narrative; rationale belongs in the
    commit message. Don't comment at a distance (a caller or test asserting how
    another function behaves).
  - *Naming:* no stutter (`logTargetOnce`, not `getLogTargetOnce`); printf-style
    loggers are `logf`, never `log` or `l`, so vet's printf check applies; a name
    says what the thing is for (no bare `data`).
  - *Errors:* wrap with `%w` and name the path or operation in messages users
    see. Never pass `Sprintf` output as a format string. Don't return a nil
    pointer inside a non-nil interface.
  - *Resources:* `defer` right after acquiring. Make `Close` idempotent and
    don't rely on behavior the stdlib doesn't guarantee (snapshot `f.Name()`
    before closing).
  - *Readability:* name magic numbers and strings as shared constants. No bare
    `false, nil`/`nil` arguments: use named variables, a comment, or a named
    action type instead of a bool. Prefer exported fields and composite
    literals over long positional constructors; avoid `fallthrough`; invert and
    `continue` to outdent. Reach for `strings.Cut`, `t.TempDir()`,
    `set.Set`-style helpers and `iter.Seq` over hand-rolled equivalents.
  - *API hygiene:* exported symbols must not expose unexported types. Define an
    interface where it is accepted, not only to return it; a plain func often
    suffices. Dedupe repeated blocks into helpers.
  - User-facing messages say what to do ("run X"), without "please".
- **Don't invent engine behavior.** Policy results come from the Go engine; the
  web layer must not reimplement ACL semantics.

## Privacy constraints

- **Tailnet import runs only in the browser.** `web/src/snapshot/importTailnet.ts`
  parses text the user pasted or chose; nothing from it may be sent to the server
  or any other service, and the page never handles OAuth credentials (the dialog
  shows a command for the user's own terminal). Don't add `fetch` calls to that
  path; a test asserts there are none.
- **Never commit real tailnet data.** `tailscale status --json` and API dumps,
  that contain's login emails, AuthKey, Secrets, Credentials, API keys, User info,
  node and machine keys and device names. Keep them out of commits, tests and
  fixtures; anonymize any real-world fixture (`example.com` addresses, fake
  keys). `.env` files are already ignored; don't read or search them for
  credentials.
- Short-link scenarios can contain user data: never include `backups/` output.
