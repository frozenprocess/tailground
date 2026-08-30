`ssh` rules are separate from `grants`. A grant still has to open **port 22**
for the TCP connection; the `ssh` rule then decides *who may log in as which
local user*, and how hard.

```json
{
  "action": "accept",
  "src":    ["group:eng"],
  "dst":    ["tag:web"],
  "users":  ["autogroup:nonroot"]
}
```

- **`action: "accept"`** — connect straight through.
- **`action: "check"`** — allowed, but the user must re-authenticate in a
  browser every `checkPeriod` (e.g. `"12h"`, or `"always"`).
- **`users`** — the local login names. `["ubuntu", "root"]` for specific
  accounts, `["autogroup:nonroot"]` for "any name except root". There's no
  wildcard: `autogroup:nonroot` never covers `root`, so name `root` explicitly
  when you want it.

### Your task

Port 22 is already open (by the grant). Add two `ssh` rules:

1. `group:eng` gets **accept** SSH to `tag:web` as `autogroup:nonroot`.
2. `group:sales` gets **check** SSH to `tag:web` as `autogroup:nonroot`.

The checks: alice reaches it as `ubuntu` but **not** as `root`; carol reaches it
only **via check**.

> [!mistake] Forgetting the grant for port 22. No open port, no SSH, no matter
> what the `ssh` block says.
