// Mirror of engine/eval/types.go — the only contract the web app knows about the
// engine. Keep field names and JSON tags in lockstep with the Go structs.

export interface EngineRequest {
  policy: string;
  snapshot: Snapshot;
  probes: Probe[];
  sshUsers?: string[];
}

export interface Snapshot {
  users: SnapshotUser[];
  groups?: Record<string, string[]>;
  hosts?: Record<string, string>;
  devices: Device[];
  /** Tailscale Services — a named VIP fronting backend devices, targetable with svc:<name> */
  services?: Service[];
  /** other tailnets whose nodes are shared into this one (declarative node sharing) */
  externalTailnets?: ExternalTailnet[];
}

export interface ExternalTailnet {
  /** local handle used in group://<alias>/<name> / tag://<alias>/<name> tokens */
  alias: string;
  nodes?: ExternalNode[];
}

export interface ExternalNode {
  name: string;
  /** tags in its home tailnet (tag: prefix optional) */
  tags?: string[];
  /** groups it belongs to in its home tailnet (group: prefix optional) */
  groups?: string[];
  ipv4?: string;
  ipv6?: string;
}

export interface Service {
  /** service name, with or without a leading "svc:" */
  name: string;
  /** the service virtual IP (a single v4 or v6 address) */
  vip: string;
  /** device IDs that advertise the service */
  backends?: number[];
}

export interface SnapshotUser {
  id: number;
  email: string;
  /** owner | admin | it-admin | network-admin | billing-admin | auditor | member. Empty ⇒ member. */
  role?: string;
  /** joined via a share invitation — feeds autogroup:shared. */
  shared?: boolean;
}

export interface Device {
  id: number;
  name: string;
  user?: string;
  tags?: string[];
  ipv4?: string;
  ipv6?: string;
  os?: string;
  /** subnet / exit routes this device advertises */
  routes?: string[];
  /** the subset of `routes` an admin has approved (both needed to carry traffic) */
  routesApproved?: string[];
  posture?: Record<string, string>;
}

export interface Probe {
  name: string;
  /** "tcp" | "udp" | "sctp" | "icmp" | "icmpv6", any other IANA proto name
   *  ("gre", "esp", "ipv4", …), or a raw protocol number 0-255 as a string. */
  proto: string;
  port?: number;
}

export type Verdict = "allow" | "deny";
export type SSHVerdict = "accept" | "check" | "reject" | "deny";

export interface EngineResult {
  matrices: Record<string, Record<string, Verdict>>;
  grants?: Record<string, string[]>;
  nodeCaps?: Record<string, Record<string, string[]>>;
  rules?: Record<string, FilterRuleView[]>;
  ssh?: Record<string, SSHVerdict>;
  posture?: PostureReport;
  routes?: RouteReport;
  services?: ServiceReport;
  externalShare?: ExternalShareReport;
  tests?: TestReport;
  actors: Actor[];
  diagnostics?: Diagnostic[];
  engine: EngineInfo;
}

export interface PostureReport {
  /** posture name -> its condition strings */
  definitions: Record<string, string[]>;
  /** policy-wide `defaultSrcPosture` baseline, applied to rules with no srcPosture */
  defaultSrcPosture?: string[];
  gates: PostureGate[];
  /** "srcID->dstID" -> why the posture layer downgraded that pair to deny */
  denials?: Record<string, string>;
  warnings?: string[];
}

export interface PostureGate {
  rule: string; // "acls[2]" | "grants[0]"
  postures: string[];
  /** gate inherited from the policy's top-level defaultSrcPosture */
  default?: boolean;
}

export interface RouteReport {
  paths: RoutePath[];
  warnings?: string[];
}

export interface RoutePath {
  prefix: string; // "10.88.0.0/24", "0.0.0.0/0"
  exit: boolean;
  routerId: number;
  router: string;
  approved: boolean;
  probe?: string; // sample address reachability was evaluated against
  column?: string; // matrix destination key, e.g. "route:10.88.0.0/24@3"
}

export interface ServiceReport {
  paths: ServicePath[];
  warnings?: string[];
}

export interface ServicePath {
  name: string; // "svc:web"
  vip: string; // "100.96.0.10"
  backends?: number[];
  column?: string; // matrix destination key, e.g. "svc:web"
}

export interface ExternalShareReport {
  tailnets: ExternalTailnetView[];
  warnings?: string[];
}

export interface ExternalTailnetView {
  alias: string;
  externalId?: string;
  allowIncomingConnections: boolean;
  allowExternalReferencesTo?: string[];
  configured: boolean;
  /** local device ids this tailnet's own policy may name as a src (from allowExternalReferencesTo) */
  referenceable: number[];
  nodes: ExternalNodeView[];
}

export interface ExternalNodeView {
  name: string;
  tags?: string[];
  groups?: string[];
  ips: string[];
  /** matrix key: "ext:corp/relay-><deviceId>" (inbound only — the other tailnet's policy decides outbound) */
  column: string;
  /** reverse-direction (shared node → local device) matrix keys are populated */
  inbound: boolean;
}

export interface TestReport {
  acl?: TestGroup;
  ssh?: TestGroup;
}

export interface TestGroup {
  passed: boolean;
  cases: TestCase[];
  rawFailures?: string[];
}

export interface TestCase {
  label: string;
  expect: "reachable" | "blocked" | "via check";
  pass: boolean;
  detail?: string;
}

export interface FilterRuleView {
  srcIPs: string[];
  dstPorts?: string[];
  ipProto?: number[];
  capGrant?: string[];
}

export interface Actor {
  id: number;
  name: string;
  kind: "user" | "tagged";
  user?: string;
  tags?: string[];
  ips: string[];
}

export type DiagnosticSeverity = "error" | "warning" | "info";
export type DiagnosticCode = "parse" | "policy" | "snapshot" | "limit" | "internal";

export interface Diagnostic {
  severity: DiagnosticSeverity;
  /** What to do about it: parse/policy — fix the policy; snapshot — fix the
   *  mock tailnet; limit — the simulation can't answer this; internal — a
   *  Tailground bug. */
  code: DiagnosticCode;
  pointer?: string; // "line:col" into the policy text
  /** The snapshot item it's about: "device:<name>", "services" or "external". */
  target?: string;
  message: string;
}

/** Who handles a feature, or why it isn't handled. Mirrors eval.CapabilityGroup. */
export type CapabilityGroup =
  | "headscale"
  | "tailground"
  | "expanded"
  | "not-modeled"
  | "by-design";

/** One row of the engine's capability manifest. `backtick spans` in name/notes are inline code. */
export interface Capability {
  id: string;
  group: CapabilityGroup;
  name: string;
  notes?: string;
}

export interface EngineInfo {
  headscaleCommit: string;
  tailscaleVersion: string;
  capabilities?: Capability[];
}

export const DEFAULT_SSH_USERS = ["root", "ubuntu"];

export function defaultProbes(): Probe[] {
  return [
    { name: "ssh", proto: "tcp", port: 22 },
    { name: "http", proto: "tcp", port: 80 },
    { name: "https", proto: "tcp", port: 443 },
    { name: "postgres", proto: "tcp", port: 5432 },
    { name: "dns", proto: "udp", port: 53 },
    { name: "icmp", proto: "icmp" },
  ];
}
