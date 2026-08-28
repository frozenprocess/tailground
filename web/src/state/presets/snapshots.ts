import type { Snapshot } from "../../engine/types";

export interface SnapshotPreset {
  id: string;
  label: string;
  blurb: string;
  snapshot: Snapshot;
}

export const SNAPSHOT_PRESETS: SnapshotPreset[] = [
  {
    id: "small-startup",
    label: "Small startup",
    blurb: "Two people, a couple of laptops, one app + one db server.",
    snapshot: {
      users: [
        { id: 1, email: "alice@example.com" },
        { id: 2, email: "bob@example.com" },
      ],
      groups: { "group:eng": ["alice@example.com", "bob@example.com"] },
      devices: [
        { id: 1, name: "alice-laptop", user: "alice@example.com", ipv4: "100.64.0.1", ipv6: "fd7a:115c:a1e0::1", os: "macos" },
        { id: 2, name: "bob-laptop", user: "bob@example.com", ipv4: "100.64.0.2", ipv6: "fd7a:115c:a1e0::2", os: "linux" },
        { id: 3, name: "app-1", tags: ["tag:app"], ipv4: "100.64.0.3", ipv6: "fd7a:115c:a1e0::3", os: "linux" },
        { id: 4, name: "db-1", tags: ["tag:db"], ipv4: "100.64.0.4", ipv6: "fd7a:115c:a1e0::4", os: "linux" },
      ],
    },
  },
  {
    id: "tagged-infra",
    label: "Tagged infra tiers",
    blurb: "web / app / db tiers plus an admin laptop.",
    snapshot: {
      users: [{ id: 1, email: "alice@example.com" }],
      devices: [
        { id: 1, name: "admin-laptop", user: "alice@example.com", ipv4: "100.64.0.1", os: "macos" },
        { id: 2, name: "web-1", tags: ["tag:web"], ipv4: "100.64.0.2", os: "linux" },
        { id: 3, name: "web-2", tags: ["tag:web"], ipv4: "100.64.0.3", os: "linux" },
        { id: 4, name: "app-1", tags: ["tag:app"], ipv4: "100.64.0.4", os: "linux" },
        { id: 5, name: "db-1", tags: ["tag:db"], ipv4: "100.64.0.5", os: "linux" },
      ],
    },
  },
  {
    id: "multi-team",
    label: "Multi-team",
    blurb: "Two teams with their own tagged services.",
    snapshot: {
      users: [
        { id: 1, email: "alice@example.com" },
        { id: 2, email: "bob@example.com" },
        { id: 3, email: "carol@example.com" },
        { id: 4, email: "dave@example.com" },
      ],
      groups: {
        "group:eng": ["alice@example.com", "bob@example.com"],
        "group:ops": ["carol@example.com", "dave@example.com"],
      },
      devices: [
        { id: 1, name: "alice-laptop", user: "alice@example.com", ipv4: "100.64.0.1", os: "macos" },
        { id: 2, name: "bob-laptop", user: "bob@example.com", ipv4: "100.64.0.2", os: "windows" },
        { id: 3, name: "carol-laptop", user: "carol@example.com", ipv4: "100.64.0.3", os: "linux" },
        { id: 4, name: "dave-laptop", user: "dave@example.com", ipv4: "100.64.0.4", os: "macos" },
        { id: 5, name: "eng-ci", tags: ["tag:eng-ci"], ipv4: "100.64.0.5", os: "linux" },
        { id: 6, name: "ops-bastion", tags: ["tag:ops-bastion"], ipv4: "100.64.0.6", os: "linux" },
        { id: 7, name: "prod-db", tags: ["tag:prod"], ipv4: "100.64.0.7", os: "linux" },
      ],
    },
  },
  {
    id: "posture-mix",
    label: "Posture mix",
    blurb: "Admin laptops with varied OS / Tailscale version, plus a prod box.",
    snapshot: {
      users: [
        { id: 1, email: "alice@example.com" },
        { id: 2, email: "bob@example.com" },
      ],
      groups: { "group:admin": ["alice@example.com", "bob@example.com"] },
      devices: [
        { id: 1, name: "alice-macbook", user: "alice@example.com", ipv4: "100.64.0.1", os: "macos", posture: { tsVersion: "1.82.0", tsReleaseTrack: "stable" } },
        { id: 2, name: "alice-old-mac", user: "alice@example.com", ipv4: "100.64.0.2", os: "macos", posture: { tsVersion: "1.70.0" } },
        { id: 3, name: "bob-linux", user: "bob@example.com", ipv4: "100.64.0.3", os: "linux", posture: { tsVersion: "1.84.0" } },
        { id: 4, name: "prod-1", tags: ["tag:prod"], ipv4: "100.64.0.4", os: "linux" },
      ],
    },
  },
  {
    id: "large",
    label: "Large (30 devices)",
    blurb: "A default-open tailnet big enough to stress the graph declutter levers.",
    snapshot: largeTailnet(),
  },
];

function largeTailnet(): Snapshot {
  const people = ["alice", "bob", "carol", "dave", "erin", "frank"];
  const users = people.map((p, i) => ({ id: i + 1, email: `${p}@example.com` }));
  const devices: Snapshot["devices"] = [];
  let id = 1;
  const ip = () => `100.64.${Math.floor(id / 250)}.${id % 250}`;

  // Two laptops each.
  for (const p of people) {
    for (const kind of ["laptop", "phone"]) {
      devices.push({ id, name: `${p}-${kind}`, user: `${p}@example.com`, ipv4: ip(), os: kind === "phone" ? "ios" : "macos" });
      id++;
    }
  }
  // Tagged infra.
  const infra: [string, number][] = [
    ["tag:web", 4],
    ["tag:app", 4],
    ["tag:db", 3],
    ["tag:ci", 2],
    ["tag:monitoring", 1],
  ];
  for (const [tag, n] of infra) {
    for (let i = 1; i <= n; i++) {
      devices.push({ id, name: `${tag.slice(4)}-${i}`, tags: [tag], ipv4: ip(), os: "linux" });
      id++;
    }
  }
  return { users, devices };
}
