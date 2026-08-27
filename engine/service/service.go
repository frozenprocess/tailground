// Package service resolves Tailscale Service destination aliases (`svc:<name>`)
// to the service's virtual IP. Headscale's policy compiler has no notion of
// Tailscale Services — a `svc:<name>` token in a rule's `dst` is an undefined
// alias to it — so preflight (see engine/preflight) rewrites every such token to
// the concrete VIP from the snapshot's service registry before the policy is
// compiled, and the hs adapter probes reachability to that VIP against each
// backend device's filter.
//
// A Service is defined only in the snapshot (Snapshot.Services): a name, one
// virtual IP, and the device IDs that advertise it. There is no policy syntax
// for defining a Service — Tailscale provisions those out of band.
//
// This is a preflight rewrite, not a semantic overlay: once `svc:<name>` becomes
// a VIP, the policy compiles and evaluates through the normal Headscale path.
package service

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/frozenprocess/tailground/engine/eval"
)

// TokenPrefix is the destination-alias prefix for a Tailscale Service.
const TokenPrefix = "svc:"

// Service is one resolved Tailscale Service.
type Service struct {
	Name     string     // bare name, without the "svc:" prefix
	VIP      netip.Addr // the service virtual IP
	Backends []uint64   // device IDs that advertise the service
}

// Column is the matrix destination key for the service ("svc:web").
func (s Service) Column() string { return TokenPrefix + s.Name }

// Registry maps a bare service name to its resolved definition.
type Registry map[string]Service

// Build validates and indexes a snapshot service list. A service with an empty
// name, a malformed VIP, or a name already seen is dropped with a warning
// diagnostic; the rest still resolve. Diagnostics are returned in a
// deterministic (name-sorted) order.
func Build(services []eval.Service) (Registry, []eval.Diagnostic) {
	reg := make(Registry, len(services))
	var diags []eval.Diagnostic

	// Sort a copy by normalized name so diagnostics are deterministic.
	sorted := make([]eval.Service, len(services))
	copy(sorted, services)
	sort.SliceStable(sorted, func(i, j int) bool {
		return Normalize(sorted[i].Name) < Normalize(sorted[j].Name)
	})

	for _, s := range sorted {
		name := Normalize(s.Name)
		if name == "" {
			diags = append(diags, warn("a service entry has no name; it was ignored"))
			continue
		}
		if _, dup := reg[name]; dup {
			diags = append(diags, warn("service %q is defined more than once; the later entry was ignored", TokenPrefix+name))
			continue
		}
		vip, err := netip.ParseAddr(strings.TrimSpace(s.VIP))
		if err != nil {
			diags = append(diags, warn("service %q has VIP %q, which is not an IP address; the service was ignored", TokenPrefix+name, s.VIP))
			continue
		}
		reg[name] = Service{
			Name:     name,
			VIP:      vip,
			Backends: append([]uint64(nil), s.Backends...),
		}
	}
	return reg, diags
}

// Names returns the registry's service names, sorted.
func (r Registry) Names() []string {
	out := make([]string, 0, len(r))
	for n := range r {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Normalize strips a leading "svc:" and surrounding space from a service name.
func Normalize(name string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(name), TokenPrefix))
}

// IsToken reports whether s is a `svc:<name>` destination alias (optionally with
// a trailing `:<ports>`).
func IsToken(s string) bool { return strings.HasPrefix(s, TokenPrefix) }

// Split breaks "svc:<name>:<ports>" into the bare service name and the trailing
// port spec ("" when the token carries no ports). A service name never contains
// a colon, so the first colon after the prefix separates name from ports.
func Split(token string) (name, ports string) {
	rest := strings.TrimPrefix(token, TokenPrefix)
	if i := strings.IndexByte(rest, ':'); i >= 0 {
		return strings.TrimSpace(rest[:i]), rest[i+1:]
	}
	return strings.TrimSpace(rest), ""
}

func warn(format string, args ...any) eval.Diagnostic {
	return eval.Diagnostic{
		Severity: "warning", Code: "snapshot", Target: "services",
		Message: fmt.Sprintf(format, args...),
	}
}
