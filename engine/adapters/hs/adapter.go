// Package hs implements eval.Evaluator on top of Headscale's policy compiler
// (github.com/juanfont/headscale/hscontrol/policy) and Tailscale's packet-filter
// matcher (tailscale.com/wgengine/filter).
//
// Pipeline: preflight (strip un-modeled features) → NewPolicyManager → per
// destination FilterForNode → filter.Check for the reachability matrix,
// filter.CapsWithValues for app grants, NodeCapMap for node attributes.
package hs

import (
	hspolicy "github.com/juanfont/headscale/hscontrol/policy"
	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/rs/zerolog"
	"go4.org/netipx"
	"tailscale.com/types/views"

	"github.com/frozenprocess/tailground/engine/eval"
	"github.com/frozenprocess/tailground/engine/preflight"
)

// Build metadata. Overridable via -ldflags "-X ...".
var (
	HeadscaleCommit  = "63123196cc21eeb301eec60bf05d5fe0a2bc4787"
	TailscaleVersion = "v1.103.0-pre"
)

// Headscale's policy compiler logs to the global zerolog logger on every
// NewPolicyManager call. In a playground that re-evaluates on every keystroke
// those lines are pure noise, so the adapter silences the global logger on
// import.
func init() {
	zerolog.SetGlobalLevel(zerolog.Disabled)
}

// Evaluator is the Headscale-backed eval.Evaluator.
type Evaluator struct{}

// New returns a ready Evaluator.
func New() *Evaluator { return &Evaluator{} }

var _ eval.Evaluator = (*Evaluator)(nil)

// Evaluate compiles req.Policy against req.Snapshot and returns the reachability
// matrix (plus app grants, node caps and the compiled rules) for each probe.
func (e *Evaluator) Evaluate(req eval.Request) eval.Result {
	res := e.evaluate(req)
	res.Diagnostics = collapseDeviceDiags(res.Diagnostics)
	locateUnhomed(req.Policy, res.Diagnostics)
	return res
}

// evalCtx is what the eval* stages share: the compiled policy (and, with a
// posture layer, the base policy minus its gated rules), the mock tailnet, the
// probes to run, and the result being filled.
type evalCtx struct {
	pm, basePM hspolicy.PolicyManager
	layer      *postureLayer // nil without postures
	nodes      []*types.Node
	devices    []eval.Device
	devByID    map[uint64]eval.Device
	probes     []eval.Probe
	allIPs     *netipx.IPSet
	hasPosture bool
	res        *eval.Result
}

func (e *Evaluator) evaluate(req eval.Request) eval.Result {
	res := eval.Result{
		Matrices: map[string]map[string]string{},
		Engine:   eval.EngineInfo{HeadscaleCommit: HeadscaleCommit, TailscaleVersion: TailscaleVersion, Capabilities: eval.Capabilities},
	}

	users, nodes, actors, diags := buildTailnet(req.Snapshot)
	res.Actors = actors
	res.Diagnostics = append(res.Diagnostics, diags...)

	pf := preflight.RunWithOptions(req.Policy, preflight.Options{
		Users:            req.Snapshot.Users,
		Services:         req.Snapshot.Services,
		ExternalTailnets: req.Snapshot.ExternalTailnets,
	})
	res.Diagnostics = append(res.Diagnostics, pf.Diagnostics...)
	if pf.Fatal {
		return res
	}

	extPeers := buildExternalNodes(pf.ExternalShare)
	extNodes := make([]*types.Node, len(extPeers))
	for i, p := range extPeers {
		extNodes[i] = p.node
	}

	allNodes := append(append([]*types.Node(nil), nodes...), extNodes...)
	nodeViews := make([]types.NodeView, len(allNodes))
	for i, n := range allNodes {
		nodeViews[i] = n.View()
	}
	nv := views.SliceOf(nodeViews)

	pm, err := hspolicy.NewPolicyManager([]byte(pf.Policy), users, nv)
	if err != nil {
		d := diag("error", "parse", "%s", cleanErr(err))
		d.Pointer = preflight.LocateCompileError(req.Policy, err.Error())
		res.Diagnostics = append(res.Diagnostics, d)
		return res
	}

	warnUndefinedTags(pm, nodes, &res)
	evalNodeCaps(pm, nodes, pf.AppNodeCaps, &res)

	allIPs := unionPrefixes(allNodes)

	probes := req.Probes
	if len(probes) == 0 {
		probes = eval.DefaultProbes()
	}

	basePM, layer := compilePostureLayer(pm, pf, users, nv, &res)

	devByID := make(map[uint64]eval.Device, len(req.Snapshot.Devices))
	for _, d := range req.Snapshot.Devices {
		devByID[d.ID] = d
	}

	c := &evalCtx{
		pm: pm, basePM: basePM, layer: layer,
		nodes: nodes, devices: req.Snapshot.Devices, devByID: devByID,
		probes: probes, allIPs: allIPs, hasPosture: pf.Posture != nil,
		res: &res,
	}
	c.deviceMatrix()
	c.routes()
	c.services(req.Snapshot.Services)
	c.externalShare(extPeers, pf.ExternalShare)
	c.ssh(req.SSHUsers)
	evalTests(pm, pf.Policy, &res)

	return res
}

// warnUndefinedTags warns when a tag a device carries isn't defined in the
// policy at all (a likely typo). We deliberately don't check tag *ownership*
// (NodeCanHaveTag) — that's a control-plane authz concern, and Tailground's
// snapshot model doesn't carry a registering user for tagged devices.
func warnUndefinedTags(pm hspolicy.PolicyManager, nodes []*types.Node, res *eval.Result) {
	for _, n := range nodes {
		for _, tag := range n.Tags {
			if !pm.TagExists(tag) {
				res.Diagnostics = append(res.Diagnostics, deviceWarn(n.Hostname, "device %q carries tag %q, which no tagOwners entry defines", n.Hostname, tag))
			}
		}
	}
}

// evalNodeCaps fills res.NodeCaps (nodeAttrs). `nodeAttrs[].app` caps that
// preflight lifted out are folded back on via the AppTargetCapPrefix marker.
func evalNodeCaps(pm hspolicy.PolicyManager, nodes []*types.Node, appCaps map[int]map[string][]string, res *eval.Result) {
	for _, n := range nodes {
		caps := foldAppNodeCaps(nodeCapStrings(pm.NodeCapMap(n.ID)), appCaps)
		if len(caps) > 0 {
			put(&res.NodeCaps, uint64(n.ID), caps)
		}
	}
}

// compilePostureLayer builds the device-posture layer (Tailground's own).
func compilePostureLayer(pm hspolicy.PolicyManager, pf preflight.Result, users types.Users, nv views.Slice[types.NodeView], res *eval.Result) (hspolicy.PolicyManager, *postureLayer) {
	if pf.Posture == nil {
		return pm, nil
	}
	basePolicy, l, pdiags := buildPostureLayer(pf, users, nv)
	res.Diagnostics = append(res.Diagnostics, pdiags...)
	bpm, err := hspolicy.NewPolicyManager(basePolicy, users, nv)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diag("warning", "internal", "%s", "posture layer disabled — base policy failed to compile: "+cleanErr(err)))
		return pm, nil
	}
	res.Posture = l.report
	return bpm, l
}

// deviceMatrix fills the device↔device cells of res.Matrices, plus
// res.Rules and res.Grants, from each destination's compiled filter. With a
// posture layer every pair goes through it; otherwise the base filter decides.
func (c *evalCtx) deviceMatrix() {
	for _, dst := range c.nodes {
		baseF, baseRules, err := nodeFilter(c.basePM, dst, c.allIPs)
		if err != nil {
			c.res.Diagnostics = append(c.res.Diagnostics, diag("error", "internal", "compiling filter for %s: %v", dst.Hostname, err))
			continue
		}
		// The inspector shows the full compiled filter (posture-gated rules
		// included, with srcPosture stripped) so nothing silently disappears;
		// which rules are posture-gated is in res.Posture.
		ruleSrc := baseRules
		if c.layer != nil {
			if fr, ferr := c.pm.FilterForNode(dst.View()); ferr == nil {
				ruleSrc = fr
			}
		}
		if v := filterRuleViews(ruleSrc); len(v) > 0 {
			put(&c.res.Rules, uint64(dst.ID), v)
		}

		for _, src := range c.nodes {
			if src.ID == dst.ID {
				continue
			}
			key := eval.PairKey(src.ID, dst.ID)

			if c.layer != nil {
				pr := c.layer.evalPair(baseF, src, dst, postureAttrs(c.devByID[uint64(src.ID)]), c.probes, c.allIPs, &c.res.Diagnostics)
				for name, v := range pr.verdicts {
					matrixFor(c.res, name)[key] = v
				}
				if len(pr.caps) > 0 {
					put(&c.res.Grants, key, pr.caps)
				}
				if pr.denialReason != "" {
					put(&c.layer.report.Denials, key, pr.denialReason)
				}
				continue
			}

			for _, probe := range c.probes {
				matrixFor(c.res, probe.Name)[key] = verdict(baseF, src, dst, probe)
			}

			if caps := capNames(baseF, src, dst); len(caps) > 0 {
				put(&c.res.Grants, key, caps)
			}
		}
	}
}

// put sets (*m)[k] = v, allocating the map on first use so a result field
// nothing was written to stays nil.
func put[K comparable, V any](m *map[K]V, k K, v V) {
	if *m == nil {
		*m = map[K]V{}
	}
	(*m)[k] = v
}

func matrixFor(res *eval.Result, probe string) map[string]string {
	m := res.Matrices[probe]
	if m == nil {
		m = map[string]string{}
		res.Matrices[probe] = m
	}
	return m
}
