package preflight

import (
	"fmt"
	"strings"

	"github.com/tailscale/hujson"

	"github.com/frozenprocess/tailground/engine/autogroups"
	"github.com/frozenprocess/tailground/engine/eval"
	"github.com/frozenprocess/tailground/engine/idresolve"
	"github.com/frozenprocess/tailground/engine/ipset"
	"github.com/frozenprocess/tailground/engine/service"
)

// The rewriters in this file replace alias tokens Headscale can't compile
// (`user:*@d`, `autogroup:admin`, `ipset:x`, `svc:x`) with concrete ones, so the
// policy goes through the normal Headscale path. They share two helpers:
// forEachAliasField walks the rules, rewriteTokens rewrites one alias array.

// aliasField is one src/dst array of an acls/grants/ssh rule.
type aliasField struct {
	Key   string // "acls" | "grants" | "ssh"
	Index int    // the rule's index in Key
	Field string // "src" | "dst"
	Arr   *hujson.Array
}

// loc is the human-readable location, e.g. "grants[2].dst".
func (f aliasField) loc() string { return fmt.Sprintf("%s[%d].%s", f.Key, f.Index, f.Field) }

// withPorts reports whether tokens here carry a trailing `:<ports>` — only an
// acls dst does; grants and ssh dsts take ports in separate fields.
func (f aliasField) withPorts() bool { return f.Key == "acls" && f.Field == "dst" }

// forEachAliasField calls fn for every src and dst array of every acls, grants
// and ssh rule, in document order (src before dst within a rule).
func forEachAliasField(obj *hujson.Object, fn func(aliasField)) {
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
			for _, field := range []string{"src", "dst"} {
				if fa := arrayMember(rule, field); fa != nil {
					fn(aliasField{Key: key, Index: ri, Field: field, Arr: fa})
				}
			}
		}
	}
}

// rewriteTokens replaces string elements of fa. For each string literal fn
// returns (repl, true) to substitute repl (empty drops the element) or
// (nil, false) to leave it alone; off is the element's source offset, for
// diagnostics. The array is only written back if something matched.
func rewriteTokens(fa *hujson.Array, fn func(s string, off int) (repl []string, match bool)) {
	var out []hujson.Value
	changed := false
	for _, el := range fa.Elements {
		s, ok := stringLiteral(el.Value)
		if ok {
			if repl, match := fn(s, el.StartOffset); match {
				changed = true
				for _, r := range repl {
					out = append(out, hujson.Value{Value: hujson.String(r)})
				}
				continue
			}
		}
		out = append(out, el)
	}
	if changed {
		fa.Elements = out
	}
}

// cutPorts splits a `name:ports` alias tail at the first colon.
func cutPorts(s string) (name, ports string) {
	name, ports, _ = strings.Cut(s, ":")
	return name, ports
}

// withPortSuffix appends `:ports` to each token when ports is non-empty.
func withPortSuffix(toks []string, ports string) []string {
	if ports == "" {
		return toks
	}
	out := make([]string, len(toks))
	for i, t := range toks {
		out[i] = t + ":" + ports
	}
	return out
}

// expandUserWildcards replaces every `user:*@<domain>` token in an acls/grants/
// ssh rule's `src` with the explicit emails of the snapshot users in that
// domain, so the policy compiles through the normal Headscale path. Headscale
// has no domain-wildcard concept: it treats the token as a literal username,
// the lookup fails, and the rule silently matches nothing. The expansion is
// silent; a wildcard whose domain matches no snapshot user is dropped with a
// `snapshot` note, since the rule then matches nothing.
//
// `localpart:*@<domain>` in an ssh rule's `users` list is left untouched — the
// vendored Headscale compiler resolves that form itself.
func expandUserWildcards(raw string, obj *hujson.Object, users []eval.User, res *Result) {
	forEachAliasField(obj, func(f aliasField) {
		if f.Field != "src" {
			return
		}
		rewriteTokens(f.Arr, func(s string, off int) ([]string, bool) {
			if !idresolve.IsUserWildcard(s) {
				return nil, false
			}
			emails, _ := idresolve.ResolveUserWildcard(s, users)
			if len(emails) == 0 {
				res.info("snapshot", lineCol(raw, off),
					"%s %q matches no user in the snapshot, so it matches nothing — add a user in that domain under Users", f.loc(), s)
			}
			return emails, true
		})
	})
}

// rewriteRoleAutogroups replaces every admin-role or `autogroup:shared` token
// in an acls/grants/ssh rule's src/dst — and in tagOwners — with the emails of
// the snapshot users it selects (engine/autogroups). Headscale's compiler
// rejects these tokens outright (its AutoGroup type won't unmarshal them), so
// substituting concrete users lets the policy compile through the normal path.
// A token that matches no user is dropped with an info note.
//
// Headscale-native autogroups and genuinely unknown ones are left untouched
// here; scanAutogroups decides those.
func rewriteRoleAutogroups(raw string, obj *hujson.Object, users []eval.User, res *Result) {
	forEachAliasField(obj, func(f aliasField) {
		expandAutogroupTokens(raw, f.loc(), f.Arr, users, f.withPorts(), f.Field == "src", res)
	})
	if to := objectMember(obj, "tagOwners"); to != nil {
		for _, m := range to.Members {
			if fa, ok := m.Value.Value.(*hujson.Array); ok {
				loc := fmt.Sprintf("tagOwners[%q]", literalString(m.Name.Value))
				expandAutogroupTokens(raw, loc, fa, users, false, false, res)
			}
		}
	}
	if aa := objectMember(obj, "autoApprovers"); aa != nil {
		if routes := objectMember(aa, "routes"); routes != nil {
			for _, m := range routes.Members {
				if fa, ok := m.Value.Value.(*hujson.Array); ok {
					loc := fmt.Sprintf("autoApprovers.routes[%q]", literalString(m.Name.Value))
					expandAutogroupTokens(raw, loc, fa, users, false, false, res)
				}
			}
		}
		if exit := arrayMember(aa, "exitNode"); exit != nil {
			expandAutogroupTokens(raw, "autoApprovers.exitNode", exit, users, false, false, res)
		}
	}
}

// expandAutogroupTokens rewrites the Tailground-resolved autogroup tokens in one
// alias array. withPorts keeps a trailing `:<ports>` (acls dst); isSrc gates the
// src-only tokens (`autogroup:shared`).
func expandAutogroupTokens(raw, loc string, fa *hujson.Array, users []eval.User, withPorts, isSrc bool, res *Result) {
	rewriteTokens(fa, func(s string, off int) ([]string, bool) {
		if !strings.HasPrefix(s, "autogroup:") {
			return nil, false
		}
		name, ports := cutPorts(strings.TrimPrefix(s, "autogroup:"))
		if !withPorts {
			ports = ""
		}
		if !autogroups.Handles(name) {
			return nil, false // Headscale-native or unknown — scanAutogroups decides
		}
		ptr := lineCol(raw, off)
		if autogroups.SrcOnly(name) && !isSrc {
			res.warn("policy", ptr, "%s: %q is only valid in a rule's src and was dropped", loc, s)
			return nil, true
		}
		emails, _ := autogroups.Resolve(name, users)
		if len(emails) == 0 {
			// autogroup:shared still matches the external shared nodes that
			// resolveExternalShare appended after it — not "nothing".
			if name != "shared" || res.sharedIncoming == 0 {
				res.info("snapshot", ptr, "%s %q matches no user in the snapshot, so it matches nothing — %s", loc, s, autogroupFix(name))
			}
			return nil, true
		}
		return withPortSuffix(emails, ports), true
	})
}

// rewriteIPSets replaces every `ipset:<name>` token in an acls/grants/ssh rule's
// src/dst with the resolved prefixes for that set, so the policy compiles
// through the normal Headscale path. An acls `dst` token carries a `:<ports>`
// suffix, which is preserved on each expanded prefix; grants and ssh dsts don't.
// A token naming an undefined set is dropped with a warning.
func rewriteIPSets(raw string, obj *hujson.Object, sets ipset.Sets, res *Result) {
	forEachAliasField(obj, func(f aliasField) {
		rewriteTokens(f.Arr, func(s string, off int) ([]string, bool) {
			if !strings.HasPrefix(s, "ipset:") {
				return nil, false
			}
			name, ports := strings.TrimPrefix(s, "ipset:"), ""
			if f.withPorts() {
				name, ports = cutPorts(name)
			}
			prefixes, known := sets[name]
			if !known || len(prefixes) == 0 {
				why := "which resolved to no prefixes"
				if !known {
					why = "which no ipsets entry defines"
				}
				res.warn("policy", lineCol(raw, off), "%s references %q, %s; the entry was dropped", f.loc(), s, why)
				return nil, true
			}
			toks := make([]string, len(prefixes))
			for i, p := range prefixes {
				toks[i] = p.String()
			}
			return withPortSuffix(toks, ports), true
		})
	})
}

// rewriteServices replaces every `svc:<name>` destination alias in an
// acls/grants/ssh rule's `dst` with the service's VIP from the snapshot registry
// (engine/service), so the policy compiles through the normal Headscale path —
// which has no notion of Tailscale Services. An acls `dst` token carries a
// `:<ports>` suffix, preserved on the VIP (defaulting to `:*`); grants and ssh
// dsts don't. A token naming a service the snapshot doesn't define is dropped
// with a warning. `svc:` is a destination alias, so a `svc:` token in `src` is
// dropped with a warning too.
func rewriteServices(raw string, obj *hujson.Object, reg service.Registry, res *Result) {
	forEachAliasField(obj, func(f aliasField) {
		if f.Field == "src" {
			dropServiceTokens(raw, f.loc(), f.Arr, res)
		} else {
			expandServiceTokens(raw, f.loc(), f.Arr, reg, f.withPorts(), res)
		}
	})
}

// expandServiceTokens rewrites the `svc:<name>` tokens in one dst array to the
// service VIP. withPorts keeps a trailing `:<ports>` (acls dst), defaulting a
// bare token to `:*`.
func expandServiceTokens(raw, loc string, fa *hujson.Array, reg service.Registry, withPorts bool, res *Result) {
	rewriteTokens(fa, func(s string, off int) ([]string, bool) {
		if !service.IsToken(s) {
			return nil, false
		}
		name, ports := service.Split(s)
		svc, known := reg[name]
		if !known {
			res.warn("snapshot", lineCol(raw, off),
				"%s references %q, which no service in the snapshot defines; the entry was dropped — add it under Tailscale Services", loc, s)
			return nil, true
		}
		tok := svc.VIP.String()
		if withPorts {
			if ports == "" {
				ports = "*"
			}
			tok += ":" + ports
		}
		return []string{tok}, true
	})
}

// dropServiceTokens removes any `svc:<name>` token from a src array (it's a
// destination-only alias) with a warning.
func dropServiceTokens(raw, loc string, fa *hujson.Array, res *Result) {
	rewriteTokens(fa, func(s string, off int) ([]string, bool) {
		if !service.IsToken(s) {
			return nil, false
		}
		res.warn("policy", lineCol(raw, off), "%s: %q is a destination alias and can't appear in src; it was dropped", loc, s)
		return nil, true
	})
}
