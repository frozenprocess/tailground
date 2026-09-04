Another tailnet can **share** some of its nodes with yours, declared right in
the policy file. Sharing takes two policies, one per tailnet, and each side
controls only its own half.

```json
"externalTailnets": {
  "partner": {
    "externalID": "acme-42",
    "allowIncomingConnections": true,
    "allowExternalReferencesTo": ["tag:client"]
  }
}
```

`partner` is a **local alias** you choose; `externalID` is the other tailnet's
ID. The shared nodes live in the snapshot (the **External tailnets** section),
each with the tags and groups its *home* tailnet gave it. You name them with:

- `group://partner/eng` — the partner's `eng` group
- `tag://partner/relay` — the partner's nodes tagged `relay`

The two flags are the two halves:

- **`allowIncomingConnections: true`** — you're the **sharing** side: you accept
  connections from the partner. Your grants use `group://partner/…` /
  `tag://partner/…` as a **`src`**, with one of *your* resources as the `dst`.
- **`allowExternalReferencesTo`** — you're the **receiving** side: the local
  tags/groups the *partner's* policy may name. Whether those devices actually
  reach the partner is up to the partner's grants, not yours.

### Try it

No checklist — open the **Declarative node sharing** panel below the matrix and
switch the probe between **https** and **ssh**.

- `eng-laptop … reaches srv-1 on https` and `relay … reaches srv-1 on ssh` —
  your two grants. `sales-laptop` reaches nothing: no grant names `sales`.
- `its policy may grant access to client-1` — from
  `"allowExternalReferencesTo": ["tag:client"]`. Tailground can't see the
  partner's policy, so this is a list of what it *may* name, not a verdict.
  Swap in `"tag:srv"` and the list changes to `srv-1`.
- Flip `allowIncomingConnections` to `false`. Tailscale now rejects the whole
  policy — `external tailnet not allowed in local policy grants: "partner"` —
  and Tailground shows the same error on both grants. You don't accept
  connections from a tailnet until you opt in, and a grant that tries is an
  error, not a no-op. Set it back to `true`.
- Now try the other direction from your side — add
  `{ "src": ["tag:client"], "dst": ["tag://partner/relay"], "ip": ["tcp:22"] }`.
  Rejected again: `dst "tag://partner/relay": unsupported target`. An external
  reference is only valid as a `src`. To let `client-1` reach `relay`, the
  partner writes that grant in *its* policy (`src: tag://<you>/client`), and
  your `allowExternalReferencesTo` lets it.

> [!mistake] External references only work in `grants`. The same
> `tag://partner/relay` in an `acls` rule — or in `ssh`, a group, an ipset, a
> grant's `via` — gets `src "tag://partner/relay": unsupported target`, even
> with `allowIncomingConnections: true`.

> [!mistake] `unsupported target` on a reference that *looks* right usually
> means a character that only looks right — a `∕` (division slash) or `／`
> (full-width slash) pasted from chat or a doc instead of `/`. Retype the
> reference by hand; Tailground names the odd character in its error.

> [!note] Declarative node sharing is in alpha (waitlist) and may change. The
> syntax follows Tailscale's
> [declarative node sharing docs](https://tailscale.com/docs/features/declarative-node-sharing);
> only the sharing tailnet needs to be enrolled.
