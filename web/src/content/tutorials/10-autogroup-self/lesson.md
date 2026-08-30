`autogroup:self` as a **destination** means "devices owned by the *same user* as
the source". It's how you let someone reach their laptop from their phone
without opening everyone's devices to everyone.

```json
{ "src": ["autogroup:member"], "dst": ["autogroup:self"], "ip": ["*"] }
```

Read it as: any member → their own devices, any port.

This scenario has Alice and Bob, each with a laptop and a phone.

### Try it

There's no checklist — a policy `tests` block resolves against the *global*
filter, and `autogroup:self` only has meaning per-device, so it can't be graded
that way. Watch the **matrix** instead.

Add the rule above. You should see:

- `alice-laptop ↔ alice-phone` — green
- `bob-laptop ↔ bob-phone` — green
- `alice-* → bob-*` and `bob-* → alice-*` — still red

> [!why] `autogroup:self` needs per-node compilation: each device gets a filter
> that only mentions its owner's other devices. Tailground's matrix does this;
> that's why the graph and matrix show it correctly even though `tests` can't.
