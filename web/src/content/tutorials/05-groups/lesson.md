A **group** is a named list of users:

```json
"groups": {
  "group:eng":   ["alice@example.com", "bob@example.com"],
  "group:sales": ["carol@example.com"]
}
```

Anywhere a rule accepts a user email — `src`, `tagOwners`, an `ssh` rule — it
also accepts a `group:` name. Define the team once; the rules read like
sentences.

### Your task

`group:eng` is already declared. Carol belongs on the sales team, which isn't.

1. Add `group:sales` with `carol@example.com`.
2. Write a rule so that **only sales** reaches `tag:web` on 443. Engineers
   should *not* — the check confirms alice is denied.

> [!mistake] Groups are **not recursive** — you can't put `group:eng` inside
> `group:all`. List the members directly in each group.

> [!tip] The snapshot panel has its own groups box, but the **policy's** groups
> block is what actually counts. Keep them in sync, or just let the policy be
> the source of truth.
