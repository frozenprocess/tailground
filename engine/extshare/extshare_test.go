package extshare

import (
	"testing"

	"github.com/frozenprocess/tailground/engine/eval"
)

func TestParseRef(t *testing.T) {
	for _, tc := range []struct {
		in                string
		ok                bool
		kind, alias, name string
	}{
		{"group://corp/eng", true, "group", "corp", "eng"},
		{"tag://corp/relay", true, "tag", "corp", "relay"},
		{"tag://corp/relay:22", true, "tag", "corp", "relay"},
		{"group://corp/eng:80,443", true, "group", "corp", "eng"},
		{"group://corp", false, "", "", ""},
		{"group:eng", false, "", "", ""},
		{"tag:relay", false, "", "", ""},
		{"group:///eng", false, "", "", ""},
		// Forms Tailscale accepts.
		{"TAG://corp/relay", true, "tag", "corp", "relay"},
		{"  tag://corp/relay  ", true, "tag", "corp", "relay"},
		{"tag://corp/tag:relay", true, "tag", "corp", "relay"},
		{"tag://corp/tag:relay:443", true, "tag", "corp", "relay"},
		{"group://corp/group:eng", true, "group", "corp", "eng"},
		// Forms Tailscale rejects as "unsupported target".
		{"tag://corp/", false, "", "", ""},
		{"tag://corp\u2215relay", false, "", "", ""},
		{"tag://corp?relay", false, "", "", ""},
	} {
		ref, ok := ParseRef(tc.in)
		if ok != tc.ok {
			t.Errorf("ParseRef(%q) ok=%v want %v", tc.in, ok, tc.ok)
			continue
		}
		if ok && (ref.Kind != tc.kind || ref.Alias != tc.alias || ref.Name != tc.name) {
			t.Errorf("ParseRef(%q) = %+v", tc.in, ref)
		}
	}
}

func TestBuild_ResolveAndDirection(t *testing.T) {
	ext := []eval.ExternalTailnet{
		{Alias: "corp", Nodes: []eval.ExternalNode{
			{Name: "e1", Groups: []string{"eng"}, Tags: []string{"laptop"}},
			{Name: "e2", Groups: []string{"eng", "sales"}},
			{Name: "r1", Tags: []string{"relay"}},
		}},
	}
	cfg := map[string]TailnetConfig{
		"corp": {ExternalID: "x", AllowIncomingConnections: true},
	}
	m, diags := Build(ext, cfg)
	if len(diags) != 0 {
		t.Fatalf("diags: %v", diags)
	}

	ref, _ := ParseRef("group://corp/eng")
	got, ok := m.Resolve(ref)
	if !ok || len(got) != 2 {
		t.Fatalf("group://corp/eng resolved to %d nodes (ok=%v)", len(got), ok)
	}

	ref, _ = ParseRef("tag://corp/relay")
	got, _ = m.Resolve(ref)
	if len(got) != 1 || got[0].Name != "r1" {
		t.Errorf("tag://corp/relay = %+v", got)
	}

	ref, _ = ParseRef("group://ghost/eng")
	if _, ok := m.Resolve(ref); ok {
		t.Errorf("unknown tailnet should be ok=false")
	}

	if len(m.AllNodes()) != 3 {
		t.Errorf("AllNodes = %d, want 3", len(m.AllNodes()))
	}
	if len(m.IncomingNodes()) != 3 {
		t.Errorf("IncomingNodes = %d, want 3 (corp allows incoming)", len(m.IncomingNodes()))
	}
	if tn, _ := m.Lookup("corp"); !tn.Config.AllowIncomingConnections || !tn.Configured {
		t.Errorf("corp config not folded: %+v", tn)
	}
}

// A policy alias with no snapshot tailnet is reported by preflight (once, on
// the externalTailnets key) — Build itself stays quiet and just leaves it out.
func TestBuild_ConfigWithoutSnapshotTailnet(t *testing.T) {
	m, diags := Build(nil, map[string]TailnetConfig{"corp": {}})
	if len(diags) != 0 {
		t.Fatalf("want no diagnostics from Build, got %v", diags)
	}
	if tn, _ := m.Lookup("corp"); tn != nil {
		t.Errorf("corp should not exist without a snapshot tailnet: %+v", tn)
	}
}

func TestIncomingNodes_GatedByAllowIncoming(t *testing.T) {
	ext := []eval.ExternalTailnet{
		{Alias: "yes", Nodes: []eval.ExternalNode{{Name: "a"}}},
		{Alias: "no", Nodes: []eval.ExternalNode{{Name: "b"}}},
	}
	cfg := map[string]TailnetConfig{
		"yes": {AllowIncomingConnections: true},
		"no":  {AllowIncomingConnections: false},
	}
	m, _ := Build(ext, cfg)
	if got := m.IncomingNodes(); len(got) != 1 || got[0].Name != "a" {
		t.Errorf("IncomingNodes = %+v, want just a", got)
	}
	if len(m.AllNodes()) != 2 {
		t.Errorf("AllNodes = %d, want 2", len(m.AllNodes()))
	}
}

// Tailscale matches aliases case-insensitively.
func TestLookup_CaseInsensitive(t *testing.T) {
	m, _ := Build(
		[]eval.ExternalTailnet{{Alias: "Corp", Nodes: []eval.ExternalNode{{Name: "r1", Tags: []string{"relay"}}}}},
		map[string]TailnetConfig{"CORP": {ExternalID: "x", AllowIncomingConnections: true}},
	)
	tn, ok := m.Lookup("corp")
	if !ok || !tn.Configured || tn.Alias != "Corp" {
		t.Fatalf("Lookup(corp) = %+v, %v", tn, ok)
	}
	ref, _ := ParseRef("tag://cOrP/relay")
	if got, ok := m.Resolve(ref); !ok || len(got) != 1 {
		t.Errorf("Resolve(tag://cOrP/relay) = %+v, %v", got, ok)
	}
}

func TestNode_Column(t *testing.T) {
	n := Node{Alias: "corp", Name: "relay"}
	if n.Column() != "ext:corp/relay" {
		t.Errorf("column = %q", n.Column())
	}
}
