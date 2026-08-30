A device that advertises a **subnet route** (`--advertise-routes=10.1.0.0/16`)
or offers itself as an **exit node** (`0.0.0.0/0` + `::/0`) can carry traffic
for addresses that aren't tailnet devices — a private LAN behind the router, or
the whole public internet.

Two things have to be true for that traffic to flow:

**1. The route is approved.** An admin approves it in the console, or
`autoApprovers` approves it automatically:

```json
"autoApprovers": {
  "routes":   { "10.1.0.0/16": ["tag:router"] },
  "exitNode": ["tag:router"]
}
```

`routes` maps a CIDR to the users / groups / tags whose advertisement of *that
exact route* is auto-approved. `exitNode` lists who may become an exit node
without review.

**2. A rule opens the destination.** Approval only means "this router is trusted
to carry the prefix" — it grants nobody access. You still need a `grant` whose
`dst` is the CIDR, or `autogroup:internet` for the exit node.

### Try it

`office-router` advertises `10.1.0.0/16`, but it's **pending** — see the
**Routes & exit nodes** panel below the matrix. `home-gateway` is already an
approved exit node. Both grants are in the policy.

Approve the subnet route: open `office-router`'s **routes** cell in the snapshot
panel and tick the box next to `10.1.0.0/16`. In the **Routes & exit nodes**
panel the prefix flips from *advertised, not approved* to *reachable on https
from `alice-laptop`*. Untick it and it goes back to pending.

> [!note] Tailground models route **reachability** from the snapshot's advertised
> and approved routes. It does **not** decide whether `autoApprovers` *would*
> have approved a given advertisement — that one check is listed under "not
> modeled" in the About panel. Approve routes yourself in the snapshot to
> explore. There's no checklist for this lesson.

> [!why] Keeping "approved" separate from "granted" is deliberate: approval is
> about trusting the router with a prefix; the grant is about who may use what
> it carries.
