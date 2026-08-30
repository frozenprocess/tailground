No hints this time — build the policy from the checks.

You have `group:eng`, `tag:web` (two servers), and `tag:db` (one). The
requirements:

1. **Anyone** in the tailnet can reach `tag:web` on **443**.
2. `tag:web` can reach `tag:db` on **5432** — nothing else can.
3. Engineers get **check**-SSH into `tag:web` as a non-root user.
4. Everything else stays closed: no SSH to the database, no web on 22.

You'll need `grants`, an `ssh` rule, and port 22 open on `tag:web` for the SSH
to have a connection.

> [!tip] Work one check at a time. Green the reachability rows first, then add
> the `ssh` block — remember it needs its own `grants` entry for port 22.

When every box is ticked you've written a policy that would hold up in a real
tailnet. That's the whole track.
