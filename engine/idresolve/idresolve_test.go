package idresolve

import (
	"reflect"
	"testing"

	"github.com/frozenprocess/tailground/engine/eval"
)

var testUsers = []eval.User{
	{ID: 1, Email: "alice@corp.example"},
	{ID: 2, Email: "bob@corp.example"},
	{ID: 3, Email: "carol@other.example"},
	{ID: 4, Email: "dave@CORP.EXAMPLE"},
	{ID: 5, Email: "malformed-no-at"},
}

func TestIsUserWildcard(t *testing.T) {
	cases := map[string]bool{
		"user:*@corp.example":   true,
		"user:*@sub.corp.co.uk": true,
		"user:*@":               false,
		"user:*@a@b":            false,
		"user:*@corp.*":         false,
		"user:alice@corp":       false,
		"alice@corp.example":    false,
		"group:eng":             false,
		"*":                     false,
	}
	for in, want := range cases {
		if got := IsUserWildcard(in); got != want {
			t.Errorf("IsUserWildcard(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestResolveUserWildcard_Match(t *testing.T) {
	got, ok := ResolveUserWildcard("user:*@corp.example", testUsers)
	if !ok {
		t.Fatal("ok = false for a well-formed wildcard")
	}
	want := []string{"alice@corp.example", "bob@corp.example", "dave@CORP.EXAMPLE"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestResolveUserWildcard_CaseInsensitiveDomain(t *testing.T) {
	got, _ := ResolveUserWildcard("user:*@Corp.Example", testUsers)
	if len(got) != 3 {
		t.Errorf("expected 3 matches regardless of domain case, got %v", got)
	}
}

func TestResolveUserWildcard_NoMatch(t *testing.T) {
	got, ok := ResolveUserWildcard("user:*@nobody.example", testUsers)
	if !ok {
		t.Fatal("ok = false for a well-formed wildcard with no matches")
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestResolveUserWildcard_NotAWildcard(t *testing.T) {
	got, ok := ResolveUserWildcard("alice@corp.example", testUsers)
	if ok || got != nil {
		t.Errorf("got (%v, %v), want (nil, false)", got, ok)
	}
}

func TestResolveUserWildcard_Dedupe(t *testing.T) {
	dupes := []eval.User{
		{ID: 1, Email: "alice@corp.example"},
		{ID: 2, Email: "alice@corp.example"},
	}
	got, _ := ResolveUserWildcard("user:*@corp.example", dupes)
	if !reflect.DeepEqual(got, []string{"alice@corp.example"}) {
		t.Errorf("got %v, want one deduped entry", got)
	}
}
