An **ipset** is a named list of addresses:

```json
"ipsets": {
  "ipset:build-deps": ["100.64.0.20/32", "100.64.0.21/32"]
}
```

Members can be CIDRs, single IPs, `a-b` ranges, `host:` references, or nested
`ipset:` names. Anywhere a rule takes a target, `ipset:build-deps` stands in for
all of them — Tailground substitutes the members before compiling.

It's the tidy way to write "our block of office subnets" or "the third-party
API endpoints" once and reuse it.

### Your task

The CI runners (`tag:ci`) build software that pulls from two internal services
which aren't tailnet devices — a registry at `100.64.0.20` and a cache at
`100.64.0.21`. They're already grouped as `ipset:build-deps`.

Add a grant: **`tag:ci` reaches `ipset:build-deps` on 443**.

The checks confirm both members are reachable on 443, but not on 22, and an
address outside the set (`100.64.0.99`) stays closed.

> [!mistake] `ipset:` names must start with the `ipset:` prefix, both in the
> `ipsets` block and where you reference them.

> [!note] The `tests` block never expands the `ipset:` token itself — on either
> side. `"src": "ipset:build-deps"` and `"accept": ["ipset:build-deps:443"]`
> both fail to resolve. Only `acls` / `grants` / `ssh` rules get ipset
> expansion. In a check, write the member IPs as literals (`"tag:ci"` →
> `"100.64.0.20:443"`), as this lesson does — that works for a set on either
> the source or the destination side.
