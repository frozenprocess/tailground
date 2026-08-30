A **tag** labels a device by what it *is* — `tag:web`, `tag:db`, `tag:ci` —
instead of who owns it. Servers are tagged; a tagged device has no user, so
rules about people never touch it by accident.

### tagOwners

Every tag must be listed in `tagOwners`, which says **who is allowed to assign
it**:

```json
"tagOwners": {
  "tag:web": ["alice@example.com"],
  "tag:db":  ["alice@example.com"]
}
```

The database server in this scenario carries `tag:db`, but the policy only
defines `tag:web`. Until you add `tag:db` to `tagOwners`, no rule can name it —
you'll see a warning under the editor.

### Your task

1. Add `tag:db` to `tagOwners`.
2. Add a rule so `tag:web` can reach `tag:db` on **5432** (the app tier talks to
   the database), but not on 22.

> [!why] Once the tiers are wired by tag, adding a third web server is just a
> matter of tagging it — no policy edit. That's the whole point of tags.

> [!mistake] A tag in `src` or `dst` that isn't in `tagOwners` makes the whole
> policy invalid. Define the tag first.
