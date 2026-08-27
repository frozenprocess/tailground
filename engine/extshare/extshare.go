// Package extshare models Tailscale declarative node sharing: the policy's
// `externalTailnets` section plus the `group://<alias>/<name>` and
// `tag://<alias>/<name>` reference tokens that name identities in another
// tailnet. The Headscale policy compiler has no cross-tailnet concept — it
// rejects both — so preflight (see engine/preflight) parses the section, this
// package resolves each reference token against the shared nodes the snapshot
// supplies, and preflight rewrites the token to those nodes' addresses before
// the policy is compiled.
//
// # Semantics
//
// Modelled on Tailscale's declarative node sharing docs (see below; retrieved
// 2026-09-26). `externalTailnets` is keyed by a local alias; each value carries
// `externalID` (required), `allowIncomingConnections` and
// `allowExternalReferencesTo`. External references are only valid in a
// grant's `src`: the *sharing* tailnet writes the grant (external group/tag →
// one of its own resources) and sets `allowIncomingConnections: true`.
// Tailscale rejects the whole policy for a reference anywhere else (acls, a
// grant's dst or via, ssh, groups, …), for a malformed one, for one naming a
// tailnet externalTailnets doesn't list, and for a grant src whose tailnet
// doesn't allow incoming connections — preflight reports each as fatal.
// Access to another tailnet's nodes is granted by that tailnet's policy, not
// this one. `allowExternalReferencesTo` is the *receiving* half: it lists the
// local groups/tags the other tailnet's policy may name. That policy isn't
// visible here, so Tailground reports which local devices those entries cover
// rather than a verdict.
//
// What Tailscale accepts and rejects was checked against the live policy API
// (2026-10-09); engine/testdata/conformance/declarative-sharing.json records
// each case with its real result, and preflight's conformance test replays it.
// Aliases match case-insensitively, as they do in Tailscale.
//
// See https://tailscale.com/docs/features/declarative-node-sharing.
package extshare

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/frozenprocess/tailground/engine/eval"
)

// Reference-token schemes.
const (
	GroupScheme = "group://"
	TagScheme   = "tag://"
)

// TailnetConfig is one entry of the policy's `externalTailnets` section.
type TailnetConfig struct {
	ExternalID                string
	AllowIncomingConnections  bool
	AllowExternalReferencesTo []string
}

// Ref is a parsed cross-tailnet reference token.
type Ref struct {
	Raw   string // the original token, ports stripped
	Kind  string // "group" | "tag"
	Alias string
	Name  string // bare group/tag name, no "group:" / "tag:" prefix
}

// IsRef reports whether s begins with a cross-tailnet reference scheme. Like
// Tailscale, it ignores surrounding whitespace and the scheme's case.
func IsRef(s string) bool {
	_, _, ok := splitScheme(strings.TrimSpace(s))
	return ok
}

// splitScheme returns the reference kind ("group" | "tag") and what follows
// the scheme in s.
func splitScheme(s string) (kind, rest string, ok bool) {
	lower := strings.ToLower(s)
	switch {
	case strings.HasPrefix(lower, GroupScheme):
		return "group", s[len(GroupScheme):], true
	case strings.HasPrefix(lower, TagScheme):
		return "tag", s[len(TagScheme):], true
	}
	return "", "", false
}

// ParseRef parses "group://<alias>/<name>" or "tag://<alias>/<name>". The name
// may repeat its own prefix ("tag://corp/tag:relay") and may carry a trailing
// ":<ports>"; neither is part of the returned Name. ok is false for a token
// that is not a reference or is malformed — no "/" after the alias, or an
// empty alias or name — which Tailscale rejects as "unsupported target".
func ParseRef(s string) (Ref, bool) {
	t := strings.TrimSpace(s)
	kind, rest, ok := splitScheme(t)
	if !ok {
		return Ref{}, false
	}
	alias, after, found := strings.Cut(rest, "/")
	alias = strings.TrimSpace(alias)
	if !found || alias == "" {
		return Ref{}, false
	}
	// A group/tag name never contains a colon, so after an optional
	// "tag:" / "group:" prefix the first colon separates ports.
	skip := 0
	if strings.HasPrefix(strings.ToLower(after), kind+":") {
		skip = len(kind) + 1
	}
	if i := strings.IndexByte(after[skip:], ':'); i >= 0 {
		after = after[:skip+i]
	}
	name := strings.TrimSpace(after[skip:])
	if name == "" {
		return Ref{}, false
	}
	raw := t[:len(t)-len(rest)+strings.IndexByte(rest, '/')+1+len(after)]
	return Ref{Raw: raw, Kind: kind, Alias: alias, Name: name}, true
}

// Node is one shared node from an external tailnet, resolved.
type Node struct {
	Name   string
	Alias  string
	Tags   []string   // normalized with the "tag:" prefix
	Groups []string   // normalized with the "group:" prefix
	IPv4   netip.Addr // zero ⇒ the caller synthesizes one
	IPv6   netip.Addr // zero ⇒ the caller synthesizes one
}

// Column is the matrix key for this node ("ext:corp/relay").
func (n Node) Column() string { return Column(n.Alias, n.Name) }

// Column is the matrix key for shared node name of external tailnet alias.
func Column(alias, name string) string { return "ext:" + alias + "/" + name }

// Tailnet is one external tailnet: its config (from the policy) and its shared
// nodes (from the snapshot).
type Tailnet struct {
	Alias      string
	Config     TailnetConfig
	Configured bool // the policy's externalTailnets section has an entry
	Nodes      []Node
}

// Model ties the policy's externalTailnets config to the snapshot's shared
// nodes.
type Model struct {
	tailnets map[string]*Tailnet
	order    []string
}

// Build indexes the snapshot's external tailnets and folds in the policy
// config. A config entry with no matching snapshot tailnet, or a snapshot
// tailnet with a blank alias / a duplicate, is reported; the rest resolve.
func Build(ext []eval.ExternalTailnet, cfg map[string]TailnetConfig) (*Model, []eval.Diagnostic) {
	m := &Model{tailnets: map[string]*Tailnet{}}
	var diags []eval.Diagnostic

	sorted := append([]eval.ExternalTailnet(nil), ext...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return strings.TrimSpace(sorted[i].Alias) < strings.TrimSpace(sorted[j].Alias)
	})

	for _, et := range sorted {
		alias := strings.TrimSpace(et.Alias)
		if alias == "" {
			diags = append(diags, warn("an externalTailnets entry in the snapshot has no alias; it was ignored"))
			continue
		}
		if _, dup := m.tailnets[aliasKey(alias)]; dup {
			diags = append(diags, warn("external tailnet %q is defined more than once in the snapshot; the later entry was ignored", alias))
			continue
		}
		t := &Tailnet{Alias: alias}
		for _, en := range et.Nodes {
			name := strings.TrimSpace(en.Name)
			if name == "" {
				diags = append(diags, warn("external tailnet %q has a node with no name; it was ignored", alias))
				continue
			}
			n := Node{
				Name:   name,
				Alias:  alias,
				Tags:   normalize(en.Tags, "tag:"),
				Groups: normalize(en.Groups, "group:"),
			}
			if en.IPv4 != "" {
				if a, err := netip.ParseAddr(en.IPv4); err == nil && a.Is4() {
					n.IPv4 = a
				} else {
					diags = append(diags, warn("external node %q/%q has bad ipv4 %q; an address was synthesized", alias, name, en.IPv4))
				}
			}
			if en.IPv6 != "" {
				if a, err := netip.ParseAddr(en.IPv6); err == nil && a.Is6() {
					n.IPv6 = a
				} else {
					diags = append(diags, warn("external node %q/%q has bad ipv6 %q; an address was synthesized", alias, name, en.IPv6))
				}
			}
			t.Nodes = append(t.Nodes, n)
		}
		m.tailnets[aliasKey(alias)] = t
		m.order = append(m.order, aliasKey(alias))
	}

	for _, alias := range sortedKeys(cfg) {
		c := cfg[alias]
		t, ok := m.tailnets[aliasKey(alias)]
		if !ok {
			continue // preflight reports it once, on the externalTailnets key
		}
		t.Config = c
		t.Configured = true
	}

	return m, diags
}

// Tailnets returns the model's external tailnets in alias order.
func (m *Model) Tailnets() []*Tailnet {
	out := make([]*Tailnet, 0, len(m.order))
	for _, a := range m.order {
		out = append(out, m.tailnets[a])
	}
	return out
}

// Lookup returns the tailnet for an alias, matched case-insensitively.
func (m *Model) Lookup(alias string) (*Tailnet, bool) {
	t, ok := m.tailnets[aliasKey(alias)]
	return t, ok
}

// Empty reports whether the model has no external tailnets.
func (m *Model) Empty() bool { return len(m.order) == 0 }

// Resolve returns the shared nodes a reference selects. ok is false when the
// alias is unknown; an empty slice with ok true means the alias exists but no
// node carries the named group/tag.
func (m *Model) Resolve(ref Ref) ([]Node, bool) {
	t, ok := m.tailnets[aliasKey(ref.Alias)]
	if !ok {
		return nil, false
	}
	want := ref.Name
	prefix := "group:"
	if ref.Kind == "tag" {
		prefix = "tag:"
	}
	if !strings.HasPrefix(want, prefix) {
		want = prefix + want
	}
	var out []Node
	for _, n := range t.Nodes {
		set := n.Groups
		if ref.Kind == "tag" {
			set = n.Tags
		}
		for _, v := range set {
			if strings.EqualFold(v, want) {
				out = append(out, n)
				break
			}
		}
	}
	return out, true
}

// AllNodes returns every shared node across every external tailnet, in alias
// then node order.
func (m *Model) AllNodes() []Node {
	var out []Node
	for _, a := range m.order {
		out = append(out, m.tailnets[a].Nodes...)
	}
	return out
}

// IncomingNodes returns the shared nodes whose tailnet sets
// allowIncomingConnections — the set that may appear in a rule's src, and the
// set autogroup:shared unions there.
func (m *Model) IncomingNodes() []Node {
	var out []Node
	for _, a := range m.order {
		t := m.tailnets[a]
		if t.Config.AllowIncomingConnections {
			out = append(out, t.Nodes...)
		}
	}
	return out
}

// aliasKey is the map key for an alias: Tailscale matches aliases
// case-insensitively.
func aliasKey(alias string) string { return strings.ToLower(alias) }

func normalize(in []string, prefix string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.HasPrefix(s, prefix) {
			s = prefix + s
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func sortedKeys(m map[string]TailnetConfig) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func warn(format string, args ...any) eval.Diagnostic {
	return eval.Diagnostic{
		Severity: "warning", Code: "snapshot", Target: "external",
		Message: fmt.Sprintf(format, args...),
	}
}
