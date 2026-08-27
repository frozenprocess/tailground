package hs

import (
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	hspolicy "github.com/juanfont/headscale/hscontrol/policy"
	"github.com/juanfont/headscale/hscontrol/types"
	"go4.org/netipx"
	"tailscale.com/tailcfg"
	"tailscale.com/types/ipproto"
	"tailscale.com/types/logger"
	"tailscale.com/wgengine/filter"

	"github.com/frozenprocess/tailground/engine/eval"
	"github.com/frozenprocess/tailground/engine/preflight"
)

// nodeFilter compiles the packet filter a destination node would enforce under
// pm. It returns the raw compiled rules too (for the inspector view).
func nodeFilter(pm hspolicy.PolicyManager, dst *types.Node, allIPs *netipx.IPSet) (*filter.Filter, []tailcfg.FilterRule, error) {
	rules, err := pm.FilterForNode(dst.View())
	if err != nil {
		return nil, nil, err
	}
	matches, _ := filter.MatchesFromFilterRules(rules) // best effort; near-never errors

	var localB netipx.IPSetBuilder
	for _, p := range dst.Prefixes() {
		localB.AddPrefix(p)
	}
	localIPs, _ := localB.IPSet()

	return filter.New(matches, nil, localIPs, allIPs, nil, logger.Discard), rules, nil
}

// unionPrefixes is the logIPs set filter.New wants: every node's prefixes plus
// every approved route, so route probes to an off-tailnet sample address still
// satisfy the IP-only quick-match paths inside the filter.
func unionPrefixes(nodes []*types.Node) *netipx.IPSet {
	var b netipx.IPSetBuilder
	for _, n := range nodes {
		for _, p := range n.Prefixes() {
			b.AddPrefix(p)
		}
		for _, p := range n.AllApprovedRoutes() {
			b.AddPrefix(p)
		}
	}
	s, _ := b.IPSet()
	return s
}

// parsePrefixes parses a list of CIDR strings, silently dropping the ones that
// don't parse — engine/route re-parses the same strings and is the single
// place that reports the bad ones.
func parsePrefixes(ss []string) []netip.Prefix {
	var out []netip.Prefix
	for _, s := range ss {
		if p, err := netip.ParsePrefix(s); err == nil {
			out = append(out, p.Masked())
		}
	}
	return out
}

// verdict runs filter.Check for every same-family (srcIP, dstIP) pair and
// returns allow if any pair is accepted.
func verdict(f *filter.Filter, src, dst *types.Node, probe eval.Probe) string {
	port := uint16(probe.Port)
	base := protoOf(probe.Proto)

	for _, sa := range src.IPs() {
		for _, da := range dst.IPs() {
			if sa.Is4() != da.Is4() {
				continue
			}
			proto := base
			if base == ipproto.ICMPv4 && sa.Is6() {
				proto = ipproto.ICMPv6
			}
			if f.Check(sa, da, port, proto) == filter.Accept {
				return eval.VerdictAllow
			}
		}
	}
	return eval.VerdictDeny
}

// capNames returns the sorted set of application capability names src is
// granted on dst.
func capNames(f *filter.Filter, src, dst *types.Node) []string {
	seen := map[string]bool{}
	for _, sa := range src.IPs() {
		for _, da := range dst.IPs() {
			if sa.Is4() != da.Is4() {
				continue
			}
			for cap := range f.CapsWithValues(sa, da) {
				seen[string(cap)] = true
			}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// protoOf resolves a probe's protocol string to an IANA number. It accepts the
// common aliases, every name tailscale's ipproto package knows (tcp, udp, sctp,
// icmp, icmpv6, gre, esp, ah, igmp, egp, igp, ipv4/ip-in-ip, dccp, …) and any
// raw protocol number 0-255. An unrecognised string falls back to TCP.
func protoOf(s string) ipproto.Proto {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return ipproto.TCP
	case "icmp", "icmpv4", "icmp4":
		return ipproto.ICMPv4
	case "icmpv6", "icmp6", "ipv6-icmp":
		return ipproto.ICMPv6
	case "ipip":
		return ipproto.Proto(4)
	}
	var p ipproto.Proto
	if err := p.UnmarshalText([]byte(strings.ToLower(strings.TrimSpace(s)))); err == nil {
		return p
	}
	return ipproto.TCP
}

// foldAppNodeCaps replaces every AppTargetCapPrefix marker cap in caps (stamped
// by preflight on the nodes a `nodeAttrs[].app` entry targets, resolved by
// Headscale) with that entry's real app capabilities.
func foldAppNodeCaps(caps map[string][]string, appCaps map[int]map[string][]string) map[string][]string {
	var markers []string
	for name := range caps {
		if strings.HasPrefix(name, preflight.AppTargetCapPrefix) {
			markers = append(markers, name)
		}
	}
	for _, name := range markers {
		delete(caps, name)
		idx, err := strconv.Atoi(strings.TrimPrefix(name, preflight.AppTargetCapPrefix))
		if err != nil {
			continue
		}
		for capName, vals := range appCaps[idx] {
			caps[capName] = append(caps[capName], vals...)
		}
	}
	return caps
}

// nodeCapStrings converts a tailcfg.NodeCapMap to a plain map of cap name → raw
// JSON value strings.
func nodeCapStrings(cm tailcfg.NodeCapMap) map[string][]string {
	out := make(map[string][]string, len(cm))
	for cap, vals := range cm {
		ss := make([]string, 0, len(vals))
		for _, v := range vals {
			ss = append(ss, string(v))
		}
		out[string(cap)] = ss
	}
	return out
}

// filterRuleViews projects tailcfg.FilterRules into the JSON-friendly view the
// inspector renders.
func filterRuleViews(rules []tailcfg.FilterRule) []eval.FilterRuleView {
	out := make([]eval.FilterRuleView, 0, len(rules))
	for _, r := range rules {
		v := eval.FilterRuleView{SrcIPs: r.SrcIPs, IPProto: append([]int(nil), r.IPProto...)}
		for _, d := range r.DstPorts {
			ports := "*"
			if !(d.Ports.First == 0 && d.Ports.Last == 65535) {
				if d.Ports.First == d.Ports.Last {
					ports = fmt.Sprintf("%d", d.Ports.First)
				} else {
					ports = fmt.Sprintf("%d-%d", d.Ports.First, d.Ports.Last)
				}
			}
			v.DstPorts = append(v.DstPorts, d.IP+":"+ports)
		}
		for _, cg := range r.CapGrant {
			dsts := make([]string, 0, len(cg.Dsts))
			for _, p := range cg.Dsts {
				dsts = append(dsts, p.String())
			}
			var caps []string
			for _, c := range cg.Caps {
				caps = append(caps, string(c))
			}
			for c := range cg.CapMap {
				caps = append(caps, string(c))
			}
			sort.Strings(caps)
			v.CapGrant = append(v.CapGrant, strings.Join(dsts, ",")+" => "+strings.Join(caps, ","))
		}
		out = append(out, v)
	}
	return out
}
