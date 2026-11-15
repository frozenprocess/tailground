import type { Device, Service, Snapshot, SnapshotUser } from "../engine/types";
import { isObj, str, strList, type Obj } from "./json";

// Convert pasted Tailscale output into snapshot devices. Everything here runs in
// the browser on text the user pasted; nothing is fetched or sent anywhere.
//
// Two shapes are understood:
//   - the control-plane API: GET /api/v2/tailnet/-/devices?fields=all
//   - the CLI: `tailscale status --json`

export type ImportSource = "api" | "status";

export interface ImportResult {
  source: ImportSource;
  devices: Device[];
  /** owner emails seen on the devices, in first-seen order */
  owners: string[];
  /** Tailscale Services found in the paste (CLI only), without backends */
  services: Service[];
  /** admin roles read from the paste (CLI only: the machine's own user) */
  roles: UserRole[];
}

export interface UserRole {
  email: string;
  /** a snapshot role: owner, admin, it-admin, network-admin, billing-admin, auditor */
  role: string;
}

export class ImportError extends Error {}

export function parseTailnetImport(text: string): ImportResult {
  const trimmed = text.trim();
  if (!trimmed) throw new ImportError("Paste the command output first.");
  let data: unknown;
  try {
    data = JSON.parse(trimmed);
  } catch {
    throw new ImportError("That isn't valid JSON. Paste the whole output of the command, unmodified.");
  }

  if (isObj(data)) {
    if (typeof data.access_token === "string") {
      throw new ImportError("That's the token response. Paste the output of the second command (the device list).");
    }
    if (Array.isArray(data.devices)) return fromApi(data.devices, data);
    if (isObj(data.Peer) || isObj(data.Self)) return fromStatus(data);
    const msg = str(data.message) || str(data.error);
    if (msg) throw new ImportError(`Tailscale returned an error: ${msg}`);
  } else if (Array.isArray(data)) {
    return fromApi(data);
  }
  throw new ImportError(
    "Unrecognized format. Expected the API device list or the output of `tailscale status --json`.",
  );
}

/** First dotted-numeric prefix of a client version ("1.76.1-t5d3…" or "v1.76.1" → "1.76.1"). */
function cleanVersion(v: unknown): string | undefined {
  const m = /^v?(\d+(?:\.\d+){1,2})/.exec(str(v));
  return m ? m[1] : undefined;
}

/** MagicDNS name: first label of a FQDN ("iphone192.tail1234.ts.net." → "iphone192"). */
const firstLabel = (v: unknown): string => str(v).split(".")[0] ?? "";

/** Tailscale's stand-in owner for tagged devices; not a person. */
const TAGGED_OWNER = "tagged-devices";

const nonEmpty = <T>(a: T[]): T[] | undefined => (a.length ? a : undefined);

/** What each source contributes for one node, before it becomes a Device. */
interface NodeFields {
  name: string;
  tags: string[];
  /** owner's login; ignored when the node is tagged */
  owner: string;
  addresses: string[];
  os: string;
  advertised: string[];
  approved: string[];
  version?: string;
}

type Draft = Omit<Device, "id">;

/** Shared device assembly for both sources. Records the owner in `owners`. */
function toDraft(f: NodeFields, owners: string[]): Draft {
  // Tagged devices are owned by their tags, not a person.
  const person = f.owner.toLowerCase() === TAGGED_OWNER ? "" : f.owner;
  const user = f.tags.length === 0 && person ? person : undefined;
  if (user && !owners.includes(user)) owners.push(user);
  return {
    name: f.name,
    user,
    tags: nonEmpty(f.tags),
    ipv4: f.addresses.find((a) => a.includes(".")),
    ipv6: f.addresses.find((a) => a.includes(":")),
    os: f.os.toLowerCase() || undefined,
    routes: nonEmpty(f.advertised),
    routesApproved: nonEmpty(f.approved),
    posture: f.version ? { "node:tsVersion": f.version } : undefined,
  };
}

/**
 * `doc` is the whole response when it came from the saved file: besides `devices`
 * it may carry `users` (→ roles) and `vipServices` (→ services), which the
 * import command fetches with extra calls. Missing or error responses are skipped.
 */
function fromApi(list: unknown[], doc: Obj = {}): ImportResult {
  const owners: string[] = [];
  const drafts: Draft[] = [];
  for (const item of list) {
    if (!isObj(item)) continue;
    // Prefer the MagicDNS name: `hostname` is whatever the device reports about
    // itself (an iPhone says "localhost"), and the MagicDNS name is what the
    // admin console and the policy use.
    const name = firstLabel(item.name) || str(item.hostname);
    if (!name) continue;
    drafts.push(
      toDraft(
        {
          name,
          tags: strList(item.tags),
          owner: str(item.user),
          addresses: strList(item.addresses),
          os: str(item.os),
          advertised: strList(item.advertisedRoutes),
          approved: strList(item.enabledRoutes),
          version: cleanVersion(item.clientVersion),
        },
        owners,
      ),
    );
  }
  return finish("api", drafts, owners, { services: apiServices(doc.vipServices), roles: apiRoles(doc.users) });
}

function fromStatus(status: Obj): ImportResult {
  const users = isObj(status.User) ? status.User : {};
  const loginOf = (id: unknown): string => {
    const u = users[String(id)];
    return isObj(u) ? str(u.LoginName) : "";
  };
  const nodes: Obj[] = [];
  if (isObj(status.Self)) nodes.push(status.Self);
  if (isObj(status.Peer)) for (const p of Object.values(status.Peer)) if (isObj(p)) nodes.push(p);

  const owners: string[] = [];
  const drafts: Draft[] = [];
  const services = servicesFrom(nodes);
  // `status --json` reports the client version for this machine only.
  const selfVersion = cleanVersion(status.Version);
  for (const n of nodes) {
    // Same reason as the API path: DNSName, not the self-reported HostName.
    const name = firstLabel(n.DNSName) || str(n.HostName);
    if (!name) continue;
    // Status only reports the routes a node is primary for, i.e. approved ones.
    const routes = strList(n.PrimaryRoutes);
    drafts.push(
      toDraft(
        {
          name,
          tags: strList(n.Tags),
          owner: loginOf(n.UserID),
          addresses: strList(n.TailscaleIPs),
          os: str(n.OS),
          advertised: routes,
          approved: routes,
          version: n === status.Self ? selfVersion : undefined,
        },
        owners,
      ),
    );
  }
  const role = isObj(status.Self) ? selfRole(status.Self, loginOf(status.Self.UserID)) : undefined;
  return finish("status", drafts, owners, { services, roles: role ? [role] : [] });
}

const CAP_OWNER = "https://tailscale.com/cap/is-owner";
const CAP_ADMIN = "https://tailscale.com/cap/is-admin";

/**
 * The machine's own user is an owner or admin if its node carries the matching
 * capability. Only this node says so: peers' capabilities aren't reported.
 */
function selfRole(self: Obj, login: string): UserRole | undefined {
  if (!login || login.toLowerCase() === TAGGED_OWNER || !isObj(self.CapMap)) return undefined;
  if (CAP_OWNER in self.CapMap) return { email: login, role: "owner" };
  if (CAP_ADMIN in self.CapMap) return { email: login, role: "admin" };
  return undefined;
}

/** Roles from `GET /tailnet/-/users`. Plain members and shared-in users carry none. */
function apiRoles(users: unknown): UserRole[] {
  if (!Array.isArray(users)) return [];
  const roles: UserRole[] = [];
  for (const u of users) {
    if (!isObj(u) || str(u.type).toLowerCase() === "shared") continue;
    const email = str(u.loginName);
    const role = str(u.role).toLowerCase();
    if (email && role && role !== "member" && !roles.some((r) => r.email === email)) roles.push({ email, role });
  }
  return roles;
}

/** Services from `GET /tailnet/-/services` (`vipServices`), preferring the IPv4 VIP. */
function apiServices(list: unknown): Service[] {
  if (!Array.isArray(list)) return [];
  const found = new Map<string, Service>();
  for (const e of list) {
    if (!isObj(e)) continue;
    const name = str(e.name);
    const addrs = strList(e.addrs);
    const vip = pickVip(addrs);
    if (name && vip && !found.has(serviceKey(name))) found.set(serviceKey(name), { name, vip });
  }
  return [...found.values()];
}

/** A service's VIP: the IPv4 address if there is one, else the first. */
const pickVip = (addrs: string[]): string | undefined => addrs.find((a) => a.includes(".")) ?? addrs[0];

/** Service name without its "svc:" prefix, lower-cased: the key services merge on. */
export const serviceKey = (name: string): string => name.replace(/^svc:/i, "").toLowerCase();

/**
 * Tailscale Services a node can see, from its `services/<name>` capabilities
 * (`tailscale status --json` → CapMap). Each carries the service's VIPs; which
 * devices back it isn't reported, so backends are left empty. Deduped by name.
 */
function servicesFrom(nodes: Obj[]): Service[] {
  const found = new Map<string, Service>();
  for (const n of nodes) {
    if (!isObj(n.CapMap)) continue;
    for (const [cap, entries] of Object.entries(n.CapMap)) {
      if (!cap.startsWith("services/") || !Array.isArray(entries)) continue;
      for (const e of entries) {
        if (!isObj(e)) continue;
        const name = str(e.Name) || `svc:${cap.slice("services/".length)}`;
        const addrs = strList(e.Addrs);
        const vip = pickVip(addrs);
        if (!vip || found.has(serviceKey(name))) continue;
        found.set(serviceKey(name), { name, vip });
      }
    }
  }
  return [...found.values()];
}

/** Drop undefined fields so the JSON view stays tidy. */
const compact = <T extends object>(o: T): T =>
  Object.fromEntries(Object.entries(o).filter(([, v]) => v !== undefined)) as T;

/** Number the devices and make their names unique (the engine keys on name). */
function finish(
  source: ImportSource,
  drafts: Draft[],
  owners: string[],
  extras: { services: Service[]; roles: UserRole[] } = { services: [], roles: [] },
): ImportResult {
  if (drafts.length === 0) throw new ImportError("No devices found in that output.");
  const seen = new Map<string, number>();
  const devices = drafts.map((d, i): Device => {
    const n = (seen.get(d.name) ?? 0) + 1;
    seen.set(d.name, n);
    return compact({ id: i + 1, ...d, name: n === 1 ? d.name : `${d.name}-${n}` });
  });
  return { source, devices, owners, ...extras };
}

/**
 * Replace the snapshot's devices with the imported ones. Owners (and users holding a role) not
 * already in the snapshot are added as users so device→user references resolve. Groups,
 * hosts and external tailnets are left alone.
 *
 * A user whose role the paste reports (owner/admin) gets that role, whether new or
 * already in the snapshot.
 *
 * Services merge by name: an imported service updates the VIP of the snapshot's
 * service of the same name, and new ones are appended. Backends are device ids,
 * which are renumbered, so every service loses its backends (the paste doesn't
 * say which devices back a service); pick them again in the Services section.
 */
export function applyImport(snapshot: Snapshot, result: ImportResult): Snapshot {
  const next: Snapshot = {
    ...snapshot,
    users: mergeUsers(snapshot.users, result),
    devices: result.devices,
  };
  const services = mergeServices(snapshot.services ?? [], result.services);
  if (services.length > 0) next.services = services;
  return next;
}

/**
 * Existing users keep their place; one whose role the import reports takes that
 * role (a role read from real data beats what the snapshot had). Device owners
 * and role holders the snapshot doesn't have yet are appended with fresh ids.
 */
function mergeUsers(existing: SnapshotUser[], result: ImportResult): SnapshotUser[] {
  const roleOf = new Map(result.roles.map((r) => [r.email, r.role]));
  const known = new Set(existing.map((u) => u.email));
  let nextId = existing.reduce((m, u) => Math.max(m, u.id), 0) + 1;
  const updated = existing.map((u) => (roleOf.has(u.email) ? { ...u, role: roleOf.get(u.email) } : u));
  // Users holding a role count even when they own no device.
  const wanted = new Set([...result.owners, ...roleOf.keys()]);
  const added = [...wanted]
    .filter((email) => !known.has(email))
    .map((email): SnapshotUser => ({ id: nextId++, email, ...(roleOf.has(email) ? { role: roleOf.get(email) } : {}) }));
  return [...updated, ...added];
}

/** Merge imported services into existing ones by name; backends are cleared. */
export function mergeServices(existing: Service[], imported: Service[]): Service[] {
  const vips = new Map(imported.map((s) => [serviceKey(s.name), s.vip]));
  const merged = existing.map((s) => ({ ...s, vip: vips.get(serviceKey(s.name)) ?? s.vip, backends: undefined }));
  const have = new Set(existing.map((s) => serviceKey(s.name)));
  return [...merged, ...imported.filter((s) => !have.has(serviceKey(s.name))).map((s) => ({ ...s }))];
}
