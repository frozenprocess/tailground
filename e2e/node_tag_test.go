//go:build e2e

package e2e

import "testing"

func TestTagArgRejectsFlagInjection(t *testing.T) {
	got := tagArg([]string{"tag:web", "tag:x --ssh --authkey=evil", "tag:db-1", "nope"})
	if want := " --advertise-tags=tag:web,tag:db-1"; got != want {
		t.Fatalf("tagArg = %q, want %q", got, want)
	}
	if got := tagArg([]string{"tag: bad"}); got != "" {
		t.Fatalf("tagArg = %q, want empty", got)
	}
}
