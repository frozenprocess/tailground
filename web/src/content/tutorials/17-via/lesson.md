A grant's **`via`** list forces traffic to pass through a named gateway node on
its way to the destination:

```json
{ "src": ["tag:app"], "dst": ["10.99.0.0/24"], "ip": ["tcp:80"], "via": ["tag:relay"] }
```

"`tag:app` may reach `10.99.0.0/24` on port 80, but only through a `tag:relay`
node." The relay has to advertise — and have approved — a route covering that
prefix (lesson 12). Reachability shows in the **Routes & exit nodes** panel
below the matrix, one line per advertised prefix.

### Try it

There's no checklist. The grant is already in the policy. `relay` carries an
approved route for `10.99.0.0/24`; `relay-unapproved` advertises the same prefix
but nobody approved it.

Open the **Routes & exit nodes** panel and switch the probe between **http**
and **https**:

- on **http (80)**: `10.99.0.0/24 via relay — reachable on http from app-1`
- on **https (443)**: `via relay — no source reaches it on https` — the grant
  only opened 80
- `relay-unapproved` stays `advertised, not approved` — it never carries traffic

Change `"ip"` to `["tcp:443"]` and the `relay` line reads *no source reaches it*
on http too. Remove `via` and `tag:app` still reaches the prefix through the
approved route, but no specific gateway is required.

> [!why] `via` is how you route through an inspection point — a firewall, a
> logging proxy — instead of letting clients reach the subnet directly.
