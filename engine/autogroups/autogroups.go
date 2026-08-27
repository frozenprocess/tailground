// Package autogroups resolves the Tailscale autogroup tokens that the Headscale
// policy compiler rejects outright — the admin-role groups and
// `autogroup:shared` — against Tailground's snapshot users.
//
// Headscale natively resolves autogroup:{internet,member,tagged,self,nonroot,
// danger-all}. The rest of the documented autogroups depend on a user's tailnet
// *role* or on cross-tailnet *sharing*, neither of which Headscale models, so
// its `AutoGroup` type fails to even unmarshal them. Preflight (see
// engine/preflight) calls [Resolve] and substitutes the matching users' emails
// into the policy before it is compiled, exactly as it does for `ipset:` and
// `user:*@<domain>`.
//
// This is Tailground's own reading of the syntax reference. The role→autogroup
// mapping below is pinned by tests; see the doc note on [Resolve].
package autogroups

import (
	"sort"
	"strings"

	"github.com/frozenprocess/tailground/engine/eval"
)

// Tailnet roles Tailground recognises on a snapshot user. An empty
// [eval.User.Role] is treated as RoleMember.
const (
	RoleOwner        = "owner"
	RoleAdmin        = "admin"
	RoleITAdmin      = "it-admin"
	RoleNetworkAdmin = "network-admin"
	RoleBillingAdmin = "billing-admin"
	RoleAuditor      = "auditor"
	RoleMember       = "member"
)

// Roles is every recognised role value, for validation by callers.
var Roles = []string{
	RoleOwner, RoleAdmin, RoleITAdmin, RoleNetworkAdmin,
	RoleBillingAdmin, RoleAuditor, RoleMember,
}

// roleForToken maps an autogroup name (the part after "autogroup:") to a
// predicate over a user's normalised role.
//
// Interpretation note (Tailscale syntax reference, retrieved 2026-08-28,
// https://tailscale.com/kb/1337/acl-syntax + https://tailscale.com/kb/1138/user-roles):
// the reference lists Owner and Admin as "basic" roles and it-admin /
// network-admin / billing-admin / auditor as separate "advanced" roles with no
// stated hierarchy. Tailground resolves `autogroup:admin` to users who are
// *either* Admin *or* Owner (the Owner can do anything an Admin can), and every
// other role token to an exact match. This is pinned in autogroups_test.go.
var roleForToken = map[string]func(role string) bool{
	"admin":         func(r string) bool { return r == RoleAdmin || r == RoleOwner },
	"owner":         func(r string) bool { return r == RoleOwner },
	"it-admin":      func(r string) bool { return r == RoleITAdmin },
	"network-admin": func(r string) bool { return r == RoleNetworkAdmin },
	"billing-admin": func(r string) bool { return r == RoleBillingAdmin },
	"auditor":       func(r string) bool { return r == RoleAuditor },
}

// Handles reports whether name (the token after "autogroup:") is one this
// package resolves. Tokens Headscale handles itself return false.
func Handles(name string) bool {
	if name == "shared" {
		return true
	}
	_, ok := roleForToken[name]
	return ok
}

// SrcOnly reports whether an autogroup this package handles is only valid on the
// src side of a rule (Tailscale rejects `autogroup:shared` anywhere else).
func SrcOnly(name string) bool { return name == "shared" }

// NormalizeRole lowercases role and maps "" to RoleMember. An unrecognised
// value is returned as-is (callers may want to warn).
func NormalizeRole(role string) string {
	r := strings.ToLower(strings.TrimSpace(role))
	if r == "" {
		return RoleMember
	}
	return r
}

// Resolve expands an `autogroup:<name>` token to the emails of the snapshot
// users it selects, sorted and de-duplicated. ok is false when name is not a
// Tailground-resolved autogroup, in which case the caller must leave the token
// alone (it is either Headscale-native or genuinely unknown).
//
// A recognised token that matches no user returns (nil, true): the caller
// should drop it and note that the rule lost a source/destination.
func Resolve(name string, users []eval.User) (emails []string, ok bool) {
	if !Handles(name) {
		return nil, false
	}
	pred := roleForToken[name]
	seen := map[string]bool{}
	for _, u := range users {
		match := false
		if name == "shared" {
			match = u.Shared
		} else {
			match = pred(NormalizeRole(u.Role))
		}
		if !match || u.Email == "" || seen[u.Email] {
			continue
		}
		seen[u.Email] = true
		emails = append(emails, u.Email)
	}
	sort.Strings(emails)
	return emails, true
}
