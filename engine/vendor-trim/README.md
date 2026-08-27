# vendor-trim

`hscontrol/types/config.go` in the Headscale tree exists only to parse a
Headscale **server** config file (viper + go-oidc + x/oauth2 + fsnotify +
go-toml + mapstructure + afero + …). Tailground never runs a server and never
reads that config, but because `config.go` lives in the same Go package as
`types.Node` / `types.User` — which the policy engine *does* use — importing the
policy engine drags the whole config stack into the binary.

For the WASM build that stack is ~14 MB of dead weight:

| build | raw | gzip |
|---|---|---|
| untrimmed | ~25 MB | ~6 MB |
| trimmed (this stub) | ~11 MB | ~2.8 MB |

`config.go.stub` replaces the upstream file with a minimal `type Config` carrying
only the three fields `types.NodeView.TailNode` reads (`BaseDomain`,
`Taildrop.Enabled`, `AutoUpdate.Enabled`). `TailNode` builds a `tailcfg.Node` for
netmap generation and is **not** on Tailground's evaluation path, so the stub is
never exercised — it only needs to compile.

## Applying

`make vendor` runs `go mod vendor` then copies the stub over
`vendor/github.com/juanfont/headscale/hscontrol/types/config.go`. Run it after
any Headscale/Tailscale bump.

## When upstream changes

If a Headscale bump makes `TailNode` (or any other non-test code in
`hscontrol/types`) reference a new `Config` field, the vendored build fails with
`undefined: Config.<Field>`. Add the field to `config.go.stub` with a
zero-value-friendly type. If upstream ever splits policy-relevant types into
their own package, delete this directory and the `vendor` target's `cp` line.

## Fallback

If the stub becomes hard to maintain, drop it: the untrimmed build still works,
just larger. Update `make vendor` to skip the `cp`.

## Capability allowlist

Headscale rejects `tailscale.com/cap/*` grant capabilities that are not on its
allowlist, while the Tailscale control plane accepts more of them. This makes
Tailground report a parse error for valid policies. `capabilities.patch` adds
the missing names to the vendored allowlist; `make vendor` applies it after
`go mod vendor`. It currently adds `tailscale.com/cap/aperture` (Aperture
gateway grants). Drop a line when upstream Headscale allowlists that name; if
the patch no longer applies after a bump, re-create it against the new file.
