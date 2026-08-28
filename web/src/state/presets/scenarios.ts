import type { Snapshot } from "../../engine/types";

// A scenario is a matched policy + snapshot that shows one engine capability
// working end to end — the policy templates and snapshot presets above are
// authored independently and don't demonstrate features that need both sides.
export interface Scenario {
  id: string;
  label: string;
  blurb: string;
  policy: string;
  snapshot: Snapshot;
}

export const SCENARIO_PRESETS: Scenario[] = [
  {
    id: "admin-autogroups",
    label: "Admin-role autogroups",
    blurb:
      "autogroup:admin resolves against each user's tailnet role (owner + admin); everyone else lands in autogroup:member.",
    policy: `{
  // autogroup:admin / owner / auditor / … are resolved by Tailground against
  // each snapshot user's role before compiling — the Headscale engine has no
  // notion of roles. Change a user's role in the snapshot and watch the matrix.
  "tagOwners": {
    "tag:infra": ["autogroup:admin"],
  },
  "grants": [
    { "src": ["autogroup:admin"], "dst": ["tag:infra"], "ip": ["tcp:22"] },
    { "src": ["autogroup:member"], "dst": ["tag:infra"], "ip": ["tcp:443"] },
  ],
}
`,
    snapshot: {
      users: [
        { id: 1, email: "owner@example.com", role: "owner" },
        { id: 2, email: "admin@example.com", role: "admin" },
        { id: 3, email: "auditor@example.com", role: "auditor" },
        { id: 4, email: "dev@example.com" },
      ],
      devices: [
        { id: 1, name: "owner-laptop", user: "owner@example.com", ipv4: "100.64.0.1", os: "macos" },
        { id: 2, name: "admin-laptop", user: "admin@example.com", ipv4: "100.64.0.2", os: "macos" },
        { id: 3, name: "auditor-laptop", user: "auditor@example.com", ipv4: "100.64.0.3", os: "linux" },
        { id: 4, name: "dev-laptop", user: "dev@example.com", ipv4: "100.64.0.4", os: "linux" },
        { id: 5, name: "infra-1", tags: ["tag:infra"], ipv4: "100.64.0.5", os: "linux" },
      ],
    },
  },
  {
    id: "subnet-exit",
    label: "Subnet router & exit node",
    blurb:
      "A device advertising an approved route becomes a subnet router; 0.0.0.0/0 makes an exit node. Reachability shows in Routes & exit nodes.",
    policy: `{
  "tagOwners": {
    "tag:client": ["alice@example.com"],
    "tag:router": ["alice@example.com"],
    "tag:exit": ["alice@example.com"],
  },
  "grants": [
    // Reach a private subnet the office router carries…
    { "src": ["tag:client"], "dst": ["10.20.0.0/24"], "ip": ["*"] },
    // …and the public internet through the exit node.
    { "src": ["tag:client"], "dst": ["autogroup:internet"], "ip": ["*"] },
  ],
}
`,
    snapshot: {
      users: [{ id: 1, email: "alice@example.com" }],
      devices: [
        { id: 1, name: "client-1", tags: ["tag:client"], ipv4: "100.64.0.1", os: "linux" },
        {
          id: 2,
          name: "office-router",
          tags: ["tag:router"],
          ipv4: "100.64.0.2",
          os: "linux",
          routes: ["10.20.0.0/24"],
          routesApproved: ["10.20.0.0/24"],
        },
        {
          id: 3,
          name: "exit-node",
          tags: ["tag:exit"],
          ipv4: "100.64.0.3",
          os: "linux",
          routes: ["0.0.0.0/0", "::/0"],
          routesApproved: ["0.0.0.0/0", "::/0"],
        },
      ],
    },
  },
  {
    id: "services",
    label: "Tailscale Services",
    blurb:
      "svc:<name> in a dst resolves to the service VIP; reachability is probed against each backend. Shown in Tailscale Services.",
    policy: `{
  // svc:payments is defined in the snapshot (VIP + backend devices). Tailground
  // rewrites the dst to the VIP before compiling — the Headscale engine has no
  // Tailscale Services.
  "tagOwners": {
    "tag:client": ["alice@example.com"],
    "tag:api": ["alice@example.com"],
    "tag:other": ["alice@example.com"],
  },
  "grants": [
    { "src": ["tag:client"], "dst": ["svc:payments"], "ip": ["tcp:443"] },
  ],
}
`,
    snapshot: {
      users: [{ id: 1, email: "alice@example.com" }],
      devices: [
        { id: 1, name: "client-1", tags: ["tag:client"], ipv4: "100.64.0.1", os: "linux" },
        { id: 2, name: "api-1", tags: ["tag:api"], ipv4: "100.64.0.2", os: "linux" },
        { id: 3, name: "api-2", tags: ["tag:api"], ipv4: "100.64.0.3", os: "linux" },
        { id: 4, name: "outsider", tags: ["tag:other"], ipv4: "100.64.0.4", os: "linux" },
      ],
      services: [{ name: "svc:payments", vip: "100.96.0.10", backends: [2, 3] }],
    },
  },
  {
    id: "custom-posture",
    label: "Custom device posture",
    blurb:
      "attrConfig declares custom:env / custom:riskScore; a srcPosture gate compares them (string / numeric) against per-device values.",
    policy: `{
  "attrConfig": {
    "custom:env": { "type": "string" },
    "custom:riskScore": { "type": "number" },
  },
  "postures": {
    "posture:trusted": ["custom:env == 'prod'", "custom:riskScore < '50'"],
  },
  "groups": {
    "group:eng": ["alice@example.com", "bob@example.com"],
  },
  "tagOwners": {
    "tag:secrets": ["group:eng"],
  },
  "grants": [
    {
      "src": ["group:eng"],
      "srcPosture": ["posture:trusted"],
      "dst": ["tag:secrets"],
      "ip": ["tcp:443"],
    },
  ],
}
`,
    snapshot: {
      users: [
        { id: 1, email: "alice@example.com" },
        { id: 2, email: "bob@example.com" },
      ],
      groups: { "group:eng": ["alice@example.com", "bob@example.com"] },
      devices: [
        {
          id: 1,
          name: "alice-trusted",
          user: "alice@example.com",
          ipv4: "100.64.0.1",
          os: "macos",
          posture: { "custom:env": "prod", "custom:riskScore": "10" },
        },
        {
          id: 2,
          name: "alice-risky",
          user: "alice@example.com",
          ipv4: "100.64.0.2",
          os: "macos",
          posture: { "custom:env": "prod", "custom:riskScore": "90" },
        },
        {
          id: 3,
          name: "bob-dev",
          user: "bob@example.com",
          ipv4: "100.64.0.3",
          os: "linux",
          posture: { "custom:env": "dev", "custom:riskScore": "5" },
        },
        { id: 4, name: "secrets-1", tags: ["tag:secrets"], ipv4: "100.64.0.4", os: "linux" },
      ],
    },
  },
  {
    id: "declarative-sharing",
    label: "Declarative node sharing",
    blurb:
      "externalTailnets + group:// / tag:// references resolve against shared nodes from another tailnet. Shown in Declarative node sharing.",
    policy: `{
  // The shared nodes come from the snapshot's externalTailnets. A group:// /
  // tag:// reference is only valid in a grant's src, and its tailnet must set
  // allowIncomingConnections — anywhere else, Tailscale rejects the policy.
  // For your devices to reach the partner, the partner writes the grant in its
  // policy (src: tag://<you>/srv); allowExternalReferencesTo lists what it may name.
  "tagOwners": {
    "tag:srv": ["alice@example.com"],
    "tag:client": ["alice@example.com"],
  },
  "externalTailnets": {
    "partner": {
      "externalID": "acme-42",
      "allowIncomingConnections": true,
      "allowExternalReferencesTo": ["tag:srv"],
    },
  },
  "grants": [
    { "src": ["group://partner/eng"], "dst": ["tag:srv"], "ip": ["tcp:443"] },
    { "src": ["tag://partner/relay"], "dst": ["tag:srv"], "ip": ["tcp:22"] },
  ],
}
`,
    snapshot: {
      users: [{ id: 1, email: "alice@example.com" }],
      devices: [
        { id: 1, name: "srv-1", tags: ["tag:srv"], ipv4: "100.64.0.1", os: "linux" },
        { id: 2, name: "client-1", tags: ["tag:client"], ipv4: "100.64.0.2", os: "linux" },
      ],
      externalTailnets: [
        {
          alias: "partner",
          nodes: [
            { name: "eng-laptop", groups: ["eng"] },
            { name: "relay", tags: ["relay"] },
            { name: "sales-laptop", groups: ["sales"] },
          ],
        },
      ],
    },
  },
];
