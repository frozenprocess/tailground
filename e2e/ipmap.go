//go:build e2e

package e2e

import (
	"net/netip"
	"regexp"
	"sort"
	"strings"

	"github.com/frozenprocess/tailground/engine/eval"
)

// Rewriter maps the fictional addresses authored in a case (100.x / fd7a:) to
// the real Tailscale addresses the launched nodes were assigned, then applies
// that mapping to both the snapshot and the policy so the engine and the API
// evaluate byte-identical input.
type Rewriter struct {
	// byName maps a device Name to its real (v4, v6) addresses.
	v4 map[string]netip.Addr
	v6 map[string]netip.Addr
	// literal maps an authored IP string to its real replacement, for policy
	// text substitution (hosts, dst IP literals).
	literal map[string]string
	// os / tsVer carry the posture attributes Tailscale populates automatically
	// (node:os, node:tsVersion).
	os    map[string]string
	tsVer map[string]string
}

// NewRewriter builds the mapping from the authored request and the launched
// nodes (matched by device Name).
func NewRewriter(req eval.Request, nodes []*Node) *Rewriter {
	rw := &Rewriter{
		v4:      map[string]netip.Addr{},
		v6:      map[string]netip.Addr{},
		literal: map[string]string{},
		os:      map[string]string{},
		tsVer:   map[string]string{},
	}
	byName := map[string]*Node{}
	for _, n := range nodes {
		byName[n.Name] = n
	}
	for _, dev := range req.Snapshot.Devices {
		n, ok := byName[dev.Name]
		if !ok {
			continue
		}
		if n.OS != "" {
			rw.os[dev.Name] = n.OS
		}
		if n.TSVersion != "" {
			rw.tsVer[dev.Name] = n.TSVersion
		}
		if n.IPv4.IsValid() {
			rw.v4[dev.Name] = n.IPv4
			if dev.IPv4 != "" {
				rw.literal[dev.IPv4] = n.IPv4.String()
			}
		}
		if n.IPv6.IsValid() {
			rw.v6[dev.Name] = n.IPv6
			if dev.IPv6 != "" {
				rw.literal[dev.IPv6] = n.IPv6.String()
			}
		}
	}
	return rw
}

// Apply returns a copy of req with device addresses and policy IP literals
// rewritten to the real values.
func (rw *Rewriter) Apply(req eval.Request) eval.Request {
	out := req
	out.Policy = rw.rewriteText(req.Policy)

	devs := make([]eval.Device, len(req.Snapshot.Devices))
	copy(devs, req.Snapshot.Devices)
	for i := range devs {
		if ip, ok := rw.v4[devs[i].Name]; ok {
			devs[i].IPv4 = ip.String()
		}
		if ip, ok := rw.v6[devs[i].Name]; ok {
			devs[i].IPv6 = ip.String()
		}
		if os, ok := rw.os[devs[i].Name]; ok {
			devs[i].OS = os
		}
		if v, ok := rw.tsVer[devs[i].Name]; ok {
			p := make(map[string]string, len(devs[i].Posture)+1)
			for k, val := range devs[i].Posture {
				p[k] = val
			}
			p["tsVersion"] = v
			devs[i].Posture = p
		}
	}
	out.Snapshot = req.Snapshot
	out.Snapshot.Devices = devs

	if len(req.Snapshot.Hosts) > 0 {
		hosts := make(map[string]string, len(req.Snapshot.Hosts))
		for k, v := range req.Snapshot.Hosts {
			hosts[k] = rw.rewriteText(v)
		}
		out.Snapshot.Hosts = hosts
	}
	return out
}

// RealAddr returns the real v4 address a device was assigned.
func (rw *Rewriter) RealAddr(deviceName string) (netip.Addr, bool) {
	ip, ok := rw.v4[deviceName]
	return ip, ok
}

func (rw *Rewriter) rewriteText(s string) string {
	if s == "" || len(rw.literal) == 0 {
		return s
	}
	// Replace each authored IP literal, but only at a boundary so "100.64.0.2"
	// never matches inside "100.64.0.20". Longest literals first for the same
	// reason. IPv6 literals are distinctive enough for a plain replace.
	froms := make([]string, 0, len(rw.literal))
	for from := range rw.literal {
		froms = append(froms, from)
	}
	sort.Slice(froms, func(i, j int) bool { return len(froms[i]) > len(froms[j]) })
	for _, from := range froms {
		to := rw.literal[from]
		if strings.Contains(from, ":") { // IPv6
			s = strings.ReplaceAll(s, from, to)
			continue
		}
		re := regexp.MustCompile(regexp.QuoteMeta(from) + `(\D|$)`)
		s = re.ReplaceAllString(s, to+"$1")
	}
	return s
}
