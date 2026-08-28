package autogroups

import (
	"reflect"
	"testing"

	"github.com/frozenprocess/tailground/engine/eval"
)

var users = []eval.User{
	{ID: 1, Email: "owner@corp.example", Role: "owner"},
	{ID: 2, Email: "admin@corp.example", Role: "admin"},
	{ID: 3, Email: "admin2@corp.example", Role: "Admin"}, // case-insensitive
	{ID: 4, Email: "net@corp.example", Role: "network-admin"},
	{ID: 5, Email: "it@corp.example", Role: "it-admin"},
	{ID: 6, Email: "audit@corp.example", Role: "auditor"},
	{ID: 7, Email: "plain@corp.example", Role: ""},        // ⇒ member
	{ID: 8, Email: "member@corp.example", Role: "member"}, //
	{ID: 9, Email: "guest@other.example", Shared: true},
}

func TestHandles(t *testing.T) {
	for _, name := range []string{"admin", "owner", "it-admin", "network-admin", "billing-admin", "auditor", "shared"} {
		if !Handles(name) {
			t.Errorf("Handles(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"member", "tagged", "internet", "self", "nonroot", "danger-all", "bogus"} {
		if Handles(name) {
			t.Errorf("Handles(%q) = true, want false (Headscale-native or unknown)", name)
		}
	}
}

func TestResolve_AdminIncludesOwner(t *testing.T) {
	got, ok := Resolve("admin", users)
	if !ok {
		t.Fatal("ok = false")
	}
	want := []string{"admin2@corp.example", "admin@corp.example", "owner@corp.example"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("autogroup:admin = %v, want %v (Admin + Owner)", got, want)
	}
}

func TestResolve_OwnerIsExact(t *testing.T) {
	got, _ := Resolve("owner", users)
	if !reflect.DeepEqual(got, []string{"owner@corp.example"}) {
		t.Errorf("autogroup:owner = %v, want just the owner", got)
	}
}

func TestResolve_AdvancedRolesAreExact(t *testing.T) {
	for token, wantEmail := range map[string]string{
		"network-admin": "net@corp.example",
		"it-admin":      "it@corp.example",
		"auditor":       "audit@corp.example",
	} {
		got, _ := Resolve(token, users)
		if !reflect.DeepEqual(got, []string{wantEmail}) {
			t.Errorf("autogroup:%s = %v, want [%s]", token, got, wantEmail)
		}
	}
}

func TestResolve_Shared(t *testing.T) {
	got, _ := Resolve("shared", users)
	if !reflect.DeepEqual(got, []string{"guest@other.example"}) {
		t.Errorf("autogroup:shared = %v, want the shared user", got)
	}
}

func TestResolve_NoMatch(t *testing.T) {
	got, ok := Resolve("billing-admin", users)
	if !ok || got != nil {
		t.Errorf("got (%v, %v), want (nil, true)", got, ok)
	}
}

func TestResolve_NotHandled(t *testing.T) {
	if got, ok := Resolve("member", users); ok || got != nil {
		t.Errorf("Resolve(member) = (%v, %v), want (nil, false)", got, ok)
	}
}

func TestSrcOnly(t *testing.T) {
	if !SrcOnly("shared") {
		t.Error("shared should be src-only")
	}
	if SrcOnly("admin") {
		t.Error("admin is valid in src and dst")
	}
}
