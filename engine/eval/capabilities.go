package eval

// CapabilityGroup says who handles a feature, or why it is not handled. It
// replaces a separate "supported" flag: every group except the last three
// means "evaluated", and the last three each mean "not evaluated" for a
// distinct reason.
type CapabilityGroup string

const (
	// CapHeadscale: evaluated by the vendored Headscale policy engine.
	CapHeadscale CapabilityGroup = "headscale"
	// CapTailground: evaluated by Tailground's own layer on top of it.
	CapTailground CapabilityGroup = "tailground"
	// CapExpanded: rewritten into primitives Headscale understands before
	// compilation.
	CapExpanded CapabilityGroup = "expanded"
	// CapNotModeled: stripped by preflight; a gap, not a choice.
	CapNotModeled CapabilityGroup = "not-modeled"
	// CapByDesign: stripped by preflight because it cannot change a verdict.
	CapByDesign CapabilityGroup = "by-design"
)

// Capability is one row of the engine's capability manifest, rendered by the
// About modal. Name and Notes are plain text in which `backtick spans` mean
// inline code; the UI renders them, everything else is literal.
type Capability struct {
	// ID is stable and unique ("posture.srcPosture"); UIs use it as a key and
	// tests use it to point at fixtures.
	ID    string          `json:"id"`
	Group CapabilityGroup `json:"group"`
	// Name is the short label: what the feature is.
	Name string `json:"name"`
	// Notes is the caveat or mechanism, if any. Omitted when the name says it all.
	Notes string `json:"notes,omitempty"`
}

// Capabilities is the manifest the engine reports in every Result's
// EngineInfo. It is the single source for the About modal: add or change a
// feature here, not in the web app.
var Capabilities = []Capability{
	// --- Headscale engine ---
	{ID: "hs.acls", Group: CapHeadscale, Name: "`acls` (action: accept) and `grants` (ip, app, via)"},
	{ID: "hs.groups", Group: CapHeadscale, Name: "`groups`, `hosts`, `tagOwners`", Notes: "groups are non-recursive"},
	{ID: "hs.ssh", Group: CapHeadscale, Name: "`ssh` accept / check", Notes: "`checkPeriod`, `acceptEnv`, `localpart:*@domain` users"},
	{ID: "hs.nodeAttrs", Group: CapHeadscale, Name: "`nodeAttrs` (target + attr)"},
	{ID: "hs.autoApprovers", Group: CapHeadscale, Name: "`autoApprovers` (routes + exitNode)"},
	{ID: "hs.tests", Group: CapHeadscale, Name: "`tests` / `sshTests` self-checks"},
	{ID: "hs.autogroups", Group: CapHeadscale, Name: "autogroups", Notes: "`internet`, `member`, `tagged`, `self`, `nonroot`, `danger-all`"},

	// --- Tailground's own layer ---
	{ID: "tg.posture", Group: CapTailground, Name: "Device postures",
		Notes: "`postures` + `acls[]` / `grants[]` / `ssh[]` `srcPosture` + `defaultSrcPosture` (policy-wide baseline; a rule's own `srcPosture` replaces it)"},
	{ID: "tg.conditions", Group: CapTailground, Name: "Posture conditions",
		Notes: "`node:os`, `node:tsVersion` (numeric compare), `node:tsReleaseTrack`, `custom:<name>` (declared in `attrConfig` — type string / bool / number, only `type` is modeled; device values come from the snapshot)"},
	{ID: "tg.operators", Group: CapTailground, Name: "Condition operators", Notes: "`==` `!=` `>` `>=` `<` `<=`, `in [ … ]`, `&&` / `||`"},
	{ID: "tg.routes", Group: CapTailground, Name: "Subnet-route & exit-node reachability",
		Notes: "each source is checked against the router's own filter (where Headscale puts the CIDR-dst / `grants[].via` rule); shown in Routes & exit nodes"},
	{ID: "tg.services", Group: CapTailground, Name: "Tailscale Service (`svc:<name>`) reachability",
		Notes: "the dst is rewritten to the service VIP and each source is checked against every backend device's filter; shown in Tailscale Services"},
	{ID: "tg.sharing", Group: CapTailground, Name: "Declarative node sharing",
		Notes: "`externalTailnets` + `group://…` / `tag://…` references resolved against the snapshot's shared nodes; reachability both ways in Declarative node sharing (the wire syntax isn't public — this is Tailground's reading, pinned)"},

	// --- Expanded before compilation ---
	{ID: "exp.ipsets", Group: CapExpanded, Name: "`ipsets` / the `ipset:` alias",
		Notes: "resolved to CIDR prefixes (members: CIDR, IP, a-b range, `host:`, nested `ipset:`) and substituted into rule src/dst"},
	{ID: "exp.nodeAttrs.app", Group: CapExpanded, Name: "`nodeAttrs[].app`",
		Notes: "capabilities lifted out and re-folded onto the target devices' nodeCaps"},
	{ID: "exp.userWildcard", Group: CapExpanded, Name: "`user:*@domain` source wildcard",
		Notes: "expanded to the matching snapshot users' emails (Headscale has no domain wildcard)"},
	{ID: "exp.roleAutogroups", Group: CapExpanded, Name: "Admin-role & shared autogroups",
		Notes: "`autogroup:admin` / `owner` / `auditor` / `it-admin` / `network-admin` / `billing-admin` / `shared` are resolved against each snapshot user's role in rules, `tagOwners` and `autoApprovers`, then substituted as explicit emails"},
	{ID: "exp.svc", Group: CapExpanded, Name: "`svc:<name>` destinations",
		Notes: "rewritten to the Tailscale Service VIP from the snapshot (the Headscale engine has no Services)"},
	{ID: "exp.crossTailnet", Group: CapExpanded, Name: "`group://<tailnet>/<name>` / `tag://<tailnet>/<name>`",
		Notes: "rewritten to the addresses of the snapshot's shared nodes (the Headscale engine has no cross-tailnet concept); accepted and rejected exactly where Tailscale's policy API does (only in a grant's `src`, with `allowIncomingConnections`), checked case by case against the live API"},

	// --- Not modeled (gaps) ---
	{ID: "nm.autoApproval", Group: CapNotModeled, Name: "Route auto-approval",
		Notes: "reachability is modeled from `routesApproved`; whether a route would be auto-approved is not"},
	{ID: "nm.appConnector", Group: CapNotModeled, Name: "App-connector routing",
		Notes: "the `nodeAttrs[].app` capability is surfaced; the routing itself isn't simulated"},
	{ID: "nm.sshRecording", Group: CapNotModeled, Name: "SSH session recording",
		Notes: "`ssh[].recorder` / `enforceRecorder` are stripped; the rule still evaluates for reachability"},
	{ID: "nm.postureIntegrations", Group: CapNotModeled, Name: "Live device-posture integrations (`postureIntegrations`)",
		Notes: "a `custom:` attribute is evaluated only when the snapshot device sets a value; nothing is pulled from Intune / Jamf / CrowdStrike / …"},

	// --- Intentionally not modeled ---
	{ID: "bd.derpMap", Group: CapByDesign, Name: "`derpMap`", Notes: "relay configuration"},
	{ID: "bd.disableIPv4", Group: CapByDesign, Name: "`disableIPv4`", Notes: "address assignment"},
	{ID: "bd.oneCGNATRoute", Group: CapByDesign, Name: "`oneCGNATRoute`", Notes: "client route generation"},
	{ID: "bd.extraDNSRecords", Group: CapByDesign, Name: "`extraDNSRecords`", Notes: "DNS names"},
}
