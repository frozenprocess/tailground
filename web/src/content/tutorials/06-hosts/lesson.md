`hosts` maps a name to **a single IP or a CIDR range**:

```json
"hosts": {
  "database":     "100.64.0.6/32",
  "office-subnet": "10.0.0.0/16"
}
```

Then `"database"` works in a rule's `src` or `dst`, exactly as if you'd written
the IP — it's purely cosmetic. It can't go inside a `groups` list, though;
those take user emails only.

### Your task

`db-1` is at `100.64.0.6`.

1. Add a host named `database` for `100.64.0.6/32`.
2. Let `group:eng` reach `database` on **5432**, and check that 22 stays
   closed.

> [!mistake] The value must be an IP or a CIDR — a DNS name (`"db.example.com"`)
> or an `ip:port` is rejected. A bare IP like `"100.64.0.6"` is fine; you don't
> need to write `/32`.

> [!note] Host names share a namespace with tags and groups, so don't call a
> host `web` if you also have `tag:web` floating around — pick distinct names.
