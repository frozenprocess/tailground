package hs

import (
	"net/netip"
	"sort"
	"strings"

	"github.com/juanfont/headscale/hscontrol/types"
	"tailscale.com/tailcfg"

	"github.com/frozenprocess/tailground/engine/eval"
)

// buildTailnet maps a Tailground snapshot to Headscale's users + nodes, and
// builds the Actor directory the UI renders from.
func buildTailnet(snap eval.Snapshot) (types.Users, []*types.Node, []eval.Actor, []eval.Diagnostic) {
	var diags []eval.Diagnostic

	users := make(types.Users, 0, len(snap.Users))
	for _, u := range snap.Users {
		hu := types.User{Name: u.Email, Email: u.Email}
		hu.ID = uint(u.ID)
		users = append(users, hu)
	}
	usersByEmail := make(map[string]*types.User, len(users))
	for i := range users {
		usersByEmail[users[i].Email] = &users[i]
	}

	nodes := make([]*types.Node, 0, len(snap.Devices))
	actors := make([]eval.Actor, 0, len(snap.Devices))

	v4seq, v6seq := 1, 1
	seenIP := map[string]string{} // ip -> device name

	vipByBackend := map[uint64][]netip.Prefix{}
	for _, s := range snap.Services {
		vip, err := netip.ParseAddr(strings.TrimSpace(s.VIP))
		if err != nil {
			continue
		}
		p := netip.PrefixFrom(vip, vip.BitLen())
		for _, b := range s.Backends {
			vipByBackend[b] = append(vipByBackend[b], p)
		}
	}

	for _, d := range snap.Devices {
		n := &types.Node{
			ID:        types.NodeID(d.ID),
			Hostname:  d.Name,
			GivenName: d.Name,
		}

		n.IPv4 = assignAddr(d.IPv4, d.Name, "ipv4", &v4seq, &diags)
		n.IPv6 = assignAddr(d.IPv6, d.Name, "ipv6", &v6seq, &diags)

		if rp := parsePrefixes(d.Routes); len(rp) > 0 {
			n.Hostinfo = &tailcfg.Hostinfo{RoutableIPs: rp}
		}
		n.ApprovedRoutes = parsePrefixes(d.RoutesApproved)
		if vips := vipByBackend[d.ID]; len(vips) > 0 {
			if n.Hostinfo == nil {
				n.Hostinfo = &tailcfg.Hostinfo{}
			}
			n.Hostinfo.RoutableIPs = append(n.Hostinfo.RoutableIPs, vips...)
			n.ApprovedRoutes = append(n.ApprovedRoutes, vips...)
		}

		for _, a := range []*netip.Addr{n.IPv4, n.IPv6} {
			if a == nil {
				continue
			}
			if prev, ok := seenIP[a.String()]; ok {
				diags = append(diags, deviceWarn(d.Name, "devices %q and %q share IP %s", prev, d.Name, a))
			}
			seenIP[a.String()] = d.Name
		}

		kind := "tagged"
		switch {
		case len(d.Tags) > 0:
			n.Tags = types.Strings(d.Tags)
		case d.User != "":
			if u, ok := usersByEmail[d.User]; ok {
				n.User = u
				id := u.ID
				n.UserID = &id
				kind = "user"
			} else {
				diags = append(diags, deviceWarn(d.Name, "device %q references unknown user %q", d.Name, d.User))
			}
		default:
			diags = append(diags, deviceWarn(d.Name, "device %q has neither a user nor tags", d.Name))
		}

		nodes = append(nodes, n)
		actors = append(actors, eval.Actor{
			ID: d.ID, Name: d.Name, Kind: kind, User: d.User, Tags: d.Tags, IPs: ipStrings(n),
		})
	}

	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	sort.Slice(actors, func(i, j int) bool { return actors[i].ID < actors[j].ID })

	return users, nodes, actors, diags
}

// assignAddr parses want, or synthesizes the next address in the Tailscale
// CGNAT (100.64.0.0/10) / ULA (fd7a:115c:a1e0::/48) range when want is empty.
func assignAddr(want, dev, field string, seq *int, diags *[]eval.Diagnostic) *netip.Addr {
	if want != "" {
		a, err := netip.ParseAddr(want)
		if err != nil {
			*diags = append(*diags, deviceWarn(dev, "device %q: bad %s %q", dev, field, want))
			return nil
		}
		return &a
	}
	n := *seq
	*seq++
	var a netip.Addr
	if field == "ipv4" {
		a = netip.AddrFrom4([4]byte{100, 64, byte(n >> 8), byte(n)})
	} else {
		a = netip.AddrFrom16([16]byte{0xfd, 0x7a, 0x11, 0x5c, 0xa1, 0xe0, 0, 0, 0, 0, 0, 0, 0, 0, byte(n >> 8), byte(n)})
	}
	return &a
}

func ipStrings(n *types.Node) []string {
	var out []string
	for _, a := range n.IPs() {
		out = append(out, a.String())
	}
	return out
}
