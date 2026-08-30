The policy now has `"grants": []` — an empty list — so **nothing** is reachable.
Every cell in the matrix is red. This is the safe place to start: you add back
only the paths you want.

### The shape of a rule

```json
{ "src": ["…"], "dst": ["…"], "ip": ["proto:port"] }
```

- `src` is a list of *who* may start the connection.
- `dst` is a list of *what* they may reach — a tag, a group, a host, an IP.
- `ip` is a list of *protocol and port*, written as `"tcp:443"`, `"udp:53"`,
  `"443"` (any protocol), a range like `"tcp:8000-8100"`, or `"*"`.

There is no explicit deny — you deny something by simply not writing a rule for
it. A `grant` only ever grants.

### Your task

Add one rule so that **alice and bob** can reach **`tag:web` on TCP 443** —
and check that the database and SSH stay closed.

> [!tip] `src` takes email addresses directly: `["alice@example.com",
> "bob@example.com"]`. The next lessons show tidier ways to say "the
> engineers".

> [!mistake] Putting the port on `dst` (`tag:web:443`). That's the older `acls`
> style — in a `grant` the port lives in `ip`. Lesson 7 covers the difference.
