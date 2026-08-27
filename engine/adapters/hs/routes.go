package hs

import (
	"net/netip"

	hspolicy "github.com/juanfont/headscale/hscontrol/policy"
	"github.com/juanfont/headscale/hscontrol/types"
	"go4.org/netipx"
	"tailscale.com/types/ipproto"
	"tailscale.com/types/logger"
	"tailscale.com/wgengine/filter"

	"github.com/frozenprocess/tailground/engine/eval"
	"github.com/frozenprocess/tailground/engine/route"
)

func (c *evalCtx) routes() {
	paths, warns := route.Plan(c.devices)
	if len(paths) == 0 {
		return
	}
	rr := &eval.RouteReport{Warnings: warns}
	if c.hasPosture {
		rr.Warnings = append(rr.Warnings, "route reachability is evaluated against the full policy; posture gates on route rules are not applied separately")
	}

	byID := make(map[uint64]*types.Node, len(c.nodes))
	for _, n := range c.nodes {
		byID[uint64(n.ID)] = n
	}
	logIPs := unionPrefixes(c.nodes)

	for _, p := range paths {
		rp := eval.RoutePath{
			Prefix:   p.Prefix.String(),
			Exit:     p.Exit,
			RouterID: p.RouterID,
			Router:   p.Router,
			Approved: p.Approved,
		}
		router := byID[p.RouterID]
		if p.Approved && router != nil {
			sample := route.SampleIP(p.Prefix)
			rp.Probe = sample.String()
			rp.Column = route.Column(p.Prefix, p.RouterID)

			if f, err := routerFilter(c.pm, router, logIPs); err != nil {
				c.res.Diagnostics = append(c.res.Diagnostics, diag("warning", "internal", "compiling route filter for %s: %v", router.Hostname, cleanErr(err)))
			} else {
				for _, src := range c.nodes {
					if src.ID == router.ID {
						continue
					}
					for _, probe := range c.probes {
						m := matrixFor(c.res, probe.Name)
						m[eval.PairKey(src.ID, rp.Column)] = routeVerdict(f, src, sample, probe)
					}
				}
			}
		}
		rr.Paths = append(rr.Paths, rp)
	}
	c.res.Routes = rr
}

// routerFilter compiles the packet filter for a subnet router / exit node,
// with its approved routes added to the filter's local set so filter.Check
// accepts a probe addressed into one of those prefixes (filter.runIn drops any
// packet whose destination isn't local).
func routerFilter(pm hspolicy.PolicyManager, router *types.Node, logIPs *netipx.IPSet) (*filter.Filter, error) {
	rules, err := pm.FilterForNode(router.View())
	if err != nil {
		return nil, err
	}
	matches, _ := filter.MatchesFromFilterRules(rules)

	var b netipx.IPSetBuilder
	for _, p := range router.Prefixes() {
		b.AddPrefix(p)
	}
	for _, p := range router.AllApprovedRoutes() {
		b.AddPrefix(p)
	}
	localIPs, _ := b.IPSet()

	return filter.New(matches, nil, localIPs, logIPs, nil, logger.Discard), nil
}

// routeVerdict runs filter.Check from every same-family src address to the
// route's sample address and returns allow if any is accepted.
func routeVerdict(f *filter.Filter, src *types.Node, dst netip.Addr, probe eval.Probe) string {
	port := uint16(probe.Port)
	base := protoOf(probe.Proto)
	for _, sa := range src.IPs() {
		if sa.Is4() != dst.Is4() {
			continue
		}
		proto := base
		if base == ipproto.ICMPv4 && sa.Is6() {
			proto = ipproto.ICMPv6
		}
		if f.Check(sa, dst, port, proto) == filter.Accept {
			return eval.VerdictAllow
		}
	}
	return eval.VerdictDeny
}
