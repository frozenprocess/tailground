package hs

import (
	"fmt"

	"github.com/juanfont/headscale/hscontrol/types"
	"tailscale.com/wgengine/filter"

	"github.com/frozenprocess/tailground/engine/eval"
	"github.com/frozenprocess/tailground/engine/service"
)

func (c *evalCtx) services(services []eval.Service) {
	reg, _ := service.Build(services) // diagnostics already emitted by preflight
	if len(reg) == 0 {
		return
	}

	byID := make(map[uint64]*types.Node, len(c.nodes))
	for _, n := range c.nodes {
		byID[uint64(n.ID)] = n
	}
	logIPs := unionPrefixes(c.nodes)

	sr := &eval.ServiceReport{}
	if c.hasPosture {
		sr.Warnings = append(sr.Warnings, "service reachability is evaluated against the full policy; posture gates on service rules are not applied separately")
	}

	for _, name := range reg.Names() {
		svc := reg[name]
		sp := eval.ServicePath{Name: svc.Column(), VIP: svc.VIP.String(), Backends: svc.Backends}

		isBackend := make(map[uint64]bool, len(svc.Backends))
		var filters []*filter.Filter
		for _, b := range svc.Backends {
			isBackend[b] = true
			bn := byID[b]
			if bn == nil {
				continue
			}
			if f, err := routerFilter(c.pm, bn, logIPs); err != nil {
				c.res.Diagnostics = append(c.res.Diagnostics, diag("warning", "internal", "compiling service filter for %s: %v", bn.Hostname, cleanErr(err)))
			} else {
				filters = append(filters, f)
			}
		}
		if len(filters) == 0 {
			sr.Warnings = append(sr.Warnings, fmt.Sprintf("service %q has no backend device in the snapshot; reachability was not evaluated", sp.Name))
			sr.Paths = append(sr.Paths, sp)
			continue
		}
		sp.Column = svc.Column()

		for _, src := range c.nodes {
			if isBackend[uint64(src.ID)] {
				continue
			}
			for _, probe := range c.probes {
				m := matrixFor(c.res, probe.Name)
				v := eval.VerdictDeny
				for _, f := range filters {
					if routeVerdict(f, src, svc.VIP, probe) == eval.VerdictAllow {
						v = eval.VerdictAllow
						break
					}
				}
				m[eval.PairKey(src.ID, sp.Column)] = v
			}
		}
		sr.Paths = append(sr.Paths, sp)
	}
	c.res.Services = sr
}
