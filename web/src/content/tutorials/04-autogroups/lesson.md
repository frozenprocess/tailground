**Autogroups** are sets the system maintains for you. The ones you'll reach for
most:

| Autogroup | Means |
|---|---|
| `autogroup:member` | every device owned by a user in your tailnet |
| `autogroup:tagged` | every tagged device |
| `autogroup:internet` | the public internet (via an exit node) |
| `autogroup:self` | *the same user's* other devices (lesson 10) |
| `autogroup:nonroot` | in an `ssh` rule's `users`: any login name except root |
| `autogroup:danger-all` | literally everything, including untagged/unknown |

### Your task

Write one rule: **`autogroup:member` → `tag:web` on 443**. Now *anyone* in the
tailnet can reach the web tier, and you never have to touch this rule as people
join or leave.

Check that tagged servers still can't reach a laptop on 22 — you didn't grant
that, so it stays denied.

> [!why] `autogroup:member` is the honest way to write "all our people". Listing
> users by hand goes stale the moment someone joins.

> [!note] The **role** autogroups — `autogroup:admin`, `autogroup:owner`,
> `autogroup:auditor`, `autogroup:it-admin`, `autogroup:network-admin`,
> `autogroup:billing-admin` — and `autogroup:shared` are also resolved, against
> each user's role in the snapshot panel. Set a user's role there and reference
> `autogroup:admin` in a rule to see it work.
