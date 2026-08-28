export interface PolicyTemplate {
  id: string;
  label: string;
  blurb: string;
  policy: string;
}

export const POLICY_TEMPLATES: PolicyTemplate[] = [
  {
    id: "allow-all",
    label: "Tailscale default (grants)",
    blurb:
      "What every new tailnet is seeded with — an allow-all grant plus SSH check to your own devices.",
    policy: `{
  // The policy Tailscale adds to every new tailnet. "grants" is the current
  // model; the older "acls" syntax still works but grants is what's recommended.
  "grants": [
    // Allow all connections. Comment this out to define specific restrictions.
    { "src": ["*"], "dst": ["*"], "ip": ["*"] },
  ],
  "ssh": [
    // Allow all users to SSH into their own devices in check mode.
    {
      "action": "check",
      "src": ["autogroup:member"],
      "dst": ["autogroup:self"],
      "users": ["autogroup:nonroot", "root"],
    },
  ],
}
`,
  },
  {
    id: "default-deny",
    label: "Default-deny + explicit grants",
    blurb: "Start closed, then open specific paths.",
    policy: `{
  "groups": {
    "group:eng": ["alice@example.com", "bob@example.com"],
  },
  "grants": [
    // Engineers can reach the app servers on HTTPS.
    { "src": ["group:eng"], "dst": ["tag:app"], "ip": ["tcp:443"] },
    // App servers can reach the database.
    { "src": ["tag:app"], "dst": ["tag:db"], "ip": ["tcp:5432"] },
  ],
  "tagOwners": {
    "tag:app": ["group:eng"],
    "tag:db": ["group:eng"],
  },
}
`,
  },
  {
    id: "tag-tiers",
    label: "Tag-based tiers",
    blurb: "web → app → db, each tier reaching only the next.",
    policy: `{
  "tagOwners": {
    "tag:web": ["alice@example.com"],
    "tag:app": ["alice@example.com"],
    "tag:db": ["alice@example.com"],
  },
  "grants": [
    { "src": ["autogroup:member"], "dst": ["tag:web"], "ip": ["tcp:443"] },
    { "src": ["tag:web"], "dst": ["tag:app"], "ip": ["tcp:8080"] },
    { "src": ["tag:app"], "dst": ["tag:db"], "ip": ["tcp:5432"] },
  ],
}
`,
  },
  {
    id: "ssh-check",
    label: "SSH: accept vs check",
    blurb: "Admins connect directly; everyone else re-authenticates.",
    policy: `{
  "groups": {
    "group:admin": ["alice@example.com"],
    "group:oncall": ["bob@example.com"],
  },
  "tagOwners": {
    "tag:prod": ["group:admin"],
  },
  "grants": [
    { "src": ["group:admin", "group:oncall"], "dst": ["tag:prod"], "ip": ["tcp:22"] },
  ],
  "ssh": [
    { "action": "accept", "src": ["group:admin"], "dst": ["tag:prod"], "users": ["root", "ubuntu"] },
    { "action": "check", "src": ["group:oncall"], "dst": ["tag:prod"], "users": ["autogroup:nonroot"], "checkPeriod": "12h" },
  ],
}
`,
  },
  {
    id: "grants-app",
    label: "Application capabilities",
    blurb: "A grant that carries an app capability payload, not just ports.",
    policy: `{
  "groups": {
    "group:eng": ["alice@example.com"],
  },
  "tagOwners": {
    "tag:app": ["group:eng"],
  },
  "grants": [
    {
      "src": ["group:eng"],
      "dst": ["tag:app"],
      "ip": ["tcp:443"],
      "app": {
        "example.com/cap/admin": [{ "role": "admin", "env": ["staging", "prod"] }],
      },
    },
  ],
}
`,
  },
  {
    id: "posture-admin",
    label: "Posture-gated admin",
    blurb: "SSH to prod only from an up-to-date, managed macOS device.",
    policy: `{
  // Tailground evaluates 'postures' itself — this is its reading of the syntax,
  // not Tailscale's code. Try lowering a device's node:tsVersion in the snapshot.
  "postures": {
    "posture:corp": ["node:os == 'macos'", "node:tsVersion >= '1.80'"],
  },
  "groups": {
    "group:admin": ["alice@example.com", "bob@example.com"],
  },
  "tagOwners": {
    "tag:prod": ["group:admin"],
  },
  "grants": [
    {
      "src": ["group:admin"],
      "srcPosture": ["posture:corp"],
      "dst": ["tag:prod"],
      "ip": ["tcp:22"],
    },
  ],
}
`,
  },
];
