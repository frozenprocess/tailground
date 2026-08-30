A **tailnet** is your private network of devices. A **policy file** (this
HuJSON document) decides which device may open a connection to which other
device, on which ports.

The panel to the right is a **mock tailnet**: three people with laptops, plus a
`tag:web` server and a `tag:db` server. The matrix on the far right shows, for
every ordered pair of devices, whether a connection is allowed.

### The default is "allow everything"

A brand-new tailnet's policy is an explicit "allow everything" rule (plus a
default Tailscale SSH rule) — not literally empty, but the effect is the same:
*every device can reach every other device*. The editor here holds a trimmed
version — just `tagOwners` so the tagged servers are named, and **no `acls` or
`grants`**, which on its own already means allow-all. The matrix is all green.

> [!why] Tailscale ships open so a new tailnet "just works", then you tighten it.
> A policy with no `acls` and no `grants` key compiles to allow-all; add either
> one — even as an empty list — and the tailnet flips to deny-by-default.
> Every lesson after this one is about writing rules that take access away.

### Try it

The checks below already pass, because everything is reachable. Add a
`"grants": []` line — an **empty list** rather than no list — and watch the
matrix go all-red and the checks fail: an empty `grants` list means "deny by
default", and you then add back exactly the paths you want.

Delete the `grants` line again to restore allow-all (or just move to the next
lesson, which starts from default-deny).
