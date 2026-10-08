// Stable positional wire format; keep compatibility fixtures unchanged during refactors.
import { deflateSync, inflateSync, strFromU8, strToU8 } from "fflate";
import { DEFAULT_SSH_USERS, defaultProbes, type Probe, type Snapshot } from "../engine/types";
import { DEFAULT_PRESET, DEFAULT_TEMPLATE, POLICY_TEMPLATES, SNAPSHOT_PRESETS } from "../state/presets";
import type { Grouping, MainView } from "../state/navigation";

export interface SharedState {
  policy: string;
  snapshot: Snapshot;
  probes: Probe[];
  sshUsers?: string[];
  view?: MainView;
  activeProbe?: string;
  grouping?: Grouping;
  activeLesson?: string | null;
}

// Wire format: a positional JSON array (defaults omitted), raw-deflated with a
// preset dictionary built from the shipped templates/presets — scenarios that
// start from a preset share most of their bytes with it — then base64url.
// fflate rather than CompressionStream: works in every browser/WebView.
// The dictionary is derived from state/presets, so editing a template or preset
// invalidates previously issued links.
//   [policy, snapshot, probes|0, sshUsers|0, view, activeProbe, grouping, lesson|0]

const VIEWS: MainView[] = ["matrix", "graph", "tutorials", "about"];
const GROUPINGS: Grouping[] = ["user", "flat", "tag"];

let dict: Uint8Array | null = null;
function dictionary(): Uint8Array {
  // Window is 32 KiB and the tail is cheapest to reach, so the defaults go last.
  dict ??= strToU8(
    [
      ...POLICY_TEMPLATES.filter((t) => t !== DEFAULT_TEMPLATE).map((t) => t.policy),
      ...SNAPSHOT_PRESETS.filter((p) => p !== DEFAULT_PRESET).map((p) => JSON.stringify(p.snapshot)),
      DEFAULT_TEMPLATE.policy,
      JSON.stringify(DEFAULT_PRESET.snapshot),
    ]
      .join("\n")
      .slice(-32768),
  );
  return dict;
}

function toBase64Url(bytes: Uint8Array): string {
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function fromBase64Url(s: string): Uint8Array {
  const bin = atob(s.replace(/-/g, "+").replace(/_/g, "/"));
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

export function encodeShare(state: SharedState): string {
  const probes = JSON.stringify(state.probes) === JSON.stringify(defaultProbes()) ? 0 : state.probes;
  const ssh = JSON.stringify(state.sshUsers) === JSON.stringify(DEFAULT_SSH_USERS) ? 0 : state.sshUsers;
  const tuple = [
    state.policy,
    state.snapshot,
    probes,
    ssh ?? 0,
    Math.max(0, VIEWS.indexOf(state.view ?? "matrix")),
    state.activeProbe ?? "https",
    Math.max(0, GROUPINGS.indexOf(state.grouping ?? "user")),
    state.activeLesson ?? 0,
  ];
  const raw = strToU8(JSON.stringify(tuple));
  return toBase64Url(deflateSync(raw, { level: 9, dictionary: dictionary() }));
}

// A hostile link can deflate a few KB into hundreds of MB. Inflate into a
// fixed buffer (fflate truncates at its length instead of growing) and treat a
// full buffer as oversize. Real scenarios are well under 1 MiB.
const MAX_INFLATED_BYTES = 4 << 20;

// A shared payload is untrusted: the store and views index straight into it
// (devices.map, tags.join, Object.entries(groups)…), so a wrong-typed nested
// field crashes the recipient's UI. Validate the shape up front; anything off
// makes the whole link invalid. Unknown extra keys are tolerated.
const MAX_STR = 4096;
const MAX_ITEMS = 5000;

const isObj = (v: unknown): v is Record<string, unknown> =>
  typeof v === "object" && v !== null && !Array.isArray(v);
const isStr = (v: unknown): v is string => typeof v === "string" && v.length <= MAX_STR;
const isInt = (v: unknown): v is number => Number.isSafeInteger(v);
const optStr = (v: unknown) => v === undefined || isStr(v);
const list = (v: unknown, each: (x: unknown) => boolean) =>
  Array.isArray(v) && v.length <= MAX_ITEMS && v.every(each);
const optList = (v: unknown, each: (x: unknown) => boolean) => v === undefined || list(v, each);
const strMap = (v: unknown, val: (x: unknown) => boolean) =>
  isObj(v) && Object.keys(v).length <= MAX_ITEMS && Object.values(v).every(val);

function validSnapshot(v: unknown): v is Snapshot {
  if (!isObj(v)) return false;
  return (
    list(v.users, (u) => isObj(u) && isInt(u.id) && isStr(u.email) && optStr(u.role) && (u.shared === undefined || typeof u.shared === "boolean")) &&
    list(
      v.devices,
      (d) =>
        isObj(d) &&
        isInt(d.id) &&
        isStr(d.name) &&
        optStr(d.user) &&
        optStr(d.ipv4) &&
        optStr(d.ipv6) &&
        optStr(d.os) &&
        optList(d.tags, isStr) &&
        optList(d.routes, isStr) &&
        optList(d.routesApproved, isStr) &&
        (d.posture === undefined || strMap(d.posture, isStr)),
    ) &&
    (v.groups === undefined || strMap(v.groups, (g) => list(g, isStr))) &&
    (v.hosts === undefined || strMap(v.hosts, isStr)) &&
    optList(v.services, (x) => isObj(x) && isStr(x.name) && isStr(x.vip) && optList(x.backends, isInt)) &&
    optList(
      v.externalTailnets,
      (x) =>
        isObj(x) &&
        isStr(x.alias) &&
        optList(
          x.nodes,
          (n) => isObj(n) && isStr(n.name) && optStr(n.ipv4) && optStr(n.ipv6) && optList(n.tags, isStr) && optList(n.groups, isStr),
        ),
    )
  );
}

const validProbes = (v: unknown): v is Probe[] =>
  list(v, (p) => isObj(p) && isStr(p.name) && isStr(p.proto) && (p.port === undefined || (isInt(p.port) && p.port >= 0 && p.port <= 65535)));

export function decodeShare(token: string): SharedState | null {
  try {
    const raw = inflateSync(fromBase64Url(token), {
      dictionary: dictionary(),
      out: new Uint8Array(MAX_INFLATED_BYTES),
    });
    if (raw.length >= MAX_INFLATED_BYTES) return null;
    const t = JSON.parse(strFromU8(raw));
    if (!Array.isArray(t) || typeof t[0] !== "string" || !validSnapshot(t[1])) return null;
    if (t[2] && !validProbes(t[2])) return null;
    if (t[3] && !list(t[3], isStr)) return null;
    if (t[5] !== undefined && !isStr(t[5])) return null;
    if (t[7] && !isStr(t[7])) return null;
    return {
      policy: t[0],
      snapshot: t[1],
      probes: t[2] || defaultProbes(),
      sshUsers: t[3] || DEFAULT_SSH_USERS,
      view: VIEWS[t[4]] ?? "matrix",
      activeProbe: t[5],
      grouping: GROUPINGS[t[6]] ?? "user",
      activeLesson: t[7] || null,
    };
  } catch {
    return null;
  }
}

