package hs

import (
	"net/netip"

	"github.com/juanfont/headscale/hscontrol/types"
	"tailscale.com/tailcfg"

	"github.com/frozenprocess/tailground/engine/eval"
	"github.com/frozenprocess/tailground/engine/posture"
)

// sshBaseURL only shapes the check-action HoldAndDelegate URL; its value never
// affects a verdict (a check rule always has a non-empty HoldAndDelegate).
const sshBaseURL = "https://check.tailground.invalid"

// ssh fills res.SSH with a per (login-user, src, dst) verdict, but only when
// the policy declares SSH rules for at least one device.
//
// When any ssh rule carries srcPosture, layer.sshRules holds the whole ordered
// rule list compiled individually and the per-source posture gate is applied
// here (gatedSSHVerdict); otherwise the fast path compiles the full SSH policy
// once per destination.
//
// Every verdict is additionally gated on L4 reachability of tcp/22: Tailscale
// SSH still rides an ordinary TCP connection to port 22, so if the destination's
// packet filter doesn't admit src->dst:22 the session never reaches the SSH
// handler — no matter what the ssh[] block says (real Tailscale behaviour,
// confirmed by the e2e differential: an ssh accept rule with no grant opening
// port 22 is denied on the wire). basePM/allIPs are the same inputs nodeFilter
// uses for the reachability matrix.
func (c *evalCtx) ssh(users []string) {
	if len(users) == 0 {
		users = eval.DefaultSSHUsers
	}
	gated := c.layer != nil && len(c.layer.sshRules) > 0
	sshPortProbe := eval.Probe{Proto: "tcp", Port: 22}

	for _, dst := range c.nodes {
		var basePol *tailcfg.SSHPolicy
		var gatedPols []gatedSSHPolicy
		if gated {
			gatedPols = resolveGatedSSH(c.layer.sshRules, dst, &c.res.Diagnostics)
			if len(gatedPols) == 0 {
				continue // no ssh rule targets dst
			}
		} else {
			pol, err := c.pm.SSHPolicy(sshBaseURL, dst.View())
			if err != nil {
				c.res.Diagnostics = append(c.res.Diagnostics, diag("warning", "internal", "SSH policy for %s: %v", dst.Hostname, cleanErr(err)))
				continue
			}
			if pol == nil || len(pol.Rules) == 0 {
				continue
			}
			basePol = pol
		}

		// The dst's L4 filter, for the tcp/22 reachability gate below.
		l4, _, ferr := nodeFilter(c.basePM, dst, c.allIPs)
		if ferr != nil {
			l4 = nil
		}

		for _, src := range c.nodes {
			if src.ID == dst.ID {
				continue
			}
			srcIPs := src.IPs()
			portOpen := l4 != nil && verdict(l4, src, dst, sshPortProbe) == eval.VerdictAllow
			var attrs map[string]string
			if gated {
				attrs = postureAttrs(c.devByID[uint64(src.ID)])
			}
			for _, u := range users {
				var v string
				if gated {
					v = gatedSSHVerdict(gatedPols, srcIPs, u, attrs)
				} else {
					v = sshVerdict(basePol, srcIPs, u)
				}
				if v != eval.SSHDeny && !portOpen {
					v = eval.SSHDeny
				}
				put(&c.res.SSH, eval.SSHKey(u, src.ID, dst.ID), v)
			}
		}
	}
}

// gatedSSHPolicy pairs one ssh[] rule's compiled policy for a destination with
// the posture gate its srcPosture imposes.
type gatedSSHPolicy struct {
	pol  *tailcfg.SSHPolicy
	gate posture.Gate
}

// resolveGatedSSH compiles every ssh[] sub-rule for dst once, keeping only the
// rules that name dst as a destination, in original order. Returns nil when no
// rule targets dst (so its res.SSH entries are omitted, matching the fast path).
func resolveGatedSSH(rules []*gatedSSHRule, dst *types.Node, diags *[]eval.Diagnostic) []gatedSSHPolicy {
	var out []gatedSSHPolicy
	for _, g := range rules {
		pol, err := g.policyFor(dst)
		if err != nil {
			*diags = append(*diags, diag("warning", "internal", "posture-gated %s SSH policy for %s: %v", g.ref, dst.Hostname, cleanErr(err)))
			continue
		}
		if pol == nil || len(pol.Rules) == 0 {
			continue // this rule does not target dst
		}
		out = append(out, gatedSSHPolicy{pol: pol, gate: g.gate})
	}
	return out
}

// gatedSSHVerdict walks the ordered ssh[] list for one (src, user), honouring
// each rule's srcPosture gate. A src that fails a rule's gate is treated as not
// matching that rule (Tailscale SaaS drops it from the rule's principals at
// compile time), so evaluation falls through to the next rule rather than
// hard-denying.
func gatedSSHVerdict(pols []gatedSSHPolicy, srcIPs []netip.Addr, user string, attrs map[string]string) string {
	for _, gp := range pols {
		v := sshVerdict(gp.pol, srcIPs, user)
		if v == eval.SSHDeny {
			continue // principals / users do not match this (src, user)
		}
		if !gp.gate.Allows(attrs) {
			continue // src fails srcPosture — rule does not apply, fall through
		}
		return v
	}
	return eval.SSHDeny
}

// sshVerdict returns the effective verdict for a login attempt: the first rule
// whose principals include a src IP and whose user map allows user wins.
// Headscale sorts check rules ahead of accept rules, matching Tailscale.
func sshVerdict(pol *tailcfg.SSHPolicy, srcIPs []netip.Addr, user string) string {
	for _, r := range pol.Rules {
		if r == nil || !sshPrincipalMatches(r.Principals, srcIPs) || !sshUserAllowed(r.SSHUsers, user) {
			continue
		}
		switch {
		case r.Action == nil:
			return eval.SSHDeny
		case r.Action.Reject:
			return eval.SSHReject
		case r.Action.HoldAndDelegate != "":
			return eval.SSHCheck
		case r.Action.Accept:
			return eval.SSHAccept
		default:
			return eval.SSHDeny
		}
	}
	return eval.SSHDeny
}

// sshPrincipalMatches mirrors Headscale's IP-only principal matching. Headscale
// resolves SSH src aliases to node IPs at compile time.
func sshPrincipalMatches(ps []*tailcfg.SSHPrincipal, srcIPs []netip.Addr) bool {
	for _, p := range ps {
		if p == nil {
			continue
		}
		if p.Any {
			return true
		}
		a, err := netip.ParseAddr(p.NodeIP)
		if err != nil {
			continue
		}
		for _, s := range srcIPs {
			if a == s {
				return true
			}
		}
	}
	return false
}

// sshUserAllowed mirrors Headscale's sshUserMapAllows: an explicit entry wins;
// root has no wildcard fallback; other users fall back to "*". A "" value means
// "not allowed".
func sshUserAllowed(m map[string]string, user string) bool {
	if user == "" {
		return false
	}
	if v, ok := m[user]; ok {
		return v != ""
	}
	if user == "root" {
		return false
	}
	if v, ok := m["*"]; ok {
		return v != ""
	}
	return false
}
