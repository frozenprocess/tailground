//go:build e2e

package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/frozenprocess/tailground/engine/eval"
	"github.com/frozenprocess/tailground/engine/route"
	"github.com/frozenprocess/tailground/engine/service"
)

// A case is an eval.Request plus assertions

type Case struct {
	Slug       string
	Dir        string
	Request    eval.Request
	Assertions []Assertion
	Meta       CaseMeta
	Skips      []SkipEntry
}

// CaseMeta is meta.json.
type CaseMeta struct {
	Title    string   `json:"title"`
	Needs    []string `json:"needs,omitempty"` // "ssh" | "ssh-advanced" | "posture-custom" | "internet"
	MinNodes int      `json:"minNodes,omitempty"`
	Note     string   `json:"note,omitempty"`
}

// Assertion is one row of assertions.json: an expected verdict for a flow.
type Assertion struct {
	From   string `json:"from"`            // source device name
	To     string `json:"to"`              // destination device name
	Proto  string `json:"proto,omitempty"` // "tcp" | "udp"
	Port   int    `json:"port,omitempty"`
	Expect string `json:"expect"` // "allow" | "deny"

	ToRoute string `json:"toRoute,omitempty"`

	ToService string `json:"toService,omitempty"`

	SSH  bool   `json:"ssh,omitempty"`
	User string `json:"user,omitempty"`
}

func (a Assertion) key() string {
	if a.SSH {
		return fmt.Sprintf("ssh:%s %s->%s", a.User, a.From, a.To)
	}
	if a.ToRoute != "" {
		return fmt.Sprintf("%s/%d %s->%s(%s)", a.Proto, a.Port, a.From, a.To, a.ToRoute)
	}
	if a.ToService != "" {
		return fmt.Sprintf("%s/%d %s->%s", a.Proto, a.Port, a.From, a.ToService)
	}
	return fmt.Sprintf("%s/%d %s->%s", a.Proto, a.Port, a.From, a.To)
}

// SkipEntry marks a flow the environment can't reproduce, with a reason
// It is matched against an assertion by from/to/proto/port.
type SkipEntry struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Proto  string `json:"proto,omitempty"`
	Port   int    `json:"port,omitempty"`
	Reason string `json:"reason"`
}

func (c Case) skipReason(a Assertion) (string, bool) {
	for _, s := range c.Skips {
		if s.From == a.From && s.To == a.To &&
			(s.Proto == "" || s.Proto == a.Proto) &&
			(s.Port == 0 || s.Port == a.Port) {
			return s.Reason, true
		}
	}
	return "", false
}

// DiscoverCases loads every cases/<slug>/ directory under root, optionally
// filtered by a glob against the slug (E2E_CASE).
func DiscoverCases(root, glob string) ([]Case, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read cases dir %s: %w", root, err)
	}
	var cases []Case
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if glob != "" {
			if ok, _ := filepath.Match(glob, e.Name()); !ok {
				continue
			}
		}
		c, err := loadCase(filepath.Join(root, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("case %s: %w", e.Name(), err)
		}
		cases = append(cases, c)
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].Slug < cases[j].Slug })
	return cases, nil
}

func loadCase(dir string) (Case, error) {
	c := Case{Slug: filepath.Base(dir), Dir: dir}
	if err := readJSON(filepath.Join(dir, "case.json"), &c.Request); err != nil {
		return c, err
	}
	if err := readJSON(filepath.Join(dir, "assertions.json"), &c.Assertions); err != nil {
		return c, err
	}
	if err := readJSON(filepath.Join(dir, "meta.json"), &c.Meta); err != nil {
		return c, err
	}
	if _, err := os.Stat(filepath.Join(dir, "skip.json")); err == nil {
		if err := readJSON(filepath.Join(dir, "skip.json"), &c.Skips); err != nil {
			return c, err
		}
	}
	return c, c.validate()
}

// validate enforces the constraints an API-only tailnet imposes.
func (c Case) validate() error {
	if len(c.Request.Snapshot.Devices) == 0 {
		return fmt.Errorf("no devices")
	}
	names := map[string]bool{}
	for _, d := range c.Request.Snapshot.Devices {
		if d.Name == "" {
			return fmt.Errorf("device id %d has no name", d.ID)
		}
		names[d.Name] = true
	}
	for _, a := range c.Assertions {
		if !names[a.From] {
			return fmt.Errorf("assertion references unknown device %q", a.From)
		}
		if a.ToService == "" && !names[a.To] {
			return fmt.Errorf("assertion references unknown device %q", a.To)
		}
		if a.SSH {
			switch SSHVerdict(a.Expect) {
			case SSHAccept, SSHCheck, SSHReject, SSHDeny:
			default:
				return fmt.Errorf("assertion %s: ssh expect must be accept|check|reject|deny, got %q", a.key(), a.Expect)
			}
			if a.User == "" {
				return fmt.Errorf("assertion %s: ssh assertion needs a user", a.key())
			}
		} else if a.Expect != eval.VerdictAllow && a.Expect != eval.VerdictDeny {
			return fmt.Errorf("assertion %s: expect must be allow|deny, got %q", a.key(), a.Expect)
		}
	}
	return nil
}

func (c Case) SkipReason(cfg Config) string {
	for _, d := range c.Request.Snapshot.Devices {
		if len(d.Tags) == 0 {
			return fmt.Sprintf("device %q has no tags; API-only tailnets are tag-centric (C1)", d.Name)
		}
	}
	if n := c.Meta.MinNodes; n > 0 && len(c.Request.Snapshot.Devices) < n {
		return fmt.Sprintf("minNodes %d > %d devices", n, len(c.Request.Snapshot.Devices))
	}
	for _, need := range c.Meta.Needs {
		switch need {
		case "posture-custom", "ssh-advanced":
			if cfg.Premium {
				return "" // E2E_PREMIUM=1 — run it and let it fail if the tier rejects
			}
			return "needs " + need + " (tier-gated; E2E_PREMIUM=0)"
		case "ssh-check":
			return "ssh `check` needs a user-scoped src; unreachable on API-only tailnets (C1)"
		case "subnet-route":
			// The subnet-router differential binds an in-subnet address on `lo`
			// and needs a real routing stack — kernel-TUN docker only.
			if cfg.Backend == "docker-userspace" || cfg.Backend == "tsnet" {
				return "subnet-route differential needs the kernel-TUN docker backend"
			}
			// runnable on the default docker backend — keep checking other needs
		case "services":
			// This was raised by a Discord user
			return "service (`svc:`) live differential needs Tailscale Service provisioning"
		case "exit-node":
			// The engine models exit-node reachability (golden exit-node-basic),
			// but the live probe would need `--exit-node` set on the source,
			// which reroutes all its traffic; the harness doesn't do that yet.
			return "exit-node live differential needs `--exit-node` on the source"
		default:
			// If something is not implemented yet let the user know.
			return "needs " + need + " (not covered yet!)"
		}
	}
	return ""
}

func (c Case) EngineCantModel() string {
	return ""
}

// tagSetKey is the key of one distinct tag set: the tags sorted and joined, so
// devices carrying the same tags in any order share one auth key.
func tagSetKey(tags []string) string {
	sorted := append([]string(nil), tags...)
	sort.Strings(sorted)
	return strings.Join(sorted, ",")
}

// provision readies the tailnet for a fresh set of nodes: it pushes policy's
// provisioning policy (its tags + allow-all, so the child OAuth client can mint
// tagged keys and the nodes converge), then purges any devices left over.
func provision(ctx context.Context, run *Run, policy string) error {
	provPol, err := ProvisioningPolicy(policy)
	if err != nil {
		return fmt.Errorf("provisioning policy: %w", err)
	}
	if err := run.TS().SetACLRaw(ctx, provPol); err != nil {
		return fmt.Errorf("push provisioning policy: %w", err)
	}
	if _, err := run.TS().PurgeDevices(ctx); err != nil {
		return fmt.Errorf("purge devices: %w", err)
	}
	return nil
}

// mintTagSetKeys mints one ephemeral tagged auth key per distinct tag set
// across devices, keyed by tagSetKey.
func mintTagSetKeys(ctx context.Context, run *Run, devices []eval.Device) (map[string]string, error) {
	names := map[string][]string{} // tag-set key → devices carrying it
	tags := map[string][]string{}
	for _, d := range devices {
		k := tagSetKey(d.Tags)
		names[k] = append(names[k], d.Name)
		if _, ok := tags[k]; !ok {
			tags[k] = append([]string(nil), d.Tags...)
			sort.Strings(tags[k])
		}
	}
	keys := make(map[string]string, len(tags))
	for k, ts := range tags {
		key, err := run.TS().MintAuthKey(ctx, ts)
		if err != nil {
			return nil, fmt.Errorf("mint key for %v (devices %v): %w", ts, names[k], err)
		}
		keys[k] = key
	}
	return keys, nil
}

// planNodes turns a case's devices into node specs and the subnet routes to
// approve per host.
//
// SSH assertions: the destination node needs `--ssh`, and every non-root login
// name needs a real local user there; the source needs the system ssh client.
// Subnet routers: which routes each device advertises, the subset to approve via
// the API, and the in-subnet addresses it should answer on.
func planNodes(c Case, keyByTagSet map[string]string) (specs []NodeSpec, approveByHost map[string][]string) {
	sshDst := map[string][]string{} // dst device name → distinct non-root users
	sshSrc := map[string]bool{}     // src device name → needs the system ssh client
	for _, a := range c.Assertions {
		if !a.SSH {
			continue
		}
		sshSrc[a.From] = true
		if a.User != "" && a.User != "root" && !contains(sshDst[a.To], a.User) {
			sshDst[a.To] = append(sshDst[a.To], a.User)
		} else if _, seen := sshDst[a.To]; !seen {
			sshDst[a.To] = nil
		}
	}

	anyRouter := false
	for _, d := range c.Request.Snapshot.Devices {
		anyRouter = anyRouter || len(d.Routes) > 0
	}

	approveByHost = map[string][]string{}
	specs = make([]NodeSpec, 0, len(c.Request.Snapshot.Devices))
	for _, d := range c.Request.Snapshot.Devices {
		_, isSSHDst := sshDst[d.Name]
		spec := NodeSpec{
			Name: d.Name, Tags: d.Tags, AuthKey: keyByTagSet[tagSetKey(d.Tags)],
			SSH: isSSHDst, SSHUsers: sshDst[d.Name], SSHClient: sshSrc[d.Name],
			AdvertiseRoutes: d.Routes,
			AcceptRoutes:    anyRouter && len(d.Routes) == 0,
		}
		for _, s := range d.RoutesApproved {
			p, err := netip.ParsePrefix(s)
			if err != nil {
				continue
			}
			approveByHost[sanitize(d.Name)] = append(approveByHost[sanitize(d.Name)], s)
			if !route.IsExit(p.Masked()) {
				spec.RouteListen = append(spec.RouteListen, route.SampleIP(p.Masked()))
			}
		}
		specs = append(specs, spec)
	}
	return specs, approveByHost
}

// nodeAddrs renders "name=ip" for each launched node.
func nodeAddrs(nodes []*Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = fmt.Sprintf("%s=%s", n.Name, n.IPv4)
	}
	return out
}

// verdictOf is the tailnet-side verdict a probe's reached/not-reached means.
func verdictOf(reached bool) string {
	if reached {
		return eval.VerdictAllow
	}
	return eval.VerdictDeny
}

func pushPolicy(ctx context.Context, run *Run, policy string, logf func(string, ...any)) (string, error) {
	err := run.TS().SetACLRaw(ctx, policy)
	if err == nil {
		return policy, nil
	}
	stripped, removed, serr := stripTopLevel(policy, "attrConfig")
	if serr != nil || !removed || !strings.Contains(err.Error(), "attrConfig") {
		return "", fmt.Errorf("push policy: %v", err)
	}
	logf("  attrConfig rejected by this tier — retrying without it (custom values are on the devices)")
	if err := run.TS().SetACLRaw(ctx, stripped); err != nil {
		return "", fmt.Errorf("push policy (attrConfig stripped): %v", err)
	}
	return stripped, nil
}

// RunCase executes one case against the provisioned tailnet: push the policy,
// launch a node per device, wait for the filter to land, then probe every
// assertion and diff engine vs tailnet
func RunCase(ctx context.Context, cfg Config, run *Run, c Case, logf func(string, ...any)) CaseReport {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	rep := CaseReport{Slug: c.Slug, Title: c.Meta.Title}
	fail := func(format string, args ...any) CaseReport {
		rep.SetupErr = fmt.Sprintf(format, args...)
		return rep
	}

	// 1. Provisioning policy + a clean device list.
	if err := provision(ctx, run, c.Request.Policy); err != nil {
		return fail("%v", err)
	}

	// 2. One ephemeral tagged key per distinct tag set.
	backend, err := newBackend(ctx, cfg, run.RunID+"-"+c.Slug)
	if err != nil {
		return fail("node backend: %v", err)
	}
	var nodes []*Node
	keptContainers := false
	defer func() {
		// When E2E_ARTIFACTS is set, dump the bundle for every case — a passing
		// case's stored-acl.hujson / netmaps are useful fixtures, not just a
		// failure's.
		if cfg.Artifacts != "" && len(nodes) > 0 {
			DumpArtifacts(context.Background(), cfg.Artifacts, backend, nodes, rep)
		}
		if !keptContainers {
			_ = backend.Stop(context.Background())
		}
	}()

	logf("provisioning policy pushed, devices purged — minting keys [%s backend]", backendName(cfg))
	keyByTagSet, err := mintTagSetKeys(ctx, run, c.Request.Snapshot.Devices)
	if err != nil {
		return fail("%v", err)
	}

	specs, approveByHost := planNodes(c, keyByTagSet)

	// 3. Launch + converge. Each node's echo server listens on every port an
	//    assertion probes.
	logf("launching %d node(s) and waiting for the tailnet to converge…", len(specs))
	nodes, err = backend.Launch(ctx, specs, servePorts(c.Assertions), func(f string, a ...any) { logf("  "+f, a...) })
	if err != nil {
		if cfg.Keep {
			keptContainers = true
		}
		return fail("launch nodes: %v", err)
	}
	rep.Nodes = nodeAddrs(nodes)
	logf("nodes up: %s", strings.Join(rep.Nodes, " "))

	// 3b. Approve advertised subnet routes (the admin action a real operator
	//     takes), then let them propagate into every node's netmap.
	if len(approveByHost) > 0 {
		logf("approving subnet routes: %v", approveByHost)
		if err := run.TS().ApproveRoutes(ctx, approveByHost); err != nil {
			return fail("approve routes: %v", err)
		}
		waitRoutesLanded(ctx, backend, nodes, approveByHost, logf)
	}

	// 3c. Write the snapshot's custom:* device-posture attributes to the real
	//     devices (node:* attrs Tailscale populates itself). srcPosture on the
	//     case policy is evaluated against these when the filter is compiled.
	if attrs, err := customPostureAttrs(c.Request.Policy, c.Request.Snapshot.Devices); err != nil {
		return fail("custom posture attrs: %v", err)
	} else if len(attrs) > 0 {
		logf("writing custom posture attributes: %v", attrs)
		if err := run.TS().SetPostureAttrs(ctx, attrs); err != nil {
			return fail("set posture attributes: %v", err)
		}
	}

	// 4. Rewrite the case to the real addresses — the SAME bytes feed both sides.
	//    Probes are regenerated from the assertions with canonical "proto/port"
	//    names so the engine matrix lines up with what we probe.
	req := c.Request
	req.Probes = probesFromAssertions(c.Assertions)
	req.SSHUsers = sshLoginUsers(c.Assertions)
	rw := NewRewriter(req, nodes)
	realReq := rw.Apply(req)
	nodeByName := map[string]*Node{}
	for _, n := range nodes {
		nodeByName[n.Name] = n
	}

	// 5. Capture the pre-push filter digest, push the policy, wait for it to land.
	logf("pushing the case policy, waiting for the filter to land…")
	pre := digestFilters(ctx, backend, nodes)
	livePol, err := pushPolicy(ctx, run, realReq.Policy, logf)
	if err != nil {
		return fail("%v", err)
	}
	rep.FilterLanded = waitFilterLanded(ctx, backend, nodes, pre)
	logf("filter-landed=%v — probing %d assertion(s)", rep.FilterLanded, len(c.Assertions))

	// Read the policy back — Tailscale round-trips the pushed HuJSON verbatim, so
	// this is a fixture of what the coordination server actually stored (e.g. a
	// bare `{}` or tagOwners-only policy stays as-is and behaves allow-all). Only
	// needed for the E2E_ARTIFACTS bundle. Best-effort: a read failure never
	// fails the case.
	if cfg.Artifacts != "" {
		if stored, err := run.TS().GetACLRaw(ctx); err != nil {
			logf("  (could not read back the stored ACL: %v)", err)
		} else {
			rep.storedPolicy = stored
		}
	}

	// 6. Engine prediction over the rewritten snapshot.
	res, enginePol, err := EvaluateForEngine(realReq)
	if err != nil {
		return fail("engine: %v", err)
	}
	rep.pushedPolicy = livePol
	rep.enginePolicy = enginePol
	rep.engineResult = res
	byID := actorIndex(res)

	// 7. Probe every assertion, diff. Each probe logs its own line — an SSH probe
	//    with retries can take the better part of a minute.
	pr := &caseProber{ctx: ctx, cfg: cfg, c: c, backend: backend, rw: rw, res: res, byID: byID, nodeByName: nodeByName}
	for i, a := range c.Assertions {
		logf("  [%d/%d] %s …", i+1, len(c.Assertions), a.key())
		row := pr.probe(a)
		rep.Rows = append(rep.Rows, row)
		logf("  [%d/%d] %s → engine=%s tailnet=%s  %s%s", i+1, len(c.Assertions), a.key(),
			dash(row.Engine), dash(row.Tailnet), row.Status, detailSuffix(row.Detail))
	}

	if rep.Failed() && cfg.Keep {
		keptContainers = true
	}
	return rep
}

// caseProber probes one case's assertions against the live nodes and diffs each
// against the engine's prediction.
type caseProber struct {
	ctx        context.Context
	cfg        Config
	c          Case
	backend    NodeBackend
	rw         *Rewriter
	res        eval.Result
	byID       map[string]uint64
	nodeByName map[string]*Node
}

func (p *caseProber) probe(a Assertion) Row {
	if reason, ok := p.c.skipReason(a); ok {
		return Row{Assertion: a, Status: RowSkipped, Detail: reason}
	}
	if a.SSH {
		return p.probeSSH(a)
	}
	if a.Proto != "tcp" && a.Proto != "udp" {
		return Row{Assertion: a, Status: RowSkipped, Detail: a.Proto + " probing not implemented (only tcp/udp is probed)"}
	}
	return p.probeL4(a)
}

func (p *caseProber) probeSSH(a Assertion) Row {
	dst, ok := p.rw.RealAddr(a.To)
	if !ok {
		return Row{Assertion: a, Status: RowError, Detail: "no address for " + a.To}
	}
	engineV := engineSSH(p.res, a.User, p.byID[a.From], p.byID[a.To])
	tailnetV, detail, perr := p.backend.SSHProbe(p.ctx, p.nodeByName[a.From], a.User, dst)
	if errors.Is(perr, errSSHUnsupported) {
		return Row{Assertion: a, Status: RowSkipped,
			Detail: "ssh differential not supported by the " + p.cfg.Backend + " backend"}
	}
	if perr != nil {
		return Row{Assertion: a, Status: RowError, Detail: perr.Error()}
	}
	row := classify(a, string(engineV), string(tailnetV))
	if row.Status == RowDivergence || row.Status == RowQuestionable {
		row.Detail = strings.TrimSpace(row.Detail + " | " + firstLine(detail))
	}
	return row
}

func (p *caseProber) probeL4(a Assertion) Row {
	var engineV string
	var dst netip.Addr
	if a.ToRoute != "" {
		pfx, err := netip.ParsePrefix(a.ToRoute)
		if err != nil {
			return Row{Assertion: a, Status: RowError, Detail: "bad toRoute: " + err.Error()}
		}
		engineV = engineRouteVerdict(p.res, probeName(a), p.byID[a.From], p.byID[a.To], pfx.Masked())
		dst = route.SampleIP(pfx.Masked())
	} else {
		engineV = verdict(p.res, probeName(a), p.byID[a.From], p.byID[a.To])
		var ok bool
		if dst, ok = p.rw.RealAddr(a.To); !ok {
			return Row{Assertion: a, Status: RowError, Detail: "no address for " + a.To}
		}
	}

	reached, perr := p.backend.Probe(p.ctx, p.nodeByName[a.From], dst, a.Proto, a.Port)
	if errors.Is(perr, errL4Unsupported) {
		return Row{Assertion: a, Status: RowSkipped,
			Detail: "l4 differential not supported by the " + p.cfg.Backend + " backend"}
	}
	if perr != nil {
		return Row{Assertion: a, Status: RowError, Detail: perr.Error()}
	}
	return classify(a, engineV, verdictOf(reached))
}

// probeName is the canonical matrix key for an assertion's (proto, port).
func probeName(a Assertion) string {
	return fmt.Sprintf("%s/%d", a.Proto, a.Port)
}

// actorIndex maps each actor name in a Result to its numeric ID (the matrix and
// SSH maps are keyed by "<srcID>-><dstID>").
func actorIndex(res eval.Result) map[string]uint64 {
	m := map[string]uint64{}
	for _, a := range res.Actors {
		m[a.Name] = a.ID
	}
	return m
}

// verdict reads the engine's L4 verdict for one (probe, src, dst).
func verdict(res eval.Result, probe string, src, dst uint64) string {
	mat, ok := res.Matrices[probe]
	if !ok {
		return "<no-probe>"
	}
	v, ok := mat[eval.PairKey(src, dst)]
	if !ok {
		return eval.VerdictDeny
	}
	return v
}

func engineRouteVerdict(res eval.Result, probe string, src, router uint64, prefix netip.Prefix) string {
	mat, ok := res.Matrices[probe]
	if !ok {
		return "<no-probe>"
	}
	v, ok := mat[eval.PairKey(src, route.Column(prefix, router))]
	if !ok {
		return "<no-route>"
	}
	return v
}

func engineServiceVerdict(res eval.Result, probe string, src uint64, svc string) string {
	mat, ok := res.Matrices[probe]
	if !ok {
		return "<no-probe>"
	}
	col := service.TokenPrefix + service.Normalize(svc)
	v, ok := mat[eval.PairKey(src, col)]
	if !ok {
		return "<no-service>"
	}
	return v
}

// probesFromAssertions builds one eval.Probe per distinct (proto, port) across
// the L4 assertions, named canonically so probeName resolves in the matrix.
func probesFromAssertions(as []Assertion) []eval.Probe {
	seen := map[string]bool{}
	var out []eval.Probe
	for _, a := range as {
		if a.SSH {
			continue
		}
		name := probeName(a)
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, eval.Probe{Name: name, Proto: a.Proto, Port: a.Port})
	}
	return out
}

// customPostureAttrs collects the custom:* device-posture values from the
// snapshot, keyed by the device's Tailscale hostname then attribute key, ready
// for tsClient.SetPostureAttrs. Values are coerced to the type declared in the
// policy's `attrConfig` (number → float64, bool → bool) so Tailscale's
// comparison matches the engine's; unknown/string stays a string. Returns nil
// when no device carries a custom attribute.
func customPostureAttrs(policy string, devices []eval.Device) (map[string]map[string]any, error) {
	types, err := attrConfigTypes(policy)
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]any{}
	for _, d := range devices {
		for k, v := range d.Posture {
			if !strings.HasPrefix(k, "custom:") {
				continue
			}
			host := sanitize(d.Name)
			if out[host] == nil {
				out[host] = map[string]any{}
			}
			out[host][k] = coercePostureValue(v, types[k])
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func coercePostureValue(raw, typ string) any {
	switch typ {
	case "number":
		if f, err := strconv.ParseFloat(raw, 64); err == nil {
			return f
		}
	case "bool":
		switch strings.ToLower(raw) {
		case "true", "1", "yes":
			return true
		case "false", "0", "no", "":
			return false
		}
	}
	return raw
}

// sshLoginUsers is the distinct set of login names across the SSH assertions,
// passed to the engine as req.SSHUsers.
func sshLoginUsers(as []Assertion) []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range as {
		if a.SSH && a.User != "" && !seen[a.User] {
			seen[a.User] = true
			out = append(out, a.User)
		}
	}
	return out
}

// engineSSH reads the engine's SSH verdict for one (user, src, dst).
func engineSSH(res eval.Result, user string, src, dst uint64) SSHVerdict {
	if res.SSH == nil {
		return SSHDeny
	}
	v, ok := res.SSH[eval.SSHKey(user, src, dst)]
	if !ok {
		return SSHDeny
	}
	return SSHVerdict(v)
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// backendName is cfg.Backend with the empty default spelled out, for logging.
func backendName(cfg Config) string {
	if cfg.Backend == "" {
		return "docker"
	}
	return cfg.Backend
}

// detailSuffix renders a Row.Detail as a trailing "  (…)" note, or "" if empty.
func detailSuffix(s string) string {
	if s == "" {
		return ""
	}
	return "  (" + firstLine(s) + ")"
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// servePorts is the distinct set of TCP/UDP ports any assertion probes.
func servePorts(as []Assertion) []int {
	seen := map[int]bool{}
	var out []int
	for _, a := range as {
		if a.SSH || (a.Proto != "tcp" && a.Proto != "udp") || a.Port == 0 {
			continue
		}
		if !seen[a.Port] {
			seen[a.Port] = true
			out = append(out, a.Port)
		}
	}
	return out
}

func digestFilters(ctx context.Context, b NodeBackend, nodes []*Node) map[string]string {
	out := map[string]string{}
	for _, n := range nodes {
		out[n.Name] = filterDigest(ctx, b, n)
	}
	return out
}

func waitFilterLanded(ctx context.Context, b NodeBackend, nodes []*Node, pre map[string]string) bool {
	if len(nodes) == 0 {
		return true
	}
	deadline := time.Now().Add(FilterSettleTimeout)
	var last map[string]string
	stableSince := time.Time{}
	for {
		cur := digestFilters(ctx, b, nodes)
		changed := 0
		for name, d := range cur {
			if d != pre[name] {
				changed++
			}
		}
		if mapsEqualStr(cur, last) {
			if stableSince.IsZero() {
				stableSince = time.Now()
			}
		} else {
			stableSince = time.Time{}
			last = cur
		}
		settled := !stableSince.IsZero() && time.Since(stableSince) >= 4*time.Second

		switch {
		case changed == len(nodes) && settled:
			return true
		case changed > 0 && settled:
			// The nodes whose filter was going to change have; the rest are
			// stale sources (see the note above). Good enough to probe.
			return true
		case time.Now().After(deadline):
			return changed > 0
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(2 * time.Second):
		}
	}
}

// waitRoutesLanded blocks until every approved (non-exit) subnet prefix appears
// in each non-router node's netmap, so a probe into the subnet isn't raced
// against route propagation. Best-effort: it returns on a timeout and lets the
// probe's own retry backoff finish the job.
func waitRoutesLanded(ctx context.Context, b NodeBackend, nodes []*Node, byHost map[string][]string, logf func(string, ...any)) {
	routerHost := map[string]bool{}
	var want []string
	for h, routes := range byHost {
		routerHost[h] = true
		for _, r := range routes {
			if p, err := netip.ParsePrefix(r); err == nil && !route.IsExit(p.Masked()) {
				want = append(want, p.Masked().String())
			}
		}
	}
	if len(want) == 0 {
		return
	}
	deadline := time.Now().Add(FilterSettleTimeout)
	for {
		missing := 0
		for _, n := range nodes {
			if routerHost[sanitize(n.Name)] {
				continue
			}
			raw, err := b.Netmap(ctx, n)
			if err != nil {
				missing++
				continue
			}
			s := string(raw)
			for _, w := range want {
				if !strings.Contains(s, w) {
					missing++
					break
				}
			}
		}
		if missing == 0 {
			logf("subnet routes propagated to every source")
			return
		}
		if time.Now().After(deadline) {
			logf("subnet routes not yet visible on %d source(s) — probing anyway", missing)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func mapsEqualStr(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func filterDigest(ctx context.Context, b NodeBackend, n *Node) string {
	raw, err := b.Netmap(ctx, n)
	if err != nil {
		return "err:" + err.Error()
	}
	var nm map[string]any
	if json.Unmarshal(raw, &nm) != nil {
		return fmt.Sprintf("%x", sha256.Sum256(raw))
	}
	// Prefer the packet-filter section; fall back to the whole netmap.
	for _, k := range []string{"packetFilter", "packetFilters", "PacketFilter", "PacketFilterRules"} {
		if v, ok := nm[k]; ok {
			b, _ := json.Marshal(v)
			return fmt.Sprintf("%x", sha256.Sum256(b))
		}
	}
	b2, _ := json.Marshal(nm)
	return fmt.Sprintf("%x", sha256.Sum256(b2))
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return nil
}
