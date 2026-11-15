import { describe, expect, it } from "vitest";
import { applyImport, ImportError, mergeServices, parseTailnetImport } from "./importTailnet";
import type { Snapshot } from "../engine/types";

const api = {
  devices: [
    {
      id: "123",
      hostname: "alice-laptop",
      name: "alice-laptop.tail1234.ts.net",
      user: "alice@example.com",
      addresses: ["100.64.0.1", "fd7a:115c:a1e0::1"],
      os: "macOS",
      clientVersion: "1.76.1-t5d3a2a8e8-g1234",
      tags: [],
    },
    {
      hostname: "router",
      user: "alice@example.com",
      tags: ["tag:router"],
      addresses: ["100.64.0.2"],
      os: "linux",
      advertisedRoutes: ["10.0.0.0/24", "0.0.0.0/0"],
      enabledRoutes: ["10.0.0.0/24"],
    },
    { name: "router.tail1234.ts.net", addresses: ["100.64.0.3"] },
  ],
};

describe("parseTailnetImport (API)", () => {
  const r = parseTailnetImport(JSON.stringify(api));

  it("maps devices", () => {
    expect(r.source).toBe("api");
    expect(r.devices[0]).toEqual({
      id: 1,
      name: "alice-laptop",
      user: "alice@example.com",
      ipv4: "100.64.0.1",
      ipv6: "fd7a:115c:a1e0::1",
      os: "macos",
      posture: { "node:tsVersion": "1.76.1" },
    });
  });
  it("treats tagged devices as ownerless and maps routes", () => {
    expect(r.devices[1]).toMatchObject({
      name: "router",
      tags: ["tag:router"],
      routes: ["10.0.0.0/24", "0.0.0.0/0"],
      routesApproved: ["10.0.0.0/24"],
    });
    expect(r.devices[1]!.user).toBeUndefined();
  });
  it("dedupes names, falling back to the FQDN's first label", () => {
    expect(r.devices[2]!.name).toBe("router-2");
  });
  it("collects owners from untagged devices only", () => {
    expect(r.owners).toEqual(["alice@example.com"]);
  });
  it("accepts a bare array", () => {
    expect(parseTailnetImport(JSON.stringify(api.devices)).devices).toHaveLength(3);
  });
});

describe("parseTailnetImport (tailscale status --json)", () => {
  const status = {
    Self: { HostName: "me", TailscaleIPs: ["100.64.0.9", "fd7a:115c:a1e0::9"], OS: "linux", UserID: 7 },
    Peer: {
      k1: { HostName: "web", TailscaleIPs: ["100.64.0.10"], OS: "linux", Tags: ["tag:web"], UserID: 99 },
      k2: { HostName: "gw", TailscaleIPs: ["100.64.0.11"], UserID: 7, PrimaryRoutes: ["192.168.1.0/24"] },
    },
    User: { "7": { LoginName: "bob@example.com" }, "99": { LoginName: "tagged-devices" } },
  };
  const r = parseTailnetImport(JSON.stringify(status));

  it("maps self and peers", () => {
    expect(r.source).toBe("status");
    expect(r.devices.map((d) => d.name)).toEqual(["me", "web", "gw"]);
    expect(r.devices[0]).toMatchObject({ user: "bob@example.com", ipv4: "100.64.0.9" });
  });
  it("ignores the pseudo user on tagged peers", () => {
    expect(r.devices[1]!.user).toBeUndefined();
    expect(r.owners).toEqual(["bob@example.com"]);
  });
  it("treats primary routes as advertised and approved", () => {
    expect(r.devices[2]).toMatchObject({ routes: ["192.168.1.0/24"], routesApproved: ["192.168.1.0/24"] });
  });
});

describe("parseTailnetImport errors", () => {
  it.each([
    ["", /Paste/],
    ["not json", /valid JSON/],
    ['{"access_token":"tskey-api-x"}', /token response/],
    ['{"message":"API token invalid"}', /API token invalid/],
    ['{"foo":1}', /Unrecognized/],
    ['{"devices":[]}', /No devices/],
  ])("%s", (text, re) => {
    expect(() => parseTailnetImport(text)).toThrow(ImportError);
    expect(() => parseTailnetImport(text)).toThrow(re);
  });
});

describe("applyImport", () => {
  const snap: Snapshot = {
    users: [{ id: 5, email: "alice@example.com", role: "admin" }],
    groups: { "group:eng": ["alice@example.com"] },
    devices: [{ id: 1, name: "old" }],
    services: [{ name: "svc:db", vip: "100.100.100.1", backends: [1] }],
  };
  const r = parseTailnetImport(
    JSON.stringify({ devices: [{ hostname: "a", user: "alice@example.com" }, { hostname: "b", user: "new@example.com" }] }),
  );
  const out = applyImport(snap, r);

  it("replaces devices and keeps groups", () => {
    expect(out.devices.map((d) => d.name)).toEqual(["a", "b"]);
    expect(out.groups).toEqual(snap.groups);
  });
  it("adds only unknown owners as users", () => {
    expect(out.users).toEqual([
      { id: 5, email: "alice@example.com", role: "admin" },
      { id: 6, email: "new@example.com" },
    ]);
  });
  it("clears service backends, which referenced old device ids", () => {
    expect(out.services![0]).toMatchObject({ name: "svc:db", vip: "100.100.100.1" });
    expect(out.services![0]!.backends).toBeUndefined();
  });
});

// Shaped like real `tailscale status --json` output (anonymized): an iPhone whose
// self-reported HostName is "localhost", a tagged peer, and the tagged-devices
// pseudo user.
describe("parseTailnetImport (real-world status shape)", () => {
  const status = {
    Version: "1.104.5-tabc",
    Self: { ID: "n1", HostName: "dev-box", DNSName: "dev-box.example-net.ts.net.", OS: "linux", UserID: 11, TailscaleIPs: ["100.64.0.1", "fd7a:115c:a1e0::1"] },
    Peer: {
      "nodekey:aaa": { HostName: "net-demo", DNSName: "net-demo.example-net.ts.net.", OS: "linux", UserID: 22, Tags: ["tag:demo"], TailscaleIPs: ["100.64.0.2"] },
      "nodekey:bbb": { HostName: "localhost", DNSName: "iphone192.example-net.ts.net.", OS: "iOS", UserID: 11, TailscaleIPs: ["100.64.0.3"] },
      // tagged-devices owner but no Tags field: still not a person
      "nodekey:ccc": { HostName: "orphan", DNSName: "orphan.example-net.ts.net.", OS: "linux", UserID: 22, TailscaleIPs: ["100.64.0.4"] },
    },
    User: {
      "11": { ID: 11, LoginName: "me@example.com" },
      "22": { ID: 22, LoginName: "tagged-devices" },
    },
  };
  const r = parseTailnetImport(JSON.stringify(status));

  it("keeps tags on tagged devices", () => {
    expect(r.devices.find((d) => d.name === "net-demo")).toMatchObject({ tags: ["tag:demo"] });
    expect(r.devices.find((d) => d.name === "net-demo")!.user).toBeUndefined();
  });
  it("names devices by MagicDNS name, not the self-reported HostName", () => {
    expect(r.devices.map((d) => d.name)).toEqual(["dev-box", "net-demo", "iphone192", "orphan"]);
    expect(r.devices[2]!.os).toBe("ios");
  });
  it("never treats tagged-devices as an owner", () => {
    expect(r.owners).toEqual(["me@example.com"]);
    expect(r.devices[3]!.user).toBeUndefined();
  });
});

it("API: prefers the MagicDNS name over a bogus hostname", () => {
  const r = parseTailnetImport(JSON.stringify({ devices: [{ hostname: "localhost", name: "iphone192.example-net.ts.net", user: "tagged-devices" }] }));
  expect(r.devices[0]).toEqual({ id: 1, name: "iphone192" });
  expect(r.owners).toEqual([]);
});

describe("services", () => {
  // Shaped like the CapMap of a real status --json Self node (anonymized).
  const withServices = (capMap: unknown) =>
    JSON.stringify({
      Self: { HostName: "me", DNSName: "me.example-net.ts.net.", UserID: 1, TailscaleIPs: ["100.64.0.1"], CapMap: capMap },
      Peer: {},
      User: { "1": { LoginName: "me@example.com" } },
    });
  const cap = {
    "default-auto-update": [true],
    https: null,
    "services/rps-db": [{ Name: "svc:rps-db", Addrs: ["fd7a:115c:a1e0::1b36:de24", "100.70.222.35"], Ports: ["tcp:443"] }],
    "services/cache": [{ Addrs: ["100.70.222.36"] }],
  };

  it("reads services/<name> capabilities, preferring the IPv4 VIP", () => {
    expect(parseTailnetImport(withServices(cap)).services).toEqual([
      { name: "svc:rps-db", vip: "100.70.222.35" },
      { name: "svc:cache", vip: "100.70.222.36" },
    ]);
  });
  it("ignores other capabilities and malformed entries", () => {
    const r = parseTailnetImport(withServices({ "services/x": [{ Name: "svc:x" }, "junk", null], "services/y": "nope", ssh: [{ Addrs: ["1.2.3.4"] }] }));
    expect(r.services).toEqual([]);
  });
  it("dedupes a service seen on several nodes", () => {
    const peerCap = { "services/rps-db": [{ Name: "svc:rps-db", Addrs: ["100.70.222.35"] }] };
    const text = JSON.stringify({
      Self: { HostName: "a", DNSName: "a.n.ts.net.", TailscaleIPs: ["100.64.0.1"], CapMap: peerCap },
      Peer: { k: { HostName: "b", DNSName: "b.n.ts.net.", TailscaleIPs: ["100.64.0.2"], CapMap: peerCap } },
    });
    expect(parseTailnetImport(text).services).toHaveLength(1);
  });
  it("API output has no services", () => {
    expect(parseTailnetImport(JSON.stringify(api)).services).toEqual([]);
  });

  describe("mergeServices", () => {
    const existing = [
      { name: "svc:db", vip: "100.100.100.1", backends: [1, 2] },
      { name: "web", vip: "100.100.100.2", backends: [3] },
    ];
    it("updates the VIP by name (with or without svc:), appends new ones, keeps the rest, clears backends", () => {
      const out = mergeServices(existing, [
        { name: "db", vip: "100.70.0.9" },
        { name: "svc:cache", vip: "100.70.0.10" },
      ]);
      expect(out).toEqual([
        { name: "svc:db", vip: "100.70.0.9", backends: undefined },
        { name: "web", vip: "100.100.100.2", backends: undefined },
        { name: "svc:cache", vip: "100.70.0.10" },
      ]);
    });
    it("matches names case-insensitively", () => {
      expect(mergeServices([{ name: "svc:DB", vip: "1.1.1.1" }], [{ name: "svc:db", vip: "2.2.2.2" }])).toHaveLength(1);
    });
  });

  it("applyImport adds imported services to a snapshot that had none", () => {
    const r = parseTailnetImport(withServices(cap));
    const out = applyImport({ users: [], devices: [] }, r);
    expect(out.services?.map((s) => s.name)).toEqual(["svc:rps-db", "svc:cache"]);
  });
  it("applyImport leaves services unset when there are none anywhere", () => {
    const r = parseTailnetImport(JSON.stringify(api));
    expect(applyImport({ users: [], devices: [] }, r).services).toBeUndefined();
  });
});

describe("owner role and Self version", () => {
  const OWNER = "https://tailscale.com/cap/is-owner";
  const ADMIN = "https://tailscale.com/cap/is-admin";
  const status = (selfCaps: Record<string, unknown> | undefined, extra: Record<string, unknown> = {}) =>
    JSON.stringify({
      Version: "1.104.5-t788384763-gd9218e278",
      Self: { HostName: "me", DNSName: "me.n.ts.net.", UserID: 1, TailscaleIPs: ["100.64.0.1"], CapMap: selfCaps },
      Peer: {
        k: { HostName: "peer", DNSName: "peer.n.ts.net.", UserID: 2, TailscaleIPs: ["100.64.0.2"], CapMap: { [OWNER]: null } },
      },
      User: { "1": { LoginName: "me@example.com" }, "2": { LoginName: "other@example.com" } },
      ...extra,
    });

  it("reads an owner from the Self node's capabilities", () => {
    expect(parseTailnetImport(status({ [OWNER]: null, [ADMIN]: null })).roles).toEqual([{ email: "me@example.com", role: "owner" }]);
  });
  it("reads an admin when there is no owner capability", () => {
    expect(parseTailnetImport(status({ [ADMIN]: null })).roles).toEqual([{ email: "me@example.com", role: "admin" }]);
  });
  it("reports no role without the capabilities, and ignores peers' capabilities", () => {
    expect(parseTailnetImport(status({ ssh: null })).roles).toEqual([]);
    expect(parseTailnetImport(status(undefined)).roles).toEqual([]);
  });
  it("gives a tagged Self no role", () => {
    const text = status({ [OWNER]: null }, { User: { "1": { LoginName: "tagged-devices" } } });
    expect(parseTailnetImport(text).roles).toEqual([]);
  });
  it("API output has no roles", () => {
    expect(parseTailnetImport(JSON.stringify(api)).roles).toEqual([]);
  });

  it("puts the client version on the Self device only", () => {
    const r = parseTailnetImport(status({}));
    expect(r.devices[0]!.posture).toEqual({ "node:tsVersion": "1.104.5" });
    expect(r.devices[1]!.posture).toBeUndefined();
  });
  it("sets no posture when the version is missing or unparseable", () => {
    const text = JSON.stringify({ Version: "dev", Self: { HostName: "me", DNSName: "me.n.ts.net.", TailscaleIPs: ["100.64.0.1"] } });
    expect(parseTailnetImport(text).devices[0]!.posture).toBeUndefined();
  });

  describe("applyImport", () => {
    const r = parseTailnetImport(status({ [OWNER]: null }));
    it("gives a new user the role", () => {
      const out = applyImport({ users: [], devices: [] }, r);
      expect(out.users).toEqual([
        { id: 1, email: "me@example.com", role: "owner" },
        { id: 2, email: "other@example.com" },
      ]);
    });
    it("replaces the role of a user already in the snapshot, and leaves others alone", () => {
      const out = applyImport(
        { users: [{ id: 7, email: "me@example.com", role: "member" }, { id: 8, email: "x@example.com", role: "admin" }], devices: [] },
        r,
      );
      expect(out.users).toEqual([
        { id: 7, email: "me@example.com", role: "owner" },
        { id: 8, email: "x@example.com", role: "admin" },
        { id: 9, email: "other@example.com" },
      ]);
    });
  });
});

describe("API users and services", () => {
  const devices = [{ name: "a.n.ts.net", hostname: "a", user: "boss@example.com", addresses: ["100.64.0.1"], os: "linux", clientVersion: "v1.36.0" }];
  const doc = (extra: object) => JSON.stringify({ devices, ...extra });

  it("accepts a leading v on the client version", () => {
    expect(parseTailnetImport(doc({})).devices[0]!.posture).toEqual({ "node:tsVersion": "1.36.0" });
  });
  it("reads roles from users, skipping members and shared-in users", () => {
    const users = [
      { loginName: "boss@example.com", role: "owner", type: "member" },
      { loginName: "ops@example.com", role: "it-admin", type: "member" },
      { loginName: "dev@example.com", role: "member", type: "member" },
      { loginName: "guest@other.com", role: "admin", type: "shared" },
      { loginName: "boss@example.com", role: "admin", type: "member" },
      "junk",
    ];
    expect(parseTailnetImport(doc({ users })).roles).toEqual([
      { email: "boss@example.com", role: "owner" },
      { email: "ops@example.com", role: "it-admin" },
    ]);
  });
  it("reads vipServices, preferring the IPv4 VIP, skipping ones without an address", () => {
    const vipServices = [
      { name: "svc:db", addrs: ["fd7a:115c:a1e0::3456:3cb4", "100.93.49.180"], ports: ["tcp:443"] },
      { name: "svc:nope", addrs: [] },
      { addrs: ["100.1.1.1"] },
    ];
    expect(parseTailnetImport(doc({ vipServices })).services).toEqual([{ name: "svc:db", vip: "100.93.49.180" }]);
  });
  it("ignores error responses merged in for scopes the client lacks", () => {
    const r = parseTailnetImport(doc({ message: "calling actor does not have enough permissions" }));
    expect(r.roles).toEqual([]);
    expect(r.services).toEqual([]);
    expect(r.devices).toHaveLength(1);
  });
  it("applyImport adds role holders without devices as users, and gives them the role", () => {
    const r = parseTailnetImport(doc({ users: [{ loginName: "ops@example.com", role: "it-admin", type: "member" }] }));
    const out = applyImport({ users: [], devices: [] }, r);
    expect(out.users).toEqual([
      { id: 1, email: "boss@example.com" },
      { id: 2, email: "ops@example.com", role: "it-admin" },
    ]);
  });
});
