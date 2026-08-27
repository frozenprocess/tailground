package preflight

import (
	"errors"
	"strings"
	"testing"

	"github.com/frozenprocess/tailground/engine/eval"
)

// autogroup:shared with external nodes sharing in (and no local shared user)
// still matches those nodes, so it must not be reported as matching nothing.
func TestRun_SharedAutogroupWithIncomingNodesIsQuiet(t *testing.T) {
	raw := `{
		"externalTailnets": { "corp": { "externalID": "1", "allowIncomingConnections": true } },
		"acls": [ { "action": "accept", "src": ["autogroup:shared"], "dst": ["*:22"] } ]
	}`
	r := RunWithOptions(raw, Options{
		Users:            []eval.User{{ID: 1, Email: "alice@x.com"}},
		ExternalTailnets: []eval.ExternalTailnet{{Alias: "corp", Nodes: []eval.ExternalNode{{Name: "e1"}}}},
	})
	if len(r.Diagnostics) != 0 {
		t.Errorf("want no diagnostics, got %v", r.Diagnostics)
	}
	if !strings.Contains(r.Policy, "100.127.0.1") {
		t.Errorf("shared node address missing from src:\n%s", r.Policy)
	}
}

// Without incoming nodes the same token really does match nothing — and the
// note says how to fix the snapshot.
func TestRun_SharedAutogroupWithNothingSharedNotes(t *testing.T) {
	raw := `{ "acls": [ { "action": "accept", "src": ["autogroup:shared"], "dst": ["*:22"] } ] }`
	r := RunWithOptions(raw, Options{Users: []eval.User{{ID: 1, Email: "alice@x.com"}}})
	if got := strings.Join(diagCodes(r), ","); got != "info/snapshot" {
		t.Fatalf("diagnostics = %s, want info/snapshot", got)
	}
	if !strings.Contains(r.Diagnostics[0].Message, "matches nothing") {
		t.Errorf("message should say the rule matches nothing: %q", r.Diagnostics[0].Message)
	}
}

// A configured alias the snapshot lacks is reported once, on the
// externalTailnets key — not again at every reference to it.
func TestRun_MissingExternalTailnetReportedOnce(t *testing.T) {
	raw := `{
		"externalTailnets": { "corp": { "externalID": "1", "allowIncomingConnections": true } },
		"grants": [
			{ "src": ["group://corp/eng"], "dst": ["*"], "ip": ["tcp:22"] },
			{ "src": ["tag://corp/relay"], "dst": ["*"], "ip": ["tcp:443"] }
		]
	}`
	r := Run(raw)
	if got := strings.Join(diagCodes(r), ","); got != "warning/snapshot" {
		t.Fatalf("diagnostics = %s, want one warning/snapshot", got)
	}
	if r.Diagnostics[0].Pointer != "2:3" {
		t.Errorf("pointer = %q, want the externalTailnets key (2:3)", r.Diagnostics[0].Pointer)
	}
}

// An alias externalTailnets doesn't list is fatal, as in Tailscale ("external
// tailnet not found"), reported at the reference.
func TestRun_UnknownExternalAliasIsFatal(t *testing.T) {
	raw := `{ "grants": [ { "src": ["group://ghost/eng"], "dst": ["*"], "ip": ["tcp:22"] } ] }`
	r := Run(raw)
	if !r.Fatal {
		t.Fatal("want fatal")
	}
	if got := strings.Join(diagCodes(r), ","); got != "error/policy" {
		t.Fatalf("diagnostics = %s, want error/policy", got)
	}
	if !strings.Contains(r.Diagnostics[0].Message, `external tailnet not found: "ghost"`) {
		t.Errorf("message = %q", r.Diagnostics[0].Message)
	}
	if r.Diagnostics[0].Pointer == "" {
		t.Error("reference error has no position")
	}
}

func TestRun_SyntaxErrorIsPositioned(t *testing.T) {
	r := Run("{\n  \"acls\": [\n    {\"action\": \"accept\" \"src\": []}\n  ]\n}")
	if !r.Fatal || len(r.Diagnostics) != 1 {
		t.Fatalf("want one fatal diagnostic, got %v", r.Diagnostics)
	}
	if got := r.Diagnostics[0].Pointer; got != "3:25" {
		t.Errorf("pointer = %q, want 3:25", got)
	}
}

func TestHujsonErrPointer_NoPosition(t *testing.T) {
	if got := hujsonErrPointer(errors.New("unexpected EOF")); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestLocateCompileError(t *testing.T) {
	raw := `{
  "groups": { "group:eng": ["a@x.com"] },
  "grants": [
    { "src": ["group:nope"], "dst": ["*"], "ip": ["tcp:99999"] }
  ],
  "acls": [ { "action": "accept", "src": ["*"], "dst": ["nohost:22"] } ]
}`
	for _, tc := range []struct{ msg, want string }{
		// JSON-pointer form
		{`json: cannot unmarshal JSON string into Go v2.ProtocolPort within "/grants/0/ip/0": port number out of range`, "4:51"},
		// pointer deeper than the raw text (preflight rewrote the array) walks up
		{`within "/grants/0/ip/7": bad`, "4:50"},
		// quoted-token form
		{`group not defined in policy: "group:nope"`, "4:15"},
		// a host token matches its "host:ports" use
		{`host not defined in policy: "nohost"`, "6:57"},
		{`something with no location`, ""},
	} {
		if got := LocateCompileError(raw, tc.msg); got != tc.want {
			t.Errorf("LocateCompileError(%q) = %q, want %q", tc.msg, got, tc.want)
		}
	}
}
