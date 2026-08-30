A grant's `app` field carries **application capabilities** — named blobs of
JSON that Tailscale hands to a service so it can make its *own* authorization
decisions, beyond "can this packet through":

```json
{
  "src": ["group:eng"],
  "dst": ["tag:web"],
  "ip":  ["tcp:443"],
  "app": {
    "example.com/cap/deploy": [{ "env": "staging" }]
  }
}
```

The capability name is a URL-ish string you choose. The value is an array of
JSON objects — your service reads them via the Tailscale API / `WhoIs` and
decides what "deploy in staging" means.

### Try it

This lesson has no automated checks — capabilities don't change reachability, so
`tests` can't see them.

Add the `app` block above to the grant, then **click the `alice-laptop →
web-1` cell** in the matrix. The pair detail lists **App capabilities granted**.

> [!why] Ports are a blunt instrument. App capabilities let one Tailscale grant
> express "these people, this role, these environments" and keep the logic in
> your app where it belongs.

> [!note] App connectors (`nodeAttrs[].app`) are a different feature — Tailground
> surfaces them on a device's capabilities but does **not** simulate the
> connector routing.
