package hs

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/juanfont/headscale/hscontrol/types"
	"tailscale.com/types/ipproto"
	"tailscale.com/wgengine/filter"

	"github.com/frozenprocess/tailground/engine/eval"
	"github.com/frozenprocess/tailground/engine/extshare"
	"github.com/frozenprocess/tailground/engine/preflight"
)

// extNodeIDBase offsets synthetic shared-node IDs well clear of the snapshot's
// own device IDs.
const extNodeIDBase = 1_000_000

type externalPeer struct {
	node    *types.Node
	alias   string
	name    string
	tags    []string
	groups  []string
	inbound bool // its tailnet sets allowIncomingConnections
	ip4     netip.Addr
	ip6     netip.Addr
}

func (p *externalPeer) column() string { return extshare.Column(p.alias, p.name) }

// buildExternalNodes materializes each shared node preflight resolved as a
// Headscale peer. They carry a synthetic per-tailnet tag so Headscale accepts
// them as tagged nodes; their real tag / group membership was already consumed
// by the preflight token rewrite.
func buildExternalNodes(share *preflight.ExternalShare) []*externalPeer {
	if share == nil {
		return nil
	}
	var out []*externalPeer
	i := 0
	for _, t := range share.Tailnets {
		for _, n := range t.Nodes {
			p := &externalPeer{
				alias: t.Alias, name: n.Name,
				tags: n.Tags, groups: n.Groups,
				inbound: t.AllowIncomingConnections,
			}
			hn := &types.Node{
				ID:        types.NodeID(extNodeIDBase + i),
				Hostname:  p.column(),
				GivenName: p.column(),
				Tags:      types.Strings{"tag:external-" + t.Alias},
			}
			if a, err := netip.ParseAddr(n.IPv4); err == nil {
				hn.IPv4, p.ip4 = &a, a
			}
			if a, err := netip.ParseAddr(n.IPv6); err == nil {
				hn.IPv6, p.ip6 = &a, a
			}
			p.node = hn
			out = append(out, p)
			i++
		}
	}
	return out
}

func (c *evalCtx) externalShare(peers []*externalPeer, share *preflight.ExternalShare) {
	if share == nil || len(share.Tailnets) == 0 {
		return
	}
	rep := &eval.ExternalShareReport{}
	if c.hasPosture {
		rep.Warnings = append(rep.Warnings, "shared-node reachability is evaluated against the full policy; posture gates on those rules are not applied separately")
	}

	peersByCol := make(map[string]*externalPeer, len(peers))
	for _, p := range peers {
		peersByCol[p.column()] = p
	}

	for _, t := range share.Tailnets {
		tv := eval.ExternalTailnetView{
			Alias:                     t.Alias,
			ExternalID:                t.ExternalID,
			AllowIncomingConnections:  t.AllowIncomingConnections,
			AllowExternalReferencesTo: t.AllowExternalReferencesTo,
			Configured:                t.Configured,
			Referenceable:             referenceableDevices(c.devices, t.AllowExternalReferencesTo, t.ExposedUsers),
			Nodes:                     []eval.ExternalNodeView{},
		}
		if t.Configured && !t.AllowIncomingConnections {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("external tailnet %q does not set allowIncomingConnections; its shared nodes cannot initiate connections into this tailnet", t.Alias))
		}
		for _, sn := range t.Nodes {
			p := peersByCol[extshare.Column(t.Alias, sn.Name)]
			if p == nil {
				continue
			}
			nv := eval.ExternalNodeView{
				Name: p.name, Tags: p.tags, Groups: p.groups,
				IPs: ipStrings(p.node), Column: p.column(), Inbound: p.inbound,
			}

			// inbound: shared node -> local device (only when permitted).
			if p.inbound {
				for _, dst := range c.nodes {
					inF, _, err := nodeFilter(c.pm, dst, c.allIPs)
					if err != nil {
						continue
					}
					for _, probe := range c.probes {
						matrixFor(c.res, probe.Name)[eval.PairKey(p.column(), dst.ID)] = extInboundVerdict(inF, p, dst, probe)
					}
				}
			}

			tv.Nodes = append(tv.Nodes, nv)
		}
		rep.Tailnets = append(rep.Tailnets, tv)
	}
	c.res.ExternalShare = rep
}

// referenceableDevices returns the IDs of the local devices another tailnet's
// policy may name as a src: devices carrying a tag the allowExternalReferencesTo
// list names, plus the untagged devices of users in a group it names
// (preflight resolved those to exposedUsers). Tagged devices shed their user's
// identity, so group membership never covers them.
func referenceableDevices(devices []eval.Device, refs, exposedUsers []string) []uint64 {
	tags := map[string]bool{}
	for _, r := range refs {
		if strings.HasPrefix(r, "tag:") {
			tags[r] = true
		}
	}
	users := map[string]bool{}
	for _, u := range exposedUsers {
		users[strings.ToLower(u)] = true
	}
	out := []uint64{}
	for _, d := range devices {
		hit := false
		if len(d.Tags) > 0 {
			for _, t := range d.Tags {
				hit = hit || tags[t]
			}
		} else {
			hit = users[strings.ToLower(d.User)]
		}
		if hit {
			out = append(out, d.ID)
		}
	}
	return out
}

// extInboundVerdict checks the shared node reaching a local device.
func extInboundVerdict(f *filter.Filter, p *externalPeer, dst *types.Node, probe eval.Probe) string {
	port := uint16(probe.Port)
	base := protoOf(probe.Proto)
	for _, sa := range []netip.Addr{p.ip4, p.ip6} {
		if !sa.IsValid() {
			continue
		}
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
