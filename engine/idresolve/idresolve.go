// Package idresolve resolves Tailscale identity tokens that the Headscale
// policy compiler does not model natively, against Tailground's snapshot users.
//
// Today it handles one token: the source membership wildcard
// `user:*@<domain>`, which Tailscale expands to every direct tailnet member
// whose login is in <domain>. Headscale has no domain-wildcard concept — it
// treats the whole token as a literal username, the lookup fails, and the rule
// silently matches nothing. So preflight (see engine/preflight) calls
// [ResolveUserWildcard] and replaces the token with the matching users'
// explicit emails before the policy is compiled.
//
// `localpart:*@<domain>` in an `ssh` rule's `users` list is deliberately NOT
// handled here: the vendored Headscale compiler resolves that form itself.
//
// This is a preflight rewrite, not a semantic overlay: once expanded, the
// policy compiles and evaluates entirely through the normal Headscale path.
package idresolve

import (
	"sort"
	"strings"

	"github.com/frozenprocess/tailground/engine/eval"
)

// UserWildcardPrefix is the token prefix Tailscale uses for a domain-scoped
// membership wildcard in a rule's `src`: `user:*@<domain>`.
const UserWildcardPrefix = "user:*@"

// userWildcardDomain returns the lowercased <domain> of a `user:*@<domain>`
// token. ok is false when s is not such a token.
func userWildcardDomain(s string) (domain string, ok bool) {
	if !strings.HasPrefix(s, UserWildcardPrefix) {
		return "", false
	}
	d := strings.TrimPrefix(s, UserWildcardPrefix)
	if d == "" || strings.ContainsAny(d, "@*") {
		return "", false
	}
	return strings.ToLower(d), true
}

// IsUserWildcard reports whether s is a `user:*@<domain>` token.
func IsUserWildcard(s string) bool {
	_, ok := userWildcardDomain(s)
	return ok
}

// ResolveUserWildcard expands a `user:*@<domain>` token to the emails of the
// snapshot users whose login is in <domain> (case-insensitive on the domain),
// sorted and de-duplicated. ok is false when token is not a user wildcard, in
// which case emails is nil and the caller should leave the token untouched.
//
// A well-formed wildcard that matches no user returns (nil, true): the caller
// should drop the token and note that the rule now has one fewer source.
func ResolveUserWildcard(token string, users []eval.User) (emails []string, ok bool) {
	domain, ok := userWildcardDomain(token)
	if !ok {
		return nil, false
	}
	seen := map[string]bool{}
	for _, u := range users {
		at := strings.LastIndexByte(u.Email, '@')
		if at < 0 {
			continue
		}
		if !strings.EqualFold(u.Email[at+1:], domain) || seen[u.Email] {
			continue
		}
		seen[u.Email] = true
		emails = append(emails, u.Email)
	}
	sort.Strings(emails)
	return emails, true
}
