// Package ipset resolves Tailscale `ipsets` definitions to flat lists of CIDR
// prefixes. Headscale's policy compiler doesn't understand the `ipsets` section
// or the `ipset:<name>` alias, so preflight (see engine/preflight) lifts the
// section out, calls [Resolve], and expands every `ipset:<name>` token in a
// rule's src/dst to the resolved prefixes before the policy is compiled.
//
// A set member may be a CIDR ("10.0.0.0/8"), a bare IP ("100.64.0.1", widened
// to a host prefix), an inclusive IP range ("192.0.2.10-192.0.2.20"), a
// "host:<name>" reference into the policy's `hosts` section, or another
// "ipset:<name>" (resolved transitively; reference cycles are reported and
// skipped). Tags, users and groups are not valid members; `autogroup:internet`
// (the one autogroup Tailscale allows here) is not expanded and is reported.
//
// This is a preflight rewrite, not a semantic overlay: once expanded, the
// policy compiles and evaluates entirely through the normal Headscale path.
package ipset

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"go4.org/netipx"

	"github.com/frozenprocess/tailground/engine/eval"
)

// Sets maps an ipset name (without the "ipset:" prefix) to its resolved,
// de-duplicated, sorted prefixes.
type Sets map[string][]netip.Prefix

// Resolve flattens each entry of a raw `ipsets` object (name → list of members,
// keys already stripped of the "ipset:" prefix) to its prefixes. hosts is the
// policy's `hosts` section (name → IP or CIDR), used to resolve "host:<name>"
// members; it may be nil. Unparseable members, undefined references and
// reference cycles are returned as warning diagnostics; the affected set still
// resolves with whatever else it could.
func Resolve(defs map[string][]string, hosts map[string]string) (Sets, []eval.Diagnostic) {
	out := make(Sets, len(defs))
	var diags []eval.Diagnostic

	names := make([]string, 0, len(defs))
	for name := range defs {
		names = append(names, name)
	}
	sort.Strings(names) // deterministic diagnostic order

	for _, name := range names {
		prefixes, ds := resolveOne(name, defs, hosts, map[string]bool{})
		out[name] = dedupeSort(prefixes)
		diags = append(diags, ds...)
	}
	return out, diags
}

func resolveOne(name string, defs map[string][]string, hosts map[string]string, onPath map[string]bool) ([]netip.Prefix, []eval.Diagnostic) {
	if onPath[name] {
		return nil, []eval.Diagnostic{warn("ipset %q takes part in a reference cycle; the cyclic member was skipped", "ipset:"+name)}
	}
	members, ok := defs[name]
	if !ok {
		return nil, []eval.Diagnostic{warn("ipset %q is referenced but no ipsets entry defines it", "ipset:"+name)}
	}

	onPath[name] = true
	defer delete(onPath, name)

	var (
		out   []netip.Prefix
		diags []eval.Diagnostic
	)
	for _, raw := range members {
		m := strings.TrimSpace(raw)
		switch {
		case m == "":
			continue

		case strings.HasPrefix(m, "ipset:"):
			sub, ds := resolveOne(strings.TrimPrefix(m, "ipset:"), defs, hosts, onPath)
			out = append(out, sub...)
			diags = append(diags, ds...)

		case strings.HasPrefix(m, "host:"):
			h, ok := hosts[strings.TrimPrefix(m, "host:")]
			if !ok {
				diags = append(diags, warn("ipset %q references %q, which no hosts entry defines; it was skipped", "ipset:"+name, m))
				continue
			}
			if ps, err := parseOne(h); err == nil {
				out = append(out, ps...)
			} else {
				diags = append(diags, warn("ipset %q references host %q whose value %q is not an IP or CIDR; it was skipped", "ipset:"+name, m, h))
			}

		case strings.HasPrefix(m, "autogroup:"):
			diags = append(diags, warn("ipset %q member %q is not modeled inside an ipset; it was skipped", "ipset:"+name, m))

		default:
			if ps, err := parseOne(m); err == nil {
				out = append(out, ps...)
			} else {
				diags = append(diags, warn("ipset %q has member %q, which is not a CIDR, IP, range or ipset:/host: reference; it was skipped", "ipset:"+name, m))
			}
		}
	}
	return out, diags
}

// parseOne turns a single literal member (CIDR, bare IP, or "a-b" range) into
// prefixes.
func parseOne(m string) ([]netip.Prefix, error) {
	if strings.ContainsRune(m, '/') {
		p, err := netip.ParsePrefix(m)
		if err != nil {
			return nil, err
		}
		return []netip.Prefix{p.Masked()}, nil
	}
	if i := strings.IndexByte(m, '-'); i > 0 {
		r, err := netipx.ParseIPRange(m)
		if err != nil {
			return nil, err
		}
		return r.Prefixes(), nil
	}
	a, err := netip.ParseAddr(m)
	if err != nil {
		return nil, err
	}
	return []netip.Prefix{netip.PrefixFrom(a, a.BitLen())}, nil
}

func warn(format string, args ...any) eval.Diagnostic {
	return eval.Diagnostic{
		Severity: "warning", Code: "policy",
		Message: fmt.Sprintf(format, args...),
	}
}

func dedupeSort(in []netip.Prefix) []netip.Prefix {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[netip.Prefix]bool, len(in))
	out := in[:0]
	for _, p := range in {
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Addr() != out[j].Addr() {
			return out[i].Addr().Less(out[j].Addr())
		}
		return out[i].Bits() < out[j].Bits()
	})
	return out
}
