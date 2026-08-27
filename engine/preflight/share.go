package preflight

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/tailscale/hujson"

	"github.com/frozenprocess/tailground/engine/eval"
	"github.com/frozenprocess/tailground/engine/extshare"
	"github.com/frozenprocess/tailground/engine/idresolve"
)

// resolveExternalShare parses the policy's externalTailnets config against the
// snapshot's shared nodes, assigns each shared node an address, records the
// resolved model on res.ExternalShare, and rewrites every
// `group://<alias>/<name>` / `tag://<alias>/<name>` token in a grant's src to
// the addresses of the shared nodes it selects — so the policy compiles
// through the normal Headscale path, which has no cross-tailnet concept. A
// grant's src is the only place Tailscale accepts one: checkExternalRefPlacement
// has already made every other placement fatal, and rewriteExternalRefTokens
// makes a malformed reference, an unknown alias, or a tailnet without
// `allowIncomingConnections: true` fatal too. An `autogroup:shared` src
// additionally gains every shared node's address (engine/autogroups still
// expands the token itself, to the shared local users).
//
// Successful rewrites are silent — they're faithful translations, nothing for
// the user to fix. Only problems surface.
func resolveExternalShare(raw string, obj *hujson.Object, ext []eval.ExternalTailnet, users []eval.User, cfg map[string]extshare.TailnetConfig, extPtr string, res *Result) {
	model, diags := extshare.Build(ext, cfg)
	res.Diagnostics = append(res.Diagnostics, diags...)

	// A policy alias the snapshot lacks is reported once, on the
	// externalTailnets key; its references then drop without repeating it.
	for _, alias := range sortedAliases(cfg) {
		if t, _ := model.Lookup(alias); t == nil {
			res.note("warning", "snapshot", extPtr, "externalTailnets configures %q, but the snapshot has no external tailnet with that alias, so its group:// / tag:// references match nothing — add it under External tailnets", alias)
		}
	}
	res.sharedIncoming = len(model.IncomingNodes())

	// Assign an address to every shared node (honouring any the snapshot set).
	v4, v6 := extIPv4Base, extIPv6Base
	for _, t := range model.Tailnets() {
		for i := range t.Nodes {
			if !t.Nodes[i].IPv4.IsValid() {
				t.Nodes[i].IPv4, v4 = v4, v4.Next()
			}
			if !t.Nodes[i].IPv6.IsValid() {
				t.Nodes[i].IPv6, v6 = v6, v6.Next()
			}
		}
	}

	if !model.Empty() {
		share := &ExternalShare{}
		for _, t := range model.Tailnets() {
			st := ExternalShareTailnet{
				Alias:                     t.Alias,
				ExternalID:                t.Config.ExternalID,
				AllowIncomingConnections:  t.Config.AllowIncomingConnections,
				AllowExternalReferencesTo: t.Config.AllowExternalReferencesTo,
				Configured:                t.Configured,
				ExposedUsers:              exposedUsers(obj, t.Config.AllowExternalReferencesTo, users),
			}
			for _, n := range t.Nodes {
				st.Nodes = append(st.Nodes, ExternalShareNode{
					Name: n.Name, Tags: n.Tags, Groups: n.Groups,
					IPv4: n.IPv4.String(), IPv6: n.IPv6.String(),
				})
			}
			share.Tailnets = append(share.Tailnets, st)
			validateExternalRefsTo(obj, t, extPtr, res)
		}
		res.ExternalShare = share
	}

	// Rewrite the src tokens in every rule. External references only resolve
	// in grants; elsewhere checkExternalRefPlacement has already failed them.
	for _, key := range []string{"acls", "grants", "ssh"} {
		arr := arrayMember(obj, key)
		if arr == nil {
			continue
		}
		for ri := range arr.Elements {
			rule, ok := arr.Elements[ri].Value.(*hujson.Object)
			if !ok {
				continue
			}
			if fa := arrayMember(rule, "src"); fa != nil {
				loc := fmt.Sprintf("%s[%d].src", key, ri)
				rewriteExternalRefTokens(raw, loc, fa, model, key == "grants", cfg, res)
			}
		}
	}
}

func sortedAliases(cfg map[string]extshare.TailnetConfig) []string {
	out := make([]string, 0, len(cfg))
	for a := range cfg {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// rewriteExternalRefTokens replaces the group:// / tag:// tokens in one src
// array with the addresses of the shared nodes they select, and appends every
// shared node's address after an `autogroup:shared` token. refsAllowed is true
// only for a grant's src; anywhere else a reference has already been reported
// as fatal by checkExternalRefPlacement and is left alone.
func rewriteExternalRefTokens(raw, loc string, fa *hujson.Array, model *extshare.Model, refsAllowed bool, cfg map[string]extshare.TailnetConfig, res *Result) {
	var out []hujson.Value
	changed := false
	for i := range fa.Elements {
		s, ok := stringLiteral(fa.Elements[i].Value)
		if !ok {
			out = append(out, fa.Elements[i])
			continue
		}
		ptr := lineCol(raw, fa.Elements[i].StartOffset)

		if s == "autogroup:shared" {
			if nodes := model.IncomingNodes(); len(nodes) > 0 {
				changed = true
				out = append(out, fa.Elements[i]) // keep the token for engine/autogroups
				for _, n := range nodes {
					out = append(out, addrValues(n)...)
				}
				continue
			}
			out = append(out, fa.Elements[i])
			continue
		}

		if !refsAllowed || !extshare.IsRef(s) {
			out = append(out, fa.Elements[i])
			continue
		}
		changed = true

		ref, alias, msg := checkGrantSrcRef(loc, s, cfg)
		if msg != "" {
			res.note("error", "policy", ptr, "%s", msg)
			res.Fatal = true
			continue
		}
		nodes, known := model.Resolve(ref)
		if !known {
			continue // the snapshot lacks this tailnet; reported once, on the externalTailnets key
		}
		if len(nodes) == 0 {
			res.note("info", "snapshot", ptr, "%s %q matched no shared node in %q and was dropped", loc, s, alias)
			continue
		}
		for _, n := range nodes {
			out = append(out, addrValues(n)...)
		}
	}
	if changed {
		fa.Elements = out
	}
}

// addrValues returns the string-literal address values for a shared node.
func addrValues(n extshare.Node) []hujson.Value {
	var out []hujson.Value
	if n.IPv4.IsValid() {
		out = append(out, hujson.Value{Value: hujson.String(n.IPv4.String())})
	}
	if n.IPv6.IsValid() {
		out = append(out, hujson.Value{Value: hujson.String(n.IPv6.String())})
	}
	return out
}

// validateExternalRefsTo warns when an allowExternalReferencesTo entry names a
// local group/tag the policy doesn't define (a likely typo). It gates nothing —
// with only this policy in view Tailground can't see what the other tailnet's
// policy references.
func validateExternalRefsTo(obj *hujson.Object, t *extshare.Tailnet, ptr string, res *Result) {
	if len(t.Config.AllowExternalReferencesTo) == 0 {
		return
	}
	groups := objectMember(obj, "groups")
	tagOwners := objectMember(obj, "tagOwners")
	for _, ref := range t.Config.AllowExternalReferencesTo {
		switch {
		case strings.HasPrefix(ref, "group:"):
			if groups == nil || objectMember(groups, ref) == nil && !hasMemberKey(groups, ref) {
				res.note("warning", "policy", ptr, "externalTailnets[%q].allowExternalReferencesTo names %q, which no groups entry defines", t.Alias, ref)
			}
		case strings.HasPrefix(ref, "tag:"):
			if tagOwners == nil || !hasMemberKey(tagOwners, ref) {
				res.note("warning", "policy", ptr, "externalTailnets[%q].allowExternalReferencesTo names %q, which no tagOwners entry defines", t.Alias, ref)
			}
		}
	}
}

// exposedUsers returns the emails of the snapshot users in the local groups an
// allowExternalReferencesTo list names (plain emails and `user:*@<domain>`
// wildcards; other members are ignored), sorted and de-duplicated.
func exposedUsers(obj *hujson.Object, refs []string, users []eval.User) []string {
	groups := objectMember(obj, "groups")
	if groups == nil {
		return nil
	}
	defs := parseNameLists(hujson.Value{Value: groups})
	seen := map[string]bool{}
	var out []string
	add := func(e string) {
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	for _, ref := range refs {
		if !strings.HasPrefix(ref, "group:") {
			continue
		}
		for _, m := range defs[ref] {
			if idresolve.IsUserWildcard(m) {
				emails, _ := idresolve.ResolveUserWildcard(m, users)
				for _, e := range emails {
					add(e)
				}
			} else if strings.Contains(m, "@") {
				add(strings.TrimPrefix(m, "user:"))
			}
		}
	}
	sort.Strings(out)
	return out
}

// hasMemberKey reports whether obj has a member whose key equals name.
func hasMemberKey(obj *hujson.Object, name string) bool {
	for _, m := range obj.Members {
		if literalString(m.Name.Value) == name {
			return true
		}
	}
	return false
}

// parseExternalTailnets reads the policy's `externalTailnets` section — an
// object keyed by the local alias, each value carrying `externalID`,
// `allowIncomingConnections` and `allowExternalReferencesTo`.
func parseExternalTailnets(v hujson.Value) map[string]extshare.TailnetConfig {
	return mapMembers(v, func(v hujson.Value) (extshare.TailnetConfig, bool) {
		var tc extshare.TailnetConfig
		cfg, ok := v.Value.(*hujson.Object)
		if !ok {
			return tc, true
		}
		for _, cm := range cfg.Members {
			switch strings.ToLower(literalString(cm.Name.Value)) {
			case "externalid", "id":
				tc.ExternalID = literalString(cm.Value.Value)
			case "allowincomingconnections":
				if lit, ok := cm.Value.Value.(hujson.Literal); ok {
					tc.AllowIncomingConnections = lit.Bool()
				}
			case "allowexternalreferencesto":
				if arr, ok := cm.Value.Value.(*hujson.Array); ok {
					tc.AllowExternalReferencesTo = stringElems(arr)
				}
			}
		}
		return tc, true
	})
}

var (
	extIPv4Base = netip.MustParseAddr("100.127.0.1")
	extIPv6Base = netip.MustParseAddr("fd7a:115c:a1e0:ffff::1")
)
