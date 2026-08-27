package route

import (
	"fmt"
	"net/netip"
	"sort"

	"github.com/frozenprocess/tailground/engine/eval"
)

// Path is one device advertising one route prefix.
type Path struct {
	Prefix   netip.Prefix
	Exit     bool // 0.0.0.0/0 or ::/0
	RouterID uint64
	Router   string
	Approved bool // advertised AND admin-approved
}

// Plan enumerates every (advertised prefix, device) pair across the snapshot,
// marking which are also approved, sorted by prefix string then router ID.
// Unparseable route strings and approve-without-advertise mismatches come back
// as warnings; the rest of the plan is still returned.
func Plan(devices []eval.Device) (paths []Path, warnings []string) {
	for _, d := range devices {
		approved := map[netip.Prefix]bool{}
		for _, s := range d.RoutesApproved {
			p, err := netip.ParsePrefix(s)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("device %q: unparseable approved route %q (%v)", d.Name, s, err))
				continue
			}
			approved[p.Masked()] = true
		}

		advertised := map[netip.Prefix]bool{}
		for _, s := range d.Routes {
			p, err := netip.ParsePrefix(s)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("device %q: unparseable route %q (%v)", d.Name, s, err))
				continue
			}
			pm := p.Masked()
			if advertised[pm] {
				continue
			}
			advertised[pm] = true
			paths = append(paths, Path{
				Prefix:   pm,
				Exit:     IsExit(pm),
				RouterID: d.ID,
				Router:   d.Name,
				Approved: approved[pm],
			})
		}

		for p := range approved {
			if !advertised[p] {
				warnings = append(warnings, fmt.Sprintf("device %q approves route %q it does not advertise; ignored", d.Name, p))
			}
		}
	}

	sort.Slice(paths, func(i, j int) bool {
		if paths[i].Prefix.String() != paths[j].Prefix.String() {
			return paths[i].Prefix.String() < paths[j].Prefix.String()
		}
		return paths[i].RouterID < paths[j].RouterID
	})
	return paths, warnings
}

// IsExit reports whether p is an exit route (a default route, /0).
func IsExit(p netip.Prefix) bool { return p.Bits() == 0 }

// SampleIP returns a concrete address inside p to evaluate reachability
// against: a well-known public address for an exit route, otherwise the first
// host address of the prefix (network address + 1, or the address itself for a
// single-host prefix).
func SampleIP(p netip.Prefix) netip.Addr {
	if IsExit(p) {
		if p.Addr().Is6() {
			return netip.MustParseAddr("2001:4860:4860::8888")
		}
		return netip.MustParseAddr("8.8.8.8")
	}
	if p.IsSingleIP() {
		return p.Addr()
	}
	return p.Masked().Addr().Next()
}

// Column is the Result.Matrices destination key for a path's per-source
// verdicts, e.g. "route:10.88.0.0/24@3".
func Column(prefix netip.Prefix, routerID uint64) string {
	return fmt.Sprintf("route:%s@%d", prefix, routerID)
}
