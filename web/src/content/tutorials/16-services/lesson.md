A **Tailscale Service** is one virtual IP that fronts several backend devices —
a stable address for `payments`, whichever machines actually run it today.

In the policy you write `svc:payments` as a destination:

```json
{ "src": ["tag:client"], "dst": ["svc:payments"], "ip": ["tcp:443"] }
```

The service — its VIP and backend devices — lives in the snapshot (the
**Tailscale Services** table). Tailground rewrites `svc:payments` to the VIP
before compiling, then checks each source against every backend's own filter and
shows the result in the **Tailscale Services** panel.

### Your task

`svc:payments` is defined: VIP `100.96.0.10`, backends `api-1` and `api-2`.

Add a grant: **`tag:client` reaches `svc:payments` on 443**.

The checks target the VIP directly — reachable on 443, closed on 22.

> [!why] Clients target the service, not the machines. Add or drain a backend
> and the policy doesn't change — only the snapshot does.

> [!note] `svc:` in a grant `dst` takes no port suffix — the protocol/port goes
> in `ip`, same as any other grant.
