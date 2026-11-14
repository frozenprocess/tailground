# Tailground

A browser-based playground for learning, writing, and visualizing **Tailscale
policy files** — write a policy, drop in a mock tailnet of devices and users, and
see exactly which connections it allows or denies.

The policy is evaluated by a real engine: [Headscale]'s open-source
reimplementation of Tailscale's ACL compiler, plus Tailscale's own packet-filter
matcher, compiled to WebAssembly and run entirely in your browser. Evaluation
needs no backend or account. Sharing prefers an optional SQLite-backed short-link
API; if unavailable, the full scenario is encoded in the URL fragment.

## Develop

Requires Go 1.27+.

```sh
make vendor       # once, and after any dependency bump
make test         # root module: go test ./...
(cd server && go test ./...)  # optional short-link API, separate module
make e2e-offline  # PR-safe offline differential-harness checks
go run ./engine/cli -demo | jq .   # evaluate the built-in scenario

make web-install  # once — npm install for the SPA
make web-dev      # build the engine + run the Vite dev server
make web-build    # build the engine + the static site into web/dist

make spike        # WASM engine + the no-build smoke page at :8080/spike.html
make size         # report the WASM bundle size
```

## Deploy

```sh
make k8s-deploy TAILGROUND_HOST=t.example.com IMAGE_TAG=20260101-abc123 STORAGE_SIZE=5Gi
```

## License

BSD-3-Clause (matches upstream Tailscale and Headscale) — see [LICENSE](./LICENSE).

[Headscale]: https://github.com/juanfont/headscale
[policy-file syntax reference]: https://tailscale.com/docs/reference/syntax/policy-file
