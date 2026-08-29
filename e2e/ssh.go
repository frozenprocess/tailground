//go:build e2e

package e2e

import "strings"

// SSHVerdict is the classified outcome of an SSH login attempt. The values
// match engine/eval's SSH verdict strings so the engine's Result.SSH map and a
// live probe compare directly.
type SSHVerdict string

const (
	SSHAccept SSHVerdict = "accept" // session opened
	SSHCheck  SSHVerdict = "check"
	SSHReject SSHVerdict = "reject" // an ssh rule matched with action:reject
	SSHDeny   SSHVerdict = "deny"   // no matching ssh rule / no route
)

// classifySSH turns the combined stdout+stderr of a non-interactive
// `tailscale ssh … true` (and whether it exited 0) into a verdict. The second
// return is false when the output looks transient (policy not yet propagated),
// asking the caller to retry.
func classifySSH(out string, exitOK bool) (SSHVerdict, bool) {
	l := strings.ToLower(out)

	switch {
	case containsAny(l,
		"additional check", "requires an additional", "to authenticate, visit",
		"check.tailground", "/check/", "tailscale ssh requires"):
		return SSHCheck, true

	case containsAny(l,
		"tailscale ssh not enabled",
		"does not have ssh", "ssh is not enabled"):
		// The dst node hasn't advertised --ssh yet, or tailscaled is still
		// coming up — retry.
		return SSHDeny, false

	case containsAny(l,
		"rejected", "not permitted", "permission denied",
		"no matching ssh", "denied by policy"):
		return SSHReject, true

	case exitOK && !containsAny(l, "error", "refused", "timed out", "timeout"):
		return SSHAccept, true

	case containsAny(l, "connection refused", "no route to host",
		"connection timed out", "operation timed out", "i/o timeout",
		"could not resolve", "name or service not known"):
		return SSHDeny, true
	}

	// Unrecognised — retry a couple of times, then the caller falls back to deny.
	return SSHDeny, false
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
