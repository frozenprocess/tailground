package ipset

import (
	"net/netip"
	"strings"
	"testing"

	"github.com/frozenprocess/tailground/engine/eval"
)

func strs(ps []netip.Prefix) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.String()
	}
	return out
}

func eq(t *testing.T, got []netip.Prefix, want ...string) {
	t.Helper()
	if strings.Join(strs(got), ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", strs(got), want)
	}
}

func hasMsg(diags []eval.Diagnostic, sub string) bool {
	for _, d := range diags {
		if strings.Contains(d.Message, sub) {
			return true
		}
	}
	return false
}

func TestResolve_CIDRsIPsRanges(t *testing.T) {
	sets, diags := Resolve(map[string][]string{
		"corp": {"10.0.0.0/24", "100.64.0.5", "192.0.2.10-192.0.2.11"},
	}, nil)
	if len(diags) != 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	eq(t, sets["corp"], "10.0.0.0/24", "100.64.0.5/32", "192.0.2.10/31")
}

func TestResolve_NestedAndDedupe(t *testing.T) {
	sets, _ := Resolve(map[string][]string{
		"a": {"10.0.0.0/24", "ipset:b"},
		"b": {"10.0.0.0/24", "10.1.0.0/24"},
	}, nil)
	eq(t, sets["a"], "10.0.0.0/24", "10.1.0.0/24")
}

func TestResolve_NonMaskedCIDRIsMasked(t *testing.T) {
	sets, _ := Resolve(map[string][]string{"a": {"10.0.0.9/24"}}, nil)
	eq(t, sets["a"], "10.0.0.0/24")
}

func TestResolve_HostRef(t *testing.T) {
	sets, diags := Resolve(
		map[string][]string{"a": {"host:gw", "host:missing"}},
		map[string]string{"gw": "100.64.0.9"},
	)
	eq(t, sets["a"], "100.64.0.9/32")
	if !hasMsg(diags, "no hosts entry defines") {
		t.Fatalf("missing host warning: %v", diags)
	}
}

func TestResolve_Cycle(t *testing.T) {
	sets, diags := Resolve(map[string][]string{
		"a": {"ipset:b", "10.0.0.0/24"},
		"b": {"ipset:a", "10.1.0.0/24"},
	}, nil)
	eq(t, sets["a"], "10.0.0.0/24", "10.1.0.0/24")
	if !hasMsg(diags, "reference cycle") {
		t.Fatalf("expected cycle warning, got %v", diags)
	}
}

func TestResolve_DiamondIsNotACycle(t *testing.T) {
	_, diags := Resolve(map[string][]string{
		"top":  {"ipset:l", "ipset:r"},
		"l":    {"ipset:leaf"},
		"r":    {"ipset:leaf"},
		"leaf": {"10.0.0.0/24"},
	}, nil)
	if hasMsg(diags, "cycle") {
		t.Fatalf("diamond misreported as cycle: %v", diags)
	}
}

func TestResolve_UndefinedAndBadMembers(t *testing.T) {
	sets, diags := Resolve(map[string][]string{
		"a": {"ipset:ghost", "not-an-ip", "tag:x", "autogroup:internet"},
	}, nil)
	if len(sets["a"]) != 0 {
		t.Fatalf("want empty set, got %v", strs(sets["a"]))
	}
	for _, want := range []string{
		"no ipsets entry defines it",
		"not a CIDR, IP, range",
		"not modeled inside an ipset",
	} {
		if !hasMsg(diags, want) {
			t.Errorf("missing diagnostic %q in %v", want, diags)
		}
	}
}
