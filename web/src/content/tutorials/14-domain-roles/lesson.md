Two shortcuts let a rule name a whole class of people. Tailground expands both
into explicit emails **before** the policy compiles, so everything downstream —
the matrix, the checks — sees the real users.

### Domain wildcard

```json
"src": ["user:*@corp.example"]
```

Every tailnet user whose email ends in `@corp.example`. Contractors on other
domains are not included. Handy for "all employees" without a group you have to
keep in sync.

### Role autogroups

`autogroup:admin` is everyone whose **role** in the tailnet is owner or admin.
The full set Tailground resolves: `autogroup:owner`, `autogroup:admin`,
`autogroup:auditor`, `autogroup:it-admin`, `autogroup:network-admin`,
`autogroup:billing-admin`. A user's role is set in the snapshot's Users table.

They work in `src`, `dst`, `tagOwners`, and `autoApprovers`.

### Your task

- `alice@corp.example` has the **admin** role; `bob@corp.example` is a plain
  member; `carol@contractor.dev` is an outside contractor.
- Add a grant so **anyone at `corp.example`** reaches `tag:wiki` on 443.
- Add a grant so **`autogroup:admin`** reaches `tag:infra` on 22.

The checks confirm bob gets the wiki but not infra, carol gets neither, and
alice gets both.

> [!tip] Change bob's role to `admin` in the snapshot and watch his
> `→ infra-1` cell flip green — no policy edit.

> [!note] Headscale itself has no domain wildcard and no notion of roles;
> Tailground expands these tokens to the matching snapshot users before
> compiling. A token that matches nobody gets a note in the editor, because
> its rule then matches nothing.
