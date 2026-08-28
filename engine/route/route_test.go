package route

import (
	"net/netip"
	"reflect"
	"testing"

	"github.com/frozenprocess/tailground/engine/eval"
)

func TestPlan_AdvertisedApprovedAndPending(t *testing.T) {
	devs := []eval.Device{
		{ID: 1, Name: "client"},
		{ID: 3, Name: "relay", Routes: []string{"10.88.0.0/24", "192.168.9.0/24"}, RoutesApproved: []string{"10.88.0.0/24"}},
		{ID: 4, Name: "gw", Routes: []string{"0.0.0.0/0"}, RoutesApproved: []string{"0.0.0.0/0"}},
	}
	paths, warns := Plan(devs)
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	want := []Path{
		{Prefix: netip.MustParsePrefix("0.0.0.0/0"), Exit: true, RouterID: 4, Router: "gw", Approved: true},
		{Prefix: netip.MustParsePrefix("10.88.0.0/24"), RouterID: 3, Router: "relay", Approved: true},
		{Prefix: netip.MustParsePrefix("192.168.9.0/24"), RouterID: 3, Router: "relay", Approved: false},
	}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("Plan =\n %+v\nwant\n %+v", paths, want)
	}
}

func TestPlan_ApproveWithoutAdvertiseWarns(t *testing.T) {
	_, warns := Plan([]eval.Device{
		{ID: 1, Name: "relay", RoutesApproved: []string{"10.0.0.0/24"}},
	})
	if len(warns) != 1 {
		t.Fatalf("want 1 warning, got %v", warns)
	}
}

func TestPlan_BadPrefixWarns(t *testing.T) {
	paths, warns := Plan([]eval.Device{
		{ID: 1, Name: "relay", Routes: []string{"not-a-cidr", "10.0.0.0/24"}},
	})
	if len(warns) != 1 || len(paths) != 1 {
		t.Fatalf("paths=%v warns=%v", paths, warns)
	}
}

func TestPlan_NonMaskedPrefixIsMasked(t *testing.T) {
	paths, _ := Plan([]eval.Device{
		{ID: 1, Name: "r", Routes: []string{"10.88.0.5/24"}, RoutesApproved: []string{"10.88.0.9/24"}},
	})
	if len(paths) != 1 || paths[0].Prefix.String() != "10.88.0.0/24" || !paths[0].Approved {
		t.Errorf("got %+v", paths)
	}
}

func TestSampleIP(t *testing.T) {
	cases := map[string]string{
		"10.88.0.0/24":  "10.88.0.1",
		"10.88.0.42/32": "10.88.0.42",
		"0.0.0.0/0":     "8.8.8.8",
		"::/0":          "2001:4860:4860::8888",
	}
	for in, want := range cases {
		if got := SampleIP(netip.MustParsePrefix(in)).String(); got != want {
			t.Errorf("SampleIP(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestColumn(t *testing.T) {
	if got := Column(netip.MustParsePrefix("10.88.0.0/24"), 3); got != "route:10.88.0.0/24@3" {
		t.Errorf("Column = %q", got)
	}
}
