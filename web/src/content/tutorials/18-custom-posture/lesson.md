Lesson 11 gated a rule on `node:os` and `node:tsVersion` — attributes Tailscale
provides. **`attrConfig`** lets you declare your *own*:

```json
"attrConfig": {
  "custom:env":       { "type": "string" },
  "custom:riskScore":  { "type": "number" }
}
```

Each device carries values for them in the snapshot — open a device's **posture**
popover and you'll see the custom fields. A posture then compares them exactly
like the built-in conditions: `==` / `!=`, numeric `<` `<=` `>` `>=`, `in [ … ]`,
and `&&` / `||`. The right-hand value is always quoted —
`custom:riskScore < '50'`, not `< 50`.

```json
"postures": {
  "posture:trusted": ["custom:env == 'prod'", "custom:riskScore < '50'"]
}
```

### Try it

No checklist — the `tests` block runs without posture, so this is one to watch
in the matrix and the **posture panel**.

1. Define `posture:trusted` in the `postures` block as above.
2. Add `"srcPosture": ["posture:trusted"]` to the grant.

Then:

- `alice-trusted` (env prod, score 10) — keeps access to `tag:secrets`
- `alice-risky` (env prod, score 90) — dropped, score too high
- `bob-dev` (env dev) — dropped, wrong environment

Edit a device's `custom:riskScore` in the popover and watch its row flip.

> [!note] Custom attributes are evaluated only from the values a device sets in
> the snapshot. Tailground does not pull them from Intune / Jamf / CrowdStrike —
> that live integration is out of scope (see the About panel).
