package hs

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	hspolicy "github.com/juanfont/headscale/hscontrol/policy"
	"github.com/juanfont/headscale/hscontrol/types"
	"go4.org/netipx"
	"tailscale.com/tailcfg"
	"tailscale.com/types/views"
	"tailscale.com/wgengine/filter"

	"github.com/frozenprocess/tailground/engine/eval"
	"github.com/frozenprocess/tailground/engine/posture"
	"github.com/frozenprocess/tailground/engine/preflight"
)

// postureLayer holds the posture-gated rules lifted out of a policy, each
// compiled on its own so its contribution can be gated per source device.
type postureLayer struct {
	set   *posture.Set
	gated []*gatedRule
	// sshRules is the full ordered ssh[] list, one compiled sub-policy each,
	// populated only when at least one ssh rule carries srcPosture. SSH is
	// first-match-wins, so the whole list (gated and ungated) must be kept in
	// order — unlike acls/grants, which union.
	sshRules []*gatedSSHRule
	report   *eval.PostureReport
}

type gatedRule struct {
	pm     hspolicy.PolicyManager
	gate   posture.Gate
	ref    string // "acls[2]" | "grants[0]"
	fcache map[types.NodeID]*filter.Filter
}

// gatedSSHRule is one ssh[] rule compiled as its own sub-policy, plus the
// posture gate its srcPosture imposes (an empty gate for an ungated rule).
type gatedSSHRule struct {
	pm     hspolicy.PolicyManager
	gate   posture.Gate
	ref    string // "ssh[0]"
	pcache map[types.NodeID]*tailcfg.SSHPolicy
}

// policyFor returns this rule's compiled SSH policy for dst, compiling and
// caching on first use. A nil policy manager (the sub-rule failed to compile)
// yields a nil policy, which callers treat as "this rule never matches".
func (g *gatedSSHRule) policyFor(dst *types.Node) (*tailcfg.SSHPolicy, error) {
	if g.pm == nil {
		return nil, nil
	}
	if p, ok := g.pcache[dst.ID]; ok {
		return p, nil
	}
	p, err := g.pm.SSHPolicy(sshBaseURL, dst.View())
	if err != nil {
		return nil, err
	}
	g.pcache[dst.ID] = p
	return p, nil
}

// filterFor returns this rule's compiled packet filter for dst, compiling and
// caching on first use.
func (g *gatedRule) filterFor(dst *types.Node, allIPs *netipx.IPSet) (*filter.Filter, error) {
	if f, ok := g.fcache[dst.ID]; ok {
		return f, nil
	}
	f, _, err := nodeFilter(g.pm, dst, allIPs)
	if err != nil {
		return nil, err
	}
	g.fcache[dst.ID] = f
	return f, nil
}

// buildPostureLayer parses the policy's posture config, splits the cleaned
// policy into unconditional rules (returned as basePolicy JSON) and
// posture-gated rules (compiled individually), and returns the layer plus any
// diagnostics. basePolicy is always valid JSON ready for NewPolicyManager.
func buildPostureLayer(
	pf preflight.Result,
	users types.Users,
	nv views.Slice[types.NodeView],
) (basePolicy []byte, layer *postureLayer, diags []eval.Diagnostic) {
	pc := pf.Posture
	set, warns := posture.Parse(pc.Definitions, pc.CustomAttrs)

	report := &eval.PostureReport{
		Definitions:       pc.Definitions,
		DefaultSrcPosture: pc.DefaultSrcPosture,
		Warnings:          warns,
	}
	layer = &postureLayer{set: set, report: report}

	// defaultNames is the policy-wide baseline applied to any rule with no
	// srcPosture of its own (Tailscale's `defaultSrcPosture`). A rule's own
	// srcPosture replaces it, not adds to it. We apply it uniformly to every
	// acls/grants/ssh rule; a rule whose src is "*" still gets gated per source
	// device (real Tailscale rejects an explicit srcPosture on a "*" src, but
	// the docs don't say defaultSrcPosture is exempted, and gating each concrete
	// device is the safe reading of an org-wide baseline).
	defaultNames := pc.DefaultSrcPosture
	for _, n := range defaultNames {
		if !set.Has(n) {
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("defaultSrcPosture references posture %q which is not defined", n))
		}
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(pf.Policy), &doc); err != nil {
		// Should not happen — pf.Policy is engine-standardised JSON.
		diags = append(diags, diag("error", "internal", "%s", "posture layer: re-parsing cleaned policy: "+err.Error()))
		return []byte(pf.Policy), layer, diags
	}

	// Keys a one-rule sub-policy needs for alias resolution.
	shared := map[string]json.RawMessage{}
	for _, k := range []string{"groups", "hosts", "tagOwners"} {
		if v, ok := doc[k]; ok {
			shared[k] = v
		}
	}

	split := func(arrayKey string, gatedIdx map[int][]string) {
		rawArr, ok := doc[arrayKey]
		if !ok {
			return
		}
		var rules []json.RawMessage
		if err := json.Unmarshal(rawArr, &rules); err != nil {
			return
		}
		plain := make([]json.RawMessage, 0, len(rules))
		for i, rule := range rules {
			names := gatedIdx[i]
			fromDefault := false
			if len(names) == 0 {
				if len(defaultNames) == 0 {
					plain = append(plain, rule)
					continue
				}
				names = defaultNames
				fromDefault = true
			}
			ref := fmt.Sprintf("%s[%d]", arrayKey, i)
			recordGate(report, set, ref, names, fromDefault)

			sub := map[string]json.RawMessage{arrayKey: mustJSONArray(rule)}
			for k, v := range shared {
				sub[k] = v
			}
			subBytes, _ := json.Marshal(sub)
			pm, err := hspolicy.NewPolicyManager(subBytes, users, nv)
			if err != nil {
				diags = append(diags, diag("warning", "parse", "posture-gated %s: %s", ref, cleanErr(err)))
				continue
			}
			layer.gated = append(layer.gated, &gatedRule{
				pm:     pm,
				gate:   set.Gate(names),
				ref:    ref,
				fcache: map[types.NodeID]*filter.Filter{},
			})
		}
		doc[arrayKey] = mustJSONArray(plain...)
	}

	split("acls", pc.ACLSrcPosture)
	split("grants", pc.GrantSrcPosture)

	if len(pc.SSHSrcPosture) > 0 || len(defaultNames) > 0 {
		sshRules, sshDiags := splitSSH(doc, shared, pc.SSHSrcPosture, defaultNames, set, report, users, nv)
		layer.sshRules = sshRules
		diags = append(diags, sshDiags...)
	}

	base, err := json.Marshal(doc)
	if err != nil {
		return []byte(pf.Policy), layer, diags
	}
	return base, layer, diags
}

// recordGate adds rule ref's posture gate to the report and, unless the gate
// came from defaultSrcPosture, warns about each posture name the policy doesn't
// define.
func recordGate(report *eval.PostureReport, set *posture.Set, ref string, names []string, fromDefault bool) {
	report.Gates = append(report.Gates, eval.PostureGate{Rule: ref, Postures: names, Default: fromDefault})
	if fromDefault {
		return
	}
	for _, n := range names {
		if !set.Has(n) {
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("%s references posture %q which is not defined", ref, n))
		}
	}
}

func mustJSONArray(elems ...json.RawMessage) json.RawMessage {
	if elems == nil {
		elems = []json.RawMessage{}
	}
	b, _ := json.Marshal(elems)
	return b
}

// splitSSH compiles every ssh[] rule as its own single-rule sub-policy, in
// original order, tagging each with the posture gate from its (already
// stripped) srcPosture. It then clears doc["ssh"] so the base policy manager
// carries no SSH rules — evalSSH drives the ordered list itself. Tailscale SaaS
// resolves srcPosture at compile time (a source failing the gate is dropped
// from the rule's principals), so a gate miss is a non-match that falls through
// to the next rule, not a hard deny.
func splitSSH(
	doc map[string]json.RawMessage,
	shared map[string]json.RawMessage,
	gatedIdx map[int][]string,
	defaultNames []string,
	set *posture.Set,
	report *eval.PostureReport,
	users types.Users,
	nv views.Slice[types.NodeView],
) (out []*gatedSSHRule, diags []eval.Diagnostic) {
	rawArr, ok := doc["ssh"]
	if !ok {
		return nil, nil
	}
	var rules []json.RawMessage
	if err := json.Unmarshal(rawArr, &rules); err != nil {
		return nil, nil
	}
	for i, rule := range rules {
		names := gatedIdx[i]
		fromDefault := false
		if len(names) == 0 && len(defaultNames) > 0 {
			names = defaultNames
			fromDefault = true
		}
		ref := fmt.Sprintf("ssh[%d]", i)
		if len(names) > 0 {
			recordGate(report, set, ref, names, fromDefault)
		}

		g := &gatedSSHRule{gate: set.Gate(names), ref: ref, pcache: map[types.NodeID]*tailcfg.SSHPolicy{}}

		sub := map[string]json.RawMessage{"ssh": mustJSONArray(rule)}
		for k, v := range shared {
			sub[k] = v
		}
		subBytes, _ := json.Marshal(sub)
		if pm, err := hspolicy.NewPolicyManager(subBytes, users, nv); err != nil {
			diags = append(diags, diag("warning", "parse", "posture-gated %s: %s", ref, cleanErr(err)))
		} else {
			g.pm = pm
		}
		out = append(out, g)
	}
	doc["ssh"] = mustJSONArray() // the layer owns the ssh rules now
	return out, diags
}

// gatedContribution reports, for one (src,dst) pair, whether a gated rule would
// grant it anything (so we can tell a posture denial from a plain deny) and
// whether the src device satisfies the gate.
type pairPostureResult struct {
	// finalVerdict per probe name, after folding in gated rules the src passes.
	verdicts map[string]string
	// caps the src is granted, including via passing gated rules.
	caps []string
	// denialReason is set when the pair is denied only because a gated rule's
	// posture requirement failed for src.
	denialReason string
}

// evalPair computes the posture-aware verdicts, caps and denial reason for one
// (src,dst) pair. baseFilter is the unconditional-rule filter for dst.
func (l *postureLayer) evalPair(
	baseFilter *filter.Filter,
	src, dst *types.Node,
	attrs map[string]string,
	probes []eval.Probe,
	allIPs *netipx.IPSet,
	diags *[]eval.Diagnostic,
) pairPostureResult {
	res := pairPostureResult{verdicts: make(map[string]string, len(probes))}

	for _, p := range probes {
		res.verdicts[p.Name] = verdict(baseFilter, src, dst, p)
	}
	res.caps = capNames(baseFilter, src, dst)

	gatedAllowAny := false
	var blockedRef, blockedWhy string

	for _, g := range l.gated {
		f, err := g.filterFor(dst, allIPs)
		if err != nil {
			*diags = append(*diags, diag("warning", "internal", "posture-gated %s filter for %s: %v", g.ref, dst.Hostname, err))
			continue
		}

		accepts := false
		acceptProbes := map[string]bool{}
		for _, p := range probes {
			if verdict(f, src, dst, p) == eval.VerdictAllow {
				accepts = true
				acceptProbes[p.Name] = true
			}
		}
		gCaps := capNames(f, src, dst)
		if !accepts && len(gCaps) == 0 {
			continue
		}

		if g.gate.Allows(attrs) {
			gatedAllowAny = true
			for name := range acceptProbes {
				res.verdicts[name] = eval.VerdictAllow
			}
			res.caps = append(res.caps, gCaps...)
		} else if blockedRef == "" {
			blockedRef = g.ref
			blockedWhy = g.gate.Explain(attrs)
		}
	}

	res.caps = dedupeSorted(res.caps)

	// A gated rule would have reached this pair, but src fails its posture and no
	// other gated rule lets it through.
	if blockedRef != "" && !gatedAllowAny {
		res.denialReason = fmt.Sprintf("%s blocked — %s", blockedRef, blockedWhy)
	}
	return res
}

func dedupeSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// postureAttrs builds the attribute map the posture evaluator reads for a
// device: its explicit posture attrs (with any "node:" prefix trimmed), plus a
// fallback "os" from the device's OS field.
func postureAttrs(d eval.Device) map[string]string {
	m := make(map[string]string, len(d.Posture)+1)
	for k, v := range d.Posture {
		m[strings.TrimPrefix(k, "node:")] = v
	}
	if m["os"] == "" && d.OS != "" {
		m["os"] = d.OS
	}
	return m
}
