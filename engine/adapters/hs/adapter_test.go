package hs

import (
	"fmt"
	"strings"
	"testing"

	"github.com/frozenprocess/tailground/engine/eval"
)

// threeNode is a user device + two tagged devices, addresses in the CGNAT / ULA
// ranges Tailscale uses.
func threeNode() eval.Snapshot {
	return eval.Snapshot{
		Users: []eval.User{{ID: 1, Email: "alice@example.com"}},
		Devices: []eval.Device{
			{ID: 1, Name: "alice-laptop", User: "alice@example.com", IPv4: "100.64.0.1", IPv6: "fd7a:115c:a1e0::1"},
			{ID: 2, Name: "web-1", Tags: []string{"tag:web"}, IPv4: "100.64.0.2", IPv6: "fd7a:115c:a1e0::2"},
			{ID: 3, Name: "db-1", Tags: []string{"tag:db"}, IPv4: "100.64.0.3", IPv6: "fd7a:115c:a1e0::3"},
		},
	}
}

func TestEvaluate_TagBasedACLs(t *testing.T) {
	const policy = `
{
  "tagOwners": {
    "tag:web": ["alice@example.com"],
    "tag:db":  ["alice@example.com"]
  },
  "acls": [
    { "action": "accept", "src": ["alice@example.com"], "dst": ["tag:web:443"] },
    { "action": "accept", "src": ["tag:web"],           "dst": ["tag:db:5432"] }
  ]
}`

	res := New().Evaluate(eval.Request{
		Policy:   policy,
		Snapshot: threeNode(),
		Probes: []eval.Probe{
			{Name: "https", Proto: "tcp", Port: 443},
			{Name: "pg", Proto: "tcp", Port: 5432},
			{Name: "ssh", Proto: "tcp", Port: 22},
		},
	})

	for _, d := range res.Diagnostics {
		if d.Severity == "error" {
			t.Fatalf("unexpected error diagnostic: %+v", d)
		}
	}

	want := map[string]map[string]string{
		"https": {"1->2": "allow", "1->3": "deny", "2->1": "deny", "2->3": "deny", "3->1": "deny", "3->2": "deny"},
		"pg":    {"1->2": "deny", "1->3": "deny", "2->1": "deny", "2->3": "allow", "3->1": "deny", "3->2": "deny"},
		"ssh":   {"1->2": "deny", "1->3": "deny", "2->1": "deny", "2->3": "deny", "3->1": "deny", "3->2": "deny"},
	}

	for probe, cells := range want {
		for pair, exp := range cells {
			if got := res.Matrices[probe][pair]; got != exp {
				t.Errorf("matrix[%s][%s] = %q, want %q", probe, pair, got, exp)
			}
		}
	}
}

func TestEvaluate_SSHMatrix(t *testing.T) {
	snap := eval.Snapshot{
		Users: []eval.User{{ID: 1, Email: "alice@example.com"}, {ID: 2, Email: "bob@example.com"}},
		Devices: []eval.Device{
			{ID: 1, Name: "alice-laptop", User: "alice@example.com", IPv4: "100.64.0.1"},
			{ID: 2, Name: "bob-laptop", User: "bob@example.com", IPv4: "100.64.0.2"},
			{ID: 3, Name: "prod", Tags: []string{"tag:prod"}, IPv4: "100.64.0.3"},
		},
	}
	const policy = `{
		"tagOwners": { "tag:prod": ["alice@example.com"] },
		"groups": { "group:ops": ["bob@example.com"] },
		"ssh": [
			{ "action": "accept", "src": ["alice@example.com"], "dst": ["tag:prod"], "users": ["ubuntu", "root"] },
			{ "action": "check",  "src": ["group:ops"],         "dst": ["tag:prod"], "users": ["autogroup:nonroot"] }
		]
	}`

	res := New().Evaluate(eval.Request{Policy: policy, Snapshot: snap, SSHUsers: []string{"root", "ubuntu"}})

	want := map[string]string{
		"root@1->3":   "accept",
		"ubuntu@1->3": "accept",
		"root@2->3":   "deny", // root is not autogroup:nonroot
		"ubuntu@2->3": "check",
	}
	for k, exp := range want {
		if got := res.SSH[k]; got != exp {
			t.Errorf("ssh[%s] = %q, want %q", k, got, exp)
		}
	}
	// A policy with no ssh block populates nothing.
	noSSH := New().Evaluate(eval.Request{Policy: `{}`, Snapshot: snap})
	if noSSH.SSH != nil {
		t.Errorf("expected nil SSH map for policy without ssh block, got %v", noSSH.SSH)
	}
}

func TestEvaluate_SSHPostureGate(t *testing.T) {
	// ssh[0] is gated on node:os == 'macos'; ssh[1] is ungated. node-a is linux,
	// so it fails ssh[0]'s gate. Real Tailscale drops such a source from the
	// rule's principals at compile time, so evaluation falls through to ssh[1].
	const policy = `{
		"postures": { "posture:mac": ["node:os == 'macos'"] },
		"tagOwners": { "tag:a": ["alice@example.com"], "tag:b": ["alice@example.com"] },
		"ssh": [
			{ "action": "accept", "src": ["tag:a"], "srcPosture": ["posture:mac"], "dst": ["tag:b"], "users": ["root"] },
			{ "action": "check",  "src": ["tag:a"], "dst": ["tag:b"], "users": ["root"] }
		]
	}`
	snap := eval.Snapshot{
		Users: []eval.User{{ID: 1, Email: "alice@example.com"}},
		Devices: []eval.Device{
			{ID: 1, Name: "node-a", Tags: []string{"tag:a"}, IPv4: "100.64.0.1", IPv6: "fd7a:115c:a1e0::1", OS: "linux"},
			{ID: 2, Name: "node-b", Tags: []string{"tag:b"}, IPv4: "100.64.0.2", IPv6: "fd7a:115c:a1e0::2", OS: "linux"},
		},
	}

	res := New().Evaluate(eval.Request{Policy: policy, Snapshot: snap, SSHUsers: []string{"root"}})

	if res.Posture == nil {
		t.Fatal("expected a posture report")
	}
	if len(res.Posture.Gates) != 1 || res.Posture.Gates[0].Rule != "ssh[0]" {
		t.Fatalf("expected one gate on ssh[0], got %+v", res.Posture.Gates)
	}
	// linux node-a fails ssh[0]'s mac gate → falls through to the ungated check.
	if got := res.SSH["root@1->2"]; got != eval.SSHCheck {
		t.Errorf("linux src should fall through ssh[0] to the ungated check: got %q", got)
	}

	// Flip node-a to macos: ssh[0] now matches and its accept wins over ssh[1].
	snap.Devices[0].OS = "macos"
	res2 := New().Evaluate(eval.Request{Policy: policy, Snapshot: snap, SSHUsers: []string{"root"}})
	if got := res2.SSH["root@1->2"]; got != eval.SSHAccept {
		t.Errorf("macos src should match ssh[0] accept: got %q", got)
	}

	// A gate miss with no fallback rule is a plain deny.
	const soloGated = `{
		"postures": { "posture:mac": ["node:os == 'macos'"] },
		"tagOwners": { "tag:a": ["alice@example.com"], "tag:b": ["alice@example.com"] },
		"ssh": [
			{ "action": "accept", "src": ["tag:a"], "srcPosture": ["posture:mac"], "dst": ["tag:b"], "users": ["root"] }
		]
	}`
	snap.Devices[0].OS = "linux"
	res3 := New().Evaluate(eval.Request{Policy: soloGated, Snapshot: snap, SSHUsers: []string{"root"}})
	if got := res3.SSH["root@1->2"]; got != eval.SSHDeny {
		t.Errorf("gate miss with no fallback should deny: got %q", got)
	}

	// defaultSrcPosture gates an ssh rule that carries no srcPosture of its own.
	const sshDefault = `{
		"postures": { "posture:mac": ["node:os == 'macos'"] },
		"defaultSrcPosture": ["posture:mac"],
		"tagOwners": { "tag:a": ["alice@example.com"], "tag:b": ["alice@example.com"] },
		"ssh": [
			{ "action": "accept", "src": ["tag:a"], "dst": ["tag:b"], "users": ["root"] }
		]
	}`
	snap.Devices[0].OS = "linux"
	if got := New().Evaluate(eval.Request{Policy: sshDefault, Snapshot: snap, SSHUsers: []string{"root"}}).SSH["root@1->2"]; got != eval.SSHDeny {
		t.Errorf("linux src should fail the default posture:mac gate on ssh[0]: got %q", got)
	}
	snap.Devices[0].OS = "macos"
	if got := New().Evaluate(eval.Request{Policy: sshDefault, Snapshot: snap, SSHUsers: []string{"root"}}).SSH["root@1->2"]; got != eval.SSHAccept {
		t.Errorf("macos src should pass the default posture:mac gate on ssh[0]: got %q", got)
	}
}

func TestEvaluate_PostureGate(t *testing.T) {
	const policy = `{
		"postures": {
			"posture:corp": ["node:os == 'macos'", "node:tsVersion >= '1.80'"]
		},
		"groups": { "group:admin": ["alice@example.com"] },
		"tagOwners": { "tag:prod": ["group:admin"] },
		"acls": [
			{ "action": "accept", "src": ["group:admin"], "srcPosture": ["posture:corp"], "dst": ["tag:prod:22"] }
		]
	}`
	snap := eval.Snapshot{
		Users: []eval.User{{ID: 1, Email: "alice@example.com"}},
		Devices: []eval.Device{
			{ID: 1, Name: "mac-ok", User: "alice@example.com", IPv4: "100.64.0.1", OS: "macos", Posture: map[string]string{"tsVersion": "1.82.0"}},
			{ID: 2, Name: "mac-old", User: "alice@example.com", IPv4: "100.64.0.2", OS: "macos", Posture: map[string]string{"tsVersion": "1.70.0"}},
			{ID: 3, Name: "prod", Tags: []string{"tag:prod"}, IPv4: "100.64.0.3"},
		},
	}
	probe := []eval.Probe{{Name: "ssh", Proto: "tcp", Port: 22}}

	res := New().Evaluate(eval.Request{Policy: policy, Snapshot: snap, Probes: probe})

	if res.Posture == nil {
		t.Fatal("expected a posture report")
	}
	if got := res.Matrices["ssh"]["1->3"]; got != "allow" {
		t.Errorf("up-to-date mac should reach prod: got %q", got)
	}
	if got := res.Matrices["ssh"]["2->3"]; got != "deny" {
		t.Errorf("out-of-date mac should be denied: got %q", got)
	}
	if reason := res.Posture.Denials["2->3"]; reason == "" {
		t.Errorf("expected a posture denial reason for 2->3, got none")
	}
	if len(res.Posture.Gates) != 1 || res.Posture.Gates[0].Rule != "acls[0]" {
		t.Errorf("expected one gate on acls[0], got %+v", res.Posture.Gates)
	}

	// Raising the version in the snapshot flips 2->3 back to allow.
	snap.Devices[1].Posture["tsVersion"] = "1.85.0"
	res2 := New().Evaluate(eval.Request{Policy: policy, Snapshot: snap, Probes: probe})
	if got := res2.Matrices["ssh"]["2->3"]; got != "allow" {
		t.Errorf("after version bump, 2->3 should be allow: got %q", got)
	}
	if _, denied := res2.Posture.Denials["2->3"]; denied {
		t.Errorf("2->3 should no longer be a posture denial")
	}
}

func TestEvaluate_DefaultSrcPosture(t *testing.T) {
	// acls[0] has no srcPosture → inherits defaultSrcPosture (posture:corp).
	// acls[1] has its own srcPosture (posture:linux) → replaces the default.
	const policy = `{
		"postures": {
			"posture:corp": ["node:os == 'macos'"],
			"posture:linux": ["node:os == 'linux'"]
		},
		"defaultSrcPosture": ["posture:corp"],
		"groups": { "group:admin": ["alice@example.com"] },
		"tagOwners": { "tag:prod": ["group:admin"] },
		"acls": [
			{ "action": "accept", "src": ["group:admin"], "dst": ["tag:prod:22"] },
			{ "action": "accept", "src": ["group:admin"], "srcPosture": ["posture:linux"], "dst": ["tag:prod:443"] }
		]
	}`
	snap := eval.Snapshot{
		Users: []eval.User{{ID: 1, Email: "alice@example.com"}},
		Devices: []eval.Device{
			{ID: 1, Name: "mac", User: "alice@example.com", IPv4: "100.64.0.1", OS: "macos"},
			{ID: 2, Name: "linux", User: "alice@example.com", IPv4: "100.64.0.2", OS: "linux"},
			{ID: 3, Name: "prod", Tags: []string{"tag:prod"}, IPv4: "100.64.0.3"},
		},
	}
	probes := []eval.Probe{{Name: "ssh", Proto: "tcp", Port: 22}, {Name: "https", Proto: "tcp", Port: 443}}

	res := New().Evaluate(eval.Request{Policy: policy, Snapshot: snap, Probes: probes})
	if res.Posture == nil {
		t.Fatal("expected a posture report")
	}
	// Default applies to acls[0]: only the mac passes posture:corp.
	if got := res.Matrices["ssh"]["1->3"]; got != "allow" {
		t.Errorf("mac should reach prod:22 (default posture:corp): got %q", got)
	}
	if got := res.Matrices["ssh"]["2->3"]; got != "deny" {
		t.Errorf("linux should be denied prod:22 by default posture:corp: got %q", got)
	}
	// acls[1] replaces the default with posture:linux: only the linux box passes.
	if got := res.Matrices["https"]["2->3"]; got != "allow" {
		t.Errorf("linux should reach prod:443 (own posture:linux): got %q", got)
	}
	if got := res.Matrices["https"]["1->3"]; got != "deny" {
		t.Errorf("mac should be denied prod:443 (posture:linux replaces default): got %q", got)
	}
	var gotDefault, gotOwn bool
	for _, g := range res.Posture.Gates {
		if g.Rule == "acls[0]" && g.Default && len(g.Postures) == 1 && g.Postures[0] == "posture:corp" {
			gotDefault = true
		}
		if g.Rule == "acls[1]" && !g.Default && len(g.Postures) == 1 && g.Postures[0] == "posture:linux" {
			gotOwn = true
		}
	}
	if !gotDefault || !gotOwn {
		t.Errorf("gates = %+v (want acls[0] default posture:corp, acls[1] own posture:linux)", res.Posture.Gates)
	}
	if len(res.Posture.DefaultSrcPosture) != 1 || res.Posture.DefaultSrcPosture[0] != "posture:corp" {
		t.Errorf("DefaultSrcPosture = %v", res.Posture.DefaultSrcPosture)
	}
}

func TestEvaluate_TestsGrading(t *testing.T) {
	snap := eval.Snapshot{
		Users: []eval.User{{ID: 1, Email: "alice@example.com"}},
		Devices: []eval.Device{
			{ID: 1, Name: "alice-laptop", User: "alice@example.com", IPv4: "100.64.0.1"},
			{ID: 2, Name: "web-1", Tags: []string{"tag:web"}, IPv4: "100.64.0.2"},
			{ID: 3, Name: "db-1", Tags: []string{"tag:db"}, IPv4: "100.64.0.3"},
		},
	}
	const withTests = `{
		"tagOwners": { "tag:web": ["alice@example.com"], "tag:db": ["alice@example.com"] },
		"acls": [ %s ],
		"tests": [
			{ "src": "alice@example.com", "accept": ["tag:web:443"], "deny": ["tag:db:5432"] }
		]
	}`

	// Correct policy — every assertion passes.
	pass := New().Evaluate(eval.Request{
		Snapshot: snap,
		Policy:   fmt.Sprintf(withTests, `{ "action": "accept", "src": ["alice@example.com"], "dst": ["tag:web:443"] }`),
	})
	if pass.Tests == nil || pass.Tests.ACL == nil {
		t.Fatal("expected an ACL test report")
	}
	if !pass.Tests.ACL.Passed {
		t.Errorf("expected all tests to pass, got %+v", pass.Tests.ACL)
	}
	for _, c := range pass.Tests.ACL.Cases {
		if !c.Pass {
			t.Errorf("case %q should pass", c.Label)
		}
	}

	// Wrong port — the accept assertion fails, the deny still holds.
	fail := New().Evaluate(eval.Request{
		Snapshot: snap,
		Policy:   fmt.Sprintf(withTests, `{ "action": "accept", "src": ["alice@example.com"], "dst": ["tag:web:22"] }`),
	})
	if fail.Tests.ACL.Passed {
		t.Errorf("expected the block to fail")
	}
	byLabel := map[string]eval.TestCase{}
	for _, c := range fail.Tests.ACL.Cases {
		byLabel[c.Label] = c
	}
	if c := byLabel["alice@example.com → tag:web:443"]; c.Pass || c.Detail == "" {
		t.Errorf("web:443 accept should fail with a detail, got %+v", c)
	}
	if c := byLabel["alice@example.com → tag:db:5432"]; !c.Pass {
		t.Errorf("db:5432 deny should still pass, got %+v", c)
	}

	// No tests block — no report.
	none := New().Evaluate(eval.Request{Snapshot: snap, Policy: `{}`})
	if none.Tests != nil {
		t.Errorf("policy without tests must not carry a test report")
	}
}

func TestEvaluate_NoPostureNoReport(t *testing.T) {
	res := New().Evaluate(eval.Request{
		Policy:   `{ "acls": [ { "action": "accept", "src": ["*"], "dst": ["*:*"] } ] }`,
		Snapshot: threeNode(),
	})
	if res.Posture != nil {
		t.Errorf("policy without postures must not carry a posture report")
	}
}

func TestEvaluate_EmptyPolicyIsAllowAll(t *testing.T) {
	res := New().Evaluate(eval.Request{
		Policy:   `{}`,
		Snapshot: threeNode(),
		Probes:   []eval.Probe{{Name: "tcp", Proto: "tcp", Port: 80}},
	})
	for _, pair := range []string{"1->2", "2->1", "1->3", "3->2"} {
		if got := res.Matrices["tcp"][pair]; got != "allow" {
			t.Errorf("empty policy: matrix[tcp][%s] = %q, want allow", pair, got)
		}
	}
}

// An external tailnet with no shared nodes must still produce a view whose
// Nodes is a non-nil empty slice — the web consumers (ExternalSharePanel, the
// graph model) iterate it directly and a JSON `null` crashes them.
func TestEvaluate_ExternalTailnetWithoutNodes(t *testing.T) {
	snap := threeNode()
	snap.ExternalTailnets = []eval.ExternalTailnet{{Alias: "partner"}}
	res := New().Evaluate(eval.Request{
		Policy:   `{ "acls": [ { "action": "accept", "src": ["*"], "dst": ["*:*"] } ] }`,
		Snapshot: snap,
	})
	if res.ExternalShare == nil || len(res.ExternalShare.Tailnets) != 1 {
		t.Fatalf("want one external tailnet view, got %+v", res.ExternalShare)
	}
	if res.ExternalShare.Tailnets[0].Nodes == nil {
		t.Errorf("Nodes must be a non-nil empty slice, got nil")
	}
}

func TestEvaluate_ParseErrorSurfaces(t *testing.T) {
	cases := map[string]string{
		"undefined tag":   `{ "acls": [ { "action": "accept", "src": ["*"], "dst": ["tag:ghost:*"] } ] }`,
		"undefined group": `{ "acls": [ { "action": "accept", "src": ["group:nope"], "dst": ["*:*"] } ] }`,
		"unknown field":   `{ "acls": [ { "boguskey": 1 } ] }`,
		"malformed json":  `{ "acls": [ `,
	}
	for name, policy := range cases {
		t.Run(name, func(t *testing.T) {
			res := New().Evaluate(eval.Request{Policy: policy, Snapshot: threeNode()})
			var gotErr bool
			for _, d := range res.Diagnostics {
				if d.Severity == "error" && d.Code == "parse" {
					gotErr = true
				}
			}
			if !gotErr {
				t.Fatalf("expected a parse error diagnostic, got %+v", res.Diagnostics)
			}
		})
	}
}

func TestCollapseDeviceDiags(t *testing.T) {
	in := []eval.Diagnostic{
		{Severity: "warning", Code: "internal", Message: "SSH policy for web-1: bad user"},
		{Severity: "warning", Code: "snapshot", Message: `device "db-1" has neither a user nor tags`},
		{Severity: "warning", Code: "internal", Message: "SSH policy for web-2: bad user"},
		{Severity: "warning", Code: "internal", Message: "SSH policy for web-3: other error"},
		{Severity: "warning", Code: "internal", Message: "SSH policy for db-1: bad user"},
	}
	got := collapseDeviceDiags(in)
	want := []string{
		"SSH policy for web-1, web-2, db-1: bad user",
		`device "db-1" has neither a user nor tags`,
		"SSH policy for web-3: other error",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d diagnostics, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Message != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i].Message, want[i])
		}
	}
}

// A Headscale compile error is shown at its location in the policy.
func TestEvaluate_CompileErrorIsPositioned(t *testing.T) {
	res := New().Evaluate(eval.Request{
		Policy:   "{\n  \"grants\": [\n    {\"src\": [\"group:nope\"], \"dst\": [\"*\"], \"ip\": [\"*\"]}\n  ]\n}",
		Snapshot: eval.Snapshot{Users: []eval.User{{ID: 1, Email: "a@x.com"}}},
	})
	if len(res.Diagnostics) != 1 || res.Diagnostics[0].Code != "parse" {
		t.Fatalf("want one parse error, got %+v", res.Diagnostics)
	}
	if got := res.Diagnostics[0].Pointer; got != "3:14" {
		t.Errorf("pointer = %q, want 3:14", got)
	}
}

func TestLocateUnhomed(t *testing.T) {
	raw := "{\n  \"grants\": [\n    {\"src\": [\"*\"], \"dst\": [\"*\"], \"ip\": [\"*\"]},\n    {\"src\": [\"tag:x\"], \"dst\": [\"*\"], \"ip\": [\"*\"], \"srcPosture\": [\"posture:p\"]}\n  ]\n}"
	ds := []eval.Diagnostic{
		// token named in the error → the token
		{Severity: "warning", Code: "parse", Message: `posture-gated grants[1]: tag not found: "tag:x"`},
		// sub-policy JSON pointer is meaningless → the rule itself
		{Severity: "warning", Code: "parse", Message: `posture-gated grants[1]: json: bad within "/grants/0/ip/0": nope`},
		// snapshot items keep their target and stay unpositioned
		{Severity: "warning", Code: "snapshot", Target: "device:a", Message: `device "a" has neither a user nor tags`},
	}
	locateUnhomed(raw, ds)
	for i, want := range []string{"4:14", "4:5", ""} {
		if ds[i].Pointer != want {
			t.Errorf("[%d] pointer = %q, want %q", i, ds[i].Pointer, want)
		}
	}
}

// Aperture grants use tailscale.com/cap/aperture, which the Tailscale control
// plane accepts but upstream Headscale does not allowlist. See
// engine/vendor-trim/capabilities.patch.
func TestEvaluate_ApertureCapability(t *testing.T) {
	const policy = `
{
  "tagOwners": { "tag:web": ["alice@example.com"] },
  "grants": [
    { "src": ["alice@example.com"], "dst": ["tag:web"],
      "app": { "tailscale.com/cap/aperture": [{ "role": "user" }] } }
  ]
}`

	res := New().Evaluate(eval.Request{Policy: policy, Snapshot: threeNode()})

	for _, d := range res.Diagnostics {
		if d.Severity == "error" {
			t.Fatalf("unexpected error diagnostic: %+v", d)
		}
	}
	if got := fmt.Sprint(res.Grants["1->2"]); got != "[tailscale.com/cap/aperture]" {
		t.Errorf("grants[1->2] = %s, want [tailscale.com/cap/aperture]", got)
	}
}

func TestEvaluate_UndefinedPostureWarns(t *testing.T) {
	// acls[0] and ssh[0] each name a posture the policy never defines; both are
	// gated and reported, and each gets one "not defined" warning.
	const policy = `{
		"postures": { "posture:mac": ["node:os == 'macos'"] },
		"tagOwners": { "tag:a": ["alice@example.com"], "tag:b": ["alice@example.com"] },
		"acls": [
			{ "action": "accept", "src": ["tag:a"], "srcPosture": ["posture:ghost"], "dst": ["tag:b:22"] }
		],
		"ssh": [
			{ "action": "accept", "src": ["tag:a"], "srcPosture": ["posture:ghost"], "dst": ["tag:b"], "users": ["root"] }
		]
	}`
	snap := eval.Snapshot{
		Users: []eval.User{{ID: 1, Email: "alice@example.com"}},
		Devices: []eval.Device{
			{ID: 1, Name: "node-a", Tags: []string{"tag:a"}, IPv4: "100.64.0.1", OS: "linux"},
			{ID: 2, Name: "node-b", Tags: []string{"tag:b"}, IPv4: "100.64.0.2", OS: "linux"},
		},
	}

	res := New().Evaluate(eval.Request{Policy: policy, Snapshot: snap, SSHUsers: []string{"root"}})

	if res.Posture == nil {
		t.Fatal("expected a posture report")
	}
	var rules []string
	for _, g := range res.Posture.Gates {
		rules = append(rules, g.Rule)
	}
	if got := strings.Join(rules, ","); got != "acls[0],ssh[0]" {
		t.Errorf("gates = %s, want acls[0],ssh[0]", got)
	}
	want := []string{
		`acls[0] references posture "posture:ghost" which is not defined`,
		`ssh[0] references posture "posture:ghost" which is not defined`,
	}
	if got := strings.Join(res.Posture.Warnings, "\n"); got != strings.Join(want, "\n") {
		t.Errorf("warnings =\n%s\nwant\n%s", got, strings.Join(want, "\n"))
	}
}
