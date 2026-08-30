**Device posture** lets a rule require something about the *source device* —
its OS, its Tailscale version — not just who owns it.

```json
"postures": {
  "posture:corp": ["node:os == 'macos'", "node:tsVersion >= '1.80'"]
}
```

A posture is a list of conditions, **all** of which must hold. Attach it to a
rule with `srcPosture` (a list — the device must match **any** one):

```json
{ "src": ["group:admin"], "srcPosture": ["posture:corp"],
  "dst": ["tag:prod"], "ip": ["tcp:22"] }
```

Tailground models `node:os`, `node:tsVersion` (compared as a version), and
`node:tsReleaseTrack`, with `==` `!=` `>` `>=` `<` `<=`, `in [ … ]`, and
`&&` / `||`.

A top-level `"defaultSrcPosture": ["posture:corp"]` sets a baseline for **every**
acls/grants/ssh rule that has no `srcPosture` of its own. A rule's own
`srcPosture` *replaces* the default, it doesn't add to it.

> [!why] "Only from a managed, up-to-date device" is a real security control.
> Posture is how a policy file expresses it without a separate MDM integration.

### Try it

Right now all three admin devices reach `tag:prod` on 22. Define `posture:corp`
and add `srcPosture` to the rule. Then watch the **matrix** and the **posture
panel**:

- `alice-macbook` (macOS, 1.84) — keeps SSH
- `alice-old-mac` (macOS, 1.70) — dropped, version too low
- `bob-linux` — dropped, wrong OS

Set a device's `node:tsVersion` in the snapshot's posture popover and see it
flip live.

> [!note] Posture is evaluated by **Tailground's own layer** — its reading of
> the syntax, not Tailscale's code. The `tests` block runs *without* posture, so
> this lesson has no checklist.
