Every rule so far has been a `grant`. Before grants, Tailscale policies used
`acls`, and you'll still see them everywhere — existing tailnets, older docs,
Stack Overflow answers. They're not deprecated; grants are just where new
features land, so it's worth being able to read and convert one.

A grant:

```json
{ "src": ["group:eng"], "dst": ["tag:web"], "ip": ["tcp:443"] }
```

is the same as this acl:

```json
{ "action": "accept", "src": ["group:eng"], "dst": ["tag:web:443"] }
```

Differences:

- An acl has `"action": "accept"` — that's the only action; there's no deny.
- The **port/protocol is glued onto `dst`** as `tag:web:443`, not split into
  its own `ip` field.
- An acl can't carry `app` (lesson 8) or `via` (route through a gateway) — that
  needs a grant.

`acls` and `grants` are additive and evaluated together — a connection is
allowed if *either* permits it.

### Your task

The policy below has inherited an **acl** that opens port **80** by mistake.
Replace it with a **grant** that opens **443** (`"ip": ["tcp:443"]`), and delete
the acl. The checks want 443 reachable and 80 closed.

> [!tip] New policies use `grants` for everything. Convert acls as you touch
> them rather than leaving the policy in two dialects.
