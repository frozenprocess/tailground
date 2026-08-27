// Package eval defines Tailground's engine-facing contract: a [Request]
// describing a policy plus a mock tailnet, and a [Result] carrying the computed
// reachability.
package eval

// Request is the wire shape the web app posts to the engine on every debounced
// edit.
type Request struct {
	Policy   string   `json:"policy"`   // raw HuJSON
	Snapshot Snapshot `json:"snapshot"` // the mock tailnet
	Probes   []Probe  `json:"probes"`   // (proto, port) tuples to evaluate; empty ⇒ default set
	SSHUsers []string `json:"sshUsers,omitempty"`
}

// Snapshot is the user-authored mock tailnet. It is intentionally Tailscale-
// shaped, not Headscale-shaped.
type Snapshot struct {
	Users            []User              `json:"users"`
	Groups           map[string][]string `json:"groups,omitempty"` // informational; policy `groups` is authoritative
	Hosts            map[string]string   `json:"hosts,omitempty"`  // informational; policy `hosts` is authoritative
	Devices          []Device            `json:"devices"`
	Services         []Service           `json:"services,omitempty"`
	ExternalTailnets []ExternalTailnet   `json:"externalTailnets,omitempty"`
}

type Service struct {
	// Name is the service name, with or without the leading "svc:"; it is
	// normalized to the bare form.
	Name string `json:"name"`
	// VIP is the service's virtual IP — a single address (v4 or v6).
	VIP string `json:"vip"`
	// Backends lists the device IDs that advertise the service.
	Backends []uint64 `json:"backends,omitempty"`
}

type User struct {
	ID    uint64 `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role,omitempty"`
	// Shared marks a user who joined via a share invitation. Feeds
	// autogroup:shared (C-C) and declarative node sharing (C-H).
	Shared bool `json:"shared,omitempty"`
}

// Device is one node in the mock tailnet.
type Device struct {
	ID             uint64            `json:"id"`
	Name           string            `json:"name"`
	User           string            `json:"user,omitempty"` // email; empty ⇒ a tagged device
	Tags           []string          `json:"tags,omitempty"`
	IPv4           string            `json:"ipv4,omitempty"`
	IPv6           string            `json:"ipv6,omitempty"`
	OS             string            `json:"os,omitempty"`
	Routes         []string          `json:"routes,omitempty"` // subnet routes / exit routes this device advertises
	RoutesApproved []string          `json:"routesApproved,omitempty"`
	Posture        map[string]string `json:"posture,omitempty"`
}

type ExternalTailnet struct {
	Alias string         `json:"alias"`
	Nodes []ExternalNode `json:"nodes,omitempty"`
}

// ExternalNode is one shared node as this tailnet sees it. Its tag / group
// membership is assigned by its home tailnet — which Tailground doesn't model —
// so the snapshot states it directly. Tags and groups may be given with or
// without their `tag:` / `group:` prefix.
type ExternalNode struct {
	Name   string   `json:"name"`
	Tags   []string `json:"tags,omitempty"`
	Groups []string `json:"groups,omitempty"`
	IPv4   string   `json:"ipv4,omitempty"`
	IPv6   string   `json:"ipv6,omitempty"`
}

// Probe is one (proto, port) evaluation profile.
type Probe struct {
	Name  string `json:"name"`
	Proto string `json:"proto"` // tcp | udp | icmp | sctp
	Port  int    `json:"port,omitempty"`
}

// Result is what the engine returns.
type Result struct {
	// Matrices is keyed by probe name, then by "srcID->dstID", to a verdict.
	Matrices map[string]map[string]string `json:"matrices"`
	// Grants is keyed by "srcID->dstID" to the application capability names the
	// source is granted on the destination (from `grants`/`acls` app grants).
	Grants map[string][]string `json:"grants,omitempty"`
	// NodeCaps is keyed by device ID to that device's own capability map
	// (from `nodeAttrs`): capability name → its raw JSON argument values.
	NodeCaps map[uint64]map[string][]string `json:"nodeCaps,omitempty"`
	// Rules is keyed by device ID to the packet filter that device would
	// receive — the Tailscale-native compiled artifact, for the inspector.
	// When the policy uses device posture, posture-gated rules are applied
	// separately (see Posture) and are not represented here.
	Rules map[uint64][]FilterRuleView `json:"rules,omitempty"`
	// SSH is keyed by "user@srcID->dstID" to an SSH verdict. Populated only
	// when the policy declares an `ssh` block.
	SSH           map[string]string    `json:"ssh,omitempty"`
	Posture       *PostureReport       `json:"posture,omitempty"`
	Routes        *RouteReport         `json:"routes,omitempty"`
	Services      *ServiceReport       `json:"services,omitempty"`
	ExternalShare *ExternalShareReport `json:"externalShare,omitempty"`
	// Tests is the outcome of the policy's own `tests` / `sshTests` blocks,
	// present only when the policy declares them.
	Tests       *TestReport  `json:"tests,omitempty"`
	Actors      []Actor      `json:"actors"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	Engine      EngineInfo   `json:"engine"`
}

// TestReport groups the policy's self-check results by kind.
type TestReport struct {
	ACL *TestGroup `json:"acl,omitempty"`
	SSH *TestGroup `json:"ssh,omitempty"`
}

// TestGroup is one kind of self-check (`tests` or `sshTests`).
type TestGroup struct {
	// Passed is the engine's authoritative overall verdict for the block.
	Passed bool `json:"passed"`
	// Cases enumerates each assertion, marked pass/fail on a best-effort basis.
	Cases []TestCase `json:"cases"`
	// RawFailures holds failure lines that couldn't be tied to a case.
	RawFailures []string `json:"rawFailures,omitempty"`
}

// TestCase is one assertion from a `tests` / `sshTests` entry.
type TestCase struct {
	Label  string `json:"label"`
	Expect string `json:"expect"` // "reachable" | "blocked" | "via check"
	Pass   bool   `json:"pass"`
	Detail string `json:"detail,omitempty"`
}

// PostureReport is the outcome of the device-posture layer.
type PostureReport struct {
	// Definitions echoes each posture name to its condition strings, for the UI.
	Definitions map[string][]string `json:"definitions"`
	// DefaultSrcPosture echoes the policy's top-level `defaultSrcPosture` list:
	// the baseline posture names applied to every rule with no srcPosture of its
	// own. Empty ⇒ no policy-wide default.
	DefaultSrcPosture []string `json:"defaultSrcPosture,omitempty"`
	// Gates is one entry per posture-gated policy rule.
	Gates []PostureGate `json:"gates"`
	// Denials maps "srcID->dstID" to the reason the posture layer downgraded
	// that pair from allow to deny. Only flipped pairs appear.
	Denials map[string]string `json:"denials,omitempty"`
	// Warnings collects posture-grammar problems and undefined-posture refs.
	Warnings []string `json:"warnings,omitempty"`
}

// PostureGate records that one policy rule is conditioned on device posture.
type PostureGate struct {
	Rule     string   `json:"rule"`     // "acls[2]" | "grants[0]"
	Postures []string `json:"postures"` // OR-ed; the src must satisfy one
	// Default is true when this gate comes from the policy's top-level
	// `defaultSrcPosture` rather than a srcPosture on the rule itself.
	Default bool `json:"default,omitempty"`
}

type RouteReport struct {
	// Paths is one entry per (advertised route, advertising device) pair,
	// sorted by prefix then router ID. Includes advertised-but-unapproved
	// routes (Approved=false, no matrix column).
	Paths []RoutePath `json:"paths"`
	// Warnings collects unparseable route strings and approve-without-advertise
	// mismatches.
	Warnings []string `json:"warnings,omitempty"`
}

// RoutePath is one device advertising one prefix.
type RoutePath struct {
	Prefix   string `json:"prefix"`   // "10.88.0.0/24", "0.0.0.0/0"
	Exit     bool   `json:"exit"`     // prefix is an exit route (0.0.0.0/0 or ::/0)
	RouterID uint64 `json:"routerId"` // the advertising device
	Router   string `json:"router"`   // its name
	Approved bool   `json:"approved"` // advertised AND admin-approved
	// Probe is the address inside Prefix that reachability was evaluated
	// against ("10.88.0.1", "8.8.8.8" for an exit route).
	Probe string `json:"probe,omitempty"`
	// Column is the matrix destination key ("route:10.88.0.0/24@3") whose
	// per-source verdicts live in Result.Matrices[probe]. Empty when
	// Approved is false.
	Column string `json:"column,omitempty"`
}

type ServiceReport struct {
	// Paths is one entry per service, sorted by name.
	Paths []ServicePath `json:"paths"`
	// Warnings collects services with a bad VIP or no backend device.
	Warnings []string `json:"warnings,omitempty"`
}

// ServicePath is one Tailscale Service and how it was evaluated.
type ServicePath struct {
	Name     string   `json:"name"`               // "svc:web"
	VIP      string   `json:"vip"`                // "100.96.0.1"
	Backends []uint64 `json:"backends,omitempty"` // advertising device IDs
	// Column is the matrix destination key ("svc:web") whose per-source
	// verdicts live in Result.Matrices[probe]. Empty when the service has no
	// backend device to probe against.
	Column string `json:"column,omitempty"`
}

type ExternalShareReport struct {
	// Tailnets is one entry per external tailnet the snapshot defines, sorted
	// by alias.
	Tailnets []ExternalTailnetView `json:"tailnets"`
	// Warnings collects notes on the shared-node evaluation, e.g. a tailnet
	// whose nodes can't initiate connections because it doesn't set
	// allowIncomingConnections. Policy problems (a misplaced or malformed
	// reference, an unknown alias, a grant src blocked by direction) are
	// fatal diagnostics instead, since Tailscale rejects those policies.
	Warnings []string `json:"warnings,omitempty"`
}

// ExternalTailnetView is one external tailnet and its shared nodes.
type ExternalTailnetView struct {
	Alias                     string   `json:"alias"`
	ExternalID                string   `json:"externalId,omitempty"`
	AllowIncomingConnections  bool     `json:"allowIncomingConnections"`
	AllowExternalReferencesTo []string `json:"allowExternalReferencesTo,omitempty"`
	// Configured is true when the policy's `externalTailnets` section has an
	// entry for this alias (false ⇒ snapshot-only, no policy config).
	Configured bool `json:"configured"`
	// Referenceable is the local device IDs this tailnet's own policy may name
	// as a src (devices carrying a tag, or owned by a user in a group, that
	// AllowExternalReferencesTo lists). Whether any of them actually reaches a
	// shared node is decided by that other policy, which isn't visible here.
	// Always non-nil.
	Referenceable []uint64 `json:"referenceable"`
	// Nodes is one entry per shared node.
	Nodes []ExternalNodeView `json:"nodes"`
}

// ExternalNodeView is one shared node and how it was evaluated.
type ExternalNodeView struct {
	Name   string   `json:"name"`
	Tags   []string `json:"tags,omitempty"`
	Groups []string `json:"groups,omitempty"`
	IPs    []string `json:"ips"`
	// Column is the matrix key ("ext:corp/relay") for this shared node. When
	// the tailnet allows incoming connections, shared node → local device
	// verdicts live at Matrices[probe]["ext:corp/relay-><deviceID>"]. There is
	// no local device → shared node verdict: the other tailnet's policy decides
	// that (see ExternalTailnetView.Referenceable).
	Column string `json:"column"`
	// Inbound is true when this node's tailnet allows incoming connections, so
	// the reverse-direction matrix keys are populated.
	Inbound bool `json:"inbound"`
}

// FilterRuleView is a JSON-friendly projection of tailscale.com/tailcfg.FilterRule
// — enough for the device inspector to show what each node enforces, without
// leaking the tailcfg type into this package.
type FilterRuleView struct {
	SrcIPs   []string `json:"srcIPs"`
	DstPorts []string `json:"dstPorts,omitempty"` // "prefix:portRange"
	IPProto  []int    `json:"ipProto,omitempty"`
	CapGrant []string `json:"capGrant,omitempty"` // "dst => cap[,cap]"
}

// Actor is one row/column of the matrix — the typed device directory the UI
// renders from instead of re-deriving identity itself.
type Actor struct {
	ID   uint64   `json:"id"`
	Name string   `json:"name"`
	Kind string   `json:"kind"` // "user" | "tagged"
	User string   `json:"user,omitempty"`
	Tags []string `json:"tags,omitempty"`
	IPs  []string `json:"ips"`
}

// Diagnostic is one problem or note about the policy/snapshot.
type Diagnostic struct {
	Severity string `json:"severity"` // "error" | "warning" | "info"
	// Code says what the user does about it: "parse" / "policy" — fix the
	// policy; "snapshot" — fix the mock tailnet; "limit" — the simulation can't
	// answer this (the policy may be fine); "internal" — a Tailground bug.
	// Faithful translations (autogroup expansion, svc: → VIP, …) are silent.
	Code    string `json:"code"`
	Pointer string `json:"pointer,omitempty"` // "line:col" into the policy text
	// Target names the snapshot item a diagnostic is about, so the UI can show
	// it there: "device:<name>", "services" or "external". Every diagnostic has
	// a Pointer, a Target, or (rarely — an unlocatable compile error or a
	// Tailground bug) neither.
	Target  string `json:"target,omitempty"`
	Message string `json:"message"`
}

// EngineInfo records which engine build produced a result.
type EngineInfo struct {
	HeadscaleCommit  string `json:"headscaleCommit"`
	TailscaleVersion string `json:"tailscaleVersion"`
	// Capabilities is the feature manifest the About modal renders. Omitted
	// when empty so results built outside the adapter stay minimal.
	Capabilities []Capability `json:"capabilities,omitempty"`
}

// Verdict values used in Result.Matrices cells.
const (
	VerdictAllow = "allow"
	VerdictDeny  = "deny"
)

// SSH verdict values used in Result.SSH cells.
const (
	SSHAccept = "accept"
	SSHCheck  = "check"
	SSHReject = "reject"
	SSHDeny   = "deny"
)

// DefaultSSHUsers is the login-name probe set applied when a Request declares
// none: root plus a common non-root name, so the autogroup:nonroot distinction
// is visible.
var DefaultSSHUsers = []string{"root", "ubuntu"}

// DefaultProbes is the probe set applied when a Request declares none: one per
// protocol family so a fresh tailnet exercises TCP, UDP and ICMP out of the box.
func DefaultProbes() []Probe {
	return []Probe{
		{Name: "tcp", Proto: "tcp", Port: 22},
		{Name: "https", Proto: "tcp", Port: 443},
		{Name: "udp", Proto: "udp", Port: 53},
		{Name: "icmp", Proto: "icmp"},
	}
}
