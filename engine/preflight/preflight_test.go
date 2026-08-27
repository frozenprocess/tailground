package preflight

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/frozenprocess/tailground/engine/eval"
)

func diagCodes(r Result) (codes []string) {
	for _, d := range r.Diagnostics {
		codes = append(codes, d.Severity+"/"+d.Code)
	}
	return
}

func TestRun_StripsUnmodeledTopLevelKeys(t *testing.T) {
	raw := `{
		"ipsets": { "ipset:x": ["10.0.0.0/8"] },
		"derpMap": { "regions": {} },
		"disableIPv4": true,
		"externalTailnets": { "corp": { "externalID": "1234" } },
		"weirdKey": 1,
		"acls": [ { "action": "accept", "src": ["*"], "dst": ["*:*"] } ]
	}`
	r := Run(raw)
	if r.Fatal {
		t.Fatalf("unexpected fatal: %+v", r.Diagnostics)
	}

	var stripped map[string]json.RawMessage
	if err := json.Unmarshal([]byte(r.Policy), &stripped); err != nil {
		t.Fatalf("stripped policy is not valid JSON: %v", err)
	}
	for _, k := range []string{"ipsets", "derpMap", "disableIPv4", "externalTailnets", "weirdKey"} {
		if _, ok := stripped[k]; ok {
			t.Errorf("key %q survived preflight", k)
		}
	}
	if _, ok := stripped["acls"]; !ok {
		t.Error("acls was dropped")
	}
	// Stripping a handled or inert key is silent. What's left is actionable:
	// weirdKey (policy) and externalTailnets configuring "corp", which the
	// snapshot lacks (snapshot, pointed at the externalTailnets key).
	if got := strings.Join(diagCodes(r), ","); got != "warning/snapshot,warning/policy" {
		t.Errorf("diagnostics = %s, want warning/snapshot,warning/policy", got)
	}
	for _, d := range r.Diagnostics {
		if d.Pointer == "" {
			t.Errorf("diagnostic has no position: %+v", d)
		}
	}
}

func TestRun_StripsSSHRecorder(t *testing.T) {
	raw := `{
		"ssh": [
			{
				"action": "accept",
				"src": ["autogroup:member"],
				"dst": ["autogroup:self"],
				"users": ["autogroup:nonroot"],
				"recorder": ["tag:recorder"],
				"enforceRecorder": true
			}
		]
	}`
	r := Run(raw)
	if r.Fatal {
		t.Fatalf("recorder keys should not be fatal: %v", diagCodes(r))
	}

	var doc struct {
		SSH []map[string]json.RawMessage `json:"ssh"`
	}
	if err := json.Unmarshal([]byte(r.Policy), &doc); err != nil {
		t.Fatalf("stripped policy is not valid JSON: %v", err)
	}
	if len(doc.SSH) != 1 {
		t.Fatalf("want 1 ssh rule, got %d", len(doc.SSH))
	}
	for _, k := range []string{"recorder", "enforceRecorder"} {
		if _, ok := doc.SSH[0][k]; ok {
			t.Errorf("ssh[0].%s survived preflight", k)
		}
	}
	if _, ok := doc.SSH[0]["action"]; !ok {
		t.Error("ssh[0].action was dropped")
	}

	// Session recording has no bearing on reachability: dropped silently.
	if len(r.Diagnostics) != 0 {
		t.Errorf("want no diagnostics, got %v", diagCodes(r))
	}
}

func TestRun_ResolvesExternalRefs(t *testing.T) {
	raw := `{
		"externalTailnets": {
			"Corp": { "externalID": "1234", "allowIncomingConnections": true }
		},
		"grants": [
			{ "src": ["group://corp/eng"], "dst": ["tag:srv"], "ip": ["tcp:22"] },
			{ "src": [" TAG://corp/tag:relay:443 "], "dst": ["tag:srv"], "ip": ["tcp:443"] }
		]
	}`
	opt := Options{ExternalTailnets: []eval.ExternalTailnet{
		{Alias: "corp", Nodes: []eval.ExternalNode{
			{Name: "e1", Groups: []string{"eng"}},
			{Name: "r1", Tags: []string{"relay"}},
		}},
	}}
	r := RunWithOptions(raw, opt)
	if r.Fatal {
		t.Fatalf("unexpected fatal: %v", r.Diagnostics)
	}

	var doc struct {
		Grants []struct {
			Src []string `json:"src"`
		} `json:"grants"`
	}
	if err := json.Unmarshal([]byte(r.Policy), &doc); err != nil {
		t.Fatalf("stripped policy is not valid JSON: %v", err)
	}
	// grants[0].src: group://corp/eng -> e1's v4 + v6. The alias matches
	// "Corp" case-insensitively, as in Tailscale.
	if got := strings.Join(doc.Grants[0].Src, ","); got != "100.127.0.1,fd7a:115c:a1e0:ffff::1" {
		t.Errorf("grants[0].src = %q, want e1's synthetic addresses", got)
	}
	// grants[1].src: Tailscale also accepts an upper-case scheme, surrounding
	// whitespace, a repeated "tag:" prefix and ports -> r1.
	if got := strings.Join(doc.Grants[1].Src, ","); got != "100.127.0.2,fd7a:115c:a1e0:ffff::2" {
		t.Errorf("grants[1].src = %q, want r1's synthetic addresses", got)
	}

	if r.ExternalShare == nil || len(r.ExternalShare.Tailnets) != 1 {
		t.Fatalf("ExternalShare not populated: %+v", r.ExternalShare)
	}
	if tn := r.ExternalShare.Tailnets[0]; !tn.Configured || !tn.AllowIncomingConnections || len(tn.Nodes) != 2 {
		t.Errorf("tailnet view = %+v", tn)
	}
}

// Every policy here is one Tailscale rejects outright, so preflight must be
// fatal with an error that leads with Tailscale's own wording.
func TestRun_ExternalRefRejections(t *testing.T) {
	const ext = `"externalTailnets": { "corp": { "externalID": "1", "allowIncomingConnections": true } },`
	for _, tc := range []struct {
		name, policy, want string
	}{
		{"acls src", `{` + ext + ` "acls": [ { "action": "accept", "src": ["group://corp/eng"], "dst": ["tag:srv:22"] } ] }`,
			`acls[0].src "group://corp/eng": unsupported target`},
		{"grant dst", `{` + ext + ` "grants": [ { "src": ["tag:srv"], "dst": ["tag://corp/relay"], "ip": ["*"] } ] }`,
			`grants[0].dst "tag://corp/relay": unsupported target`},
		{"grant via", `{` + ext + ` "grants": [ { "src": ["tag:srv"], "dst": ["10.0.0.0/8"], "ip": ["*"], "via": ["tag://corp/relay"] } ] }`,
			`grants[0].via "tag://corp/relay": unsupported target`},
		{"ssh src", `{` + ext + ` "ssh": [ { "action": "accept", "src": ["tag://corp/relay"], "dst": ["tag:srv"], "users": ["root"] } ] }`,
			`ssh[0].src "tag://corp/relay": external references aren't supported in ssh rules`},
		{"inside a group", `{` + ext + ` "groups": { "group:x": ["tag://corp/relay"] } }`,
			`groups["group:x"][0] "tag://corp/relay": unsupported target`},
		{"inside an ipset", `{` + ext + ` "ipsets": { "ipset:x": ["tag://corp/relay"] } }`,
			`ipsets["ipset:x"][0] "tag://corp/relay": unsupported target`},
		{"other scheme", `{` + ext + ` "grants": [ { "src": ["svc://corp/x"], "dst": ["tag:srv"], "ip": ["*"] } ] }`,
			`grants[0].src "svc://corp/x": unsupported target`},
		{"no tag name", `{` + ext + ` "grants": [ { "src": ["tag://corp"], "dst": ["tag:srv"], "ip": ["*"] } ] }`,
			`grants[0].src "tag://corp": unsupported target`},
		{"lookalike slash", `{` + ext + ` "grants": [ { "src": ["tag://corp\u2215relay"], "dst": ["tag:srv"], "ip": ["*"] } ] }`,
			`unsupported target — expected tag://<tailnet>/<tag> or group://<tailnet>/<group> (it contains U+2215`},
		{"unknown alias", `{` + ext + ` "grants": [ { "src": ["tag://ghost/relay"], "dst": ["tag:srv"], "ip": ["*"] } ] }`,
			`external tailnet not found: "ghost"`},
		{"incoming false", `{ "externalTailnets": { "corp": { "externalID": "1", "allowIncomingConnections": false } },
			"grants": [ { "src": ["tag://corp/relay"], "dst": ["tag:srv"], "ip": ["*"] } ] }`,
			`external tailnet not allowed in local policy grants: "corp"`},
		{"incoming omitted", `{ "externalTailnets": { "corp": { "externalID": "1" } },
			"grants": [ { "src": ["tag://corp/relay"], "dst": ["tag:srv"], "ip": ["*"] } ] }`,
			`external tailnet not allowed in local policy grants: "corp"`},
		{"malformed beats incoming", `{ "externalTailnets": { "corp": { "externalID": "1" } },
			"grants": [ { "src": ["tag://corp"], "dst": ["tag:srv"], "ip": ["*"] } ] }`,
			`grants[0].src "tag://corp": unsupported target`},
		{"missing externalID", `{ "externalTailnets": { "corp": { "allowIncomingConnections": true } } }`,
			`externalTailnets["corp"]: missing externalTailnet ID`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := RunWithOptions(tc.policy, Options{ExternalTailnets: []eval.ExternalTailnet{
				{Alias: "corp", Nodes: []eval.ExternalNode{{Name: "r1", Tags: []string{"relay"}}}},
			}})
			if !r.Fatal {
				t.Fatalf("want fatal, got %v", r.Diagnostics)
			}
			for _, d := range r.Diagnostics {
				if d.Severity == "error" && strings.Contains(d.Message, tc.want) {
					if d.Pointer == "" {
						t.Errorf("error has no position: %+v", d)
					}
					return
				}
			}
			t.Errorf("no error containing %q in %v", tc.want, r.Diagnostics)
		})
	}
}

func TestRun_ExternalShareExposedUsers(t *testing.T) {
	raw := `{
		"groups": {
			"group:eng":   ["alice@corp.example", "user:*@contractor.example"],
			"group:sales": ["carol@corp.example"]
		},
		"tagOwners": { "tag:app": ["alice@corp.example"] },
		"externalTailnets": {
			"corp": { "externalID": "1", "allowExternalReferencesTo": ["group:eng", "tag:app"] }
		}
	}`
	opt := Options{
		Users: []eval.User{
			{ID: 1, Email: "alice@corp.example"},
			{ID: 2, Email: "bob@contractor.example"},
			{ID: 3, Email: "carol@corp.example"},
		},
		ExternalTailnets: []eval.ExternalTailnet{{Alias: "corp"}},
	}
	r := RunWithOptions(raw, opt)
	if r.ExternalShare == nil || len(r.ExternalShare.Tailnets) != 1 {
		t.Fatalf("ExternalShare not populated: %+v", r.ExternalShare)
	}
	got := strings.Join(r.ExternalShare.Tailnets[0].ExposedUsers, ",")
	if got != "alice@corp.example,bob@contractor.example" {
		t.Errorf("ExposedUsers = %q, want group:eng's members (wildcard expanded)", got)
	}
}

func TestRun_ExpandsIPSets(t *testing.T) {
	raw := `{
		"ipsets": {
			"ipset:corp": ["10.0.0.0/24", "100.64.0.9"],
			"ipset:all":  ["ipset:corp"]
		},
		"acls": [
			{ "action": "accept", "src": ["ipset:all"], "dst": ["tag:srv:22"] },
			{ "action": "accept", "src": ["ipset:ghost"], "dst": ["tag:srv:80"] }
		]
	}`
	r := Run(raw)
	if r.Fatal {
		t.Fatalf("unexpected fatal: %v", diagCodes(r))
	}

	var doc struct {
		IPSets map[string]any `json:"ipsets"`
		Acls   []struct {
			Src []string `json:"src"`
			Dst []string `json:"dst"`
		} `json:"acls"`
	}
	if err := json.Unmarshal([]byte(r.Policy), &doc); err != nil {
		t.Fatalf("stripped policy is not valid JSON: %v", err)
	}
	if doc.IPSets != nil {
		t.Error("ipsets survived preflight")
	}
	if len(doc.Acls) != 2 {
		t.Fatalf("want 2 acls, got %d", len(doc.Acls))
	}
	if strings.Join(doc.Acls[0].Src, ",") != "10.0.0.0/24,100.64.0.9/32" {
		t.Errorf("acls[0].src not expanded: %v", doc.Acls[0].Src)
	}
	if len(doc.Acls[1].Src) != 0 {
		t.Errorf("undefined ipset:ghost should have been dropped, got %v", doc.Acls[1].Src)
	}
	// The expansion itself is silent; only the undefined ipset:ghost is reported.
	if got := strings.Join(diagCodes(r), ","); got != "warning/policy" {
		t.Errorf("diagnostics = %s, want warning/policy (ipset:ghost)", got)
	}
}

func TestRun_ExpandsUserDomainWildcards(t *testing.T) {
	raw := `{
		"acls": [
			{ "action": "accept", "src": ["user:*@corp.example"], "dst": ["tag:srv:22"] },
			{ "action": "accept", "src": ["user:*@ghost.example"], "dst": ["tag:srv:80"] }
		]
	}`
	opt := Options{Users: []eval.User{
		{ID: 1, Email: "alice@corp.example"},
		{ID: 2, Email: "bob@corp.example"},
		{ID: 3, Email: "carol@other.example"},
	}}
	r := RunWithOptions(raw, opt)
	if r.Fatal {
		t.Fatalf("unexpected fatal: %v", diagCodes(r))
	}

	var doc struct {
		Acls []struct {
			Src []string `json:"src"`
		} `json:"acls"`
	}
	if err := json.Unmarshal([]byte(r.Policy), &doc); err != nil {
		t.Fatalf("stripped policy is not valid JSON: %v", err)
	}
	if got := strings.Join(doc.Acls[0].Src, ","); got != "alice@corp.example,bob@corp.example" {
		t.Errorf("acls[0].src = %q, want the two corp.example users", got)
	}
	if len(doc.Acls[1].Src) != 0 {
		t.Errorf("acls[1].src should be empty (no user in ghost.example), got %v", doc.Acls[1].Src)
	}
	for _, d := range r.Diagnostics {
		if d.Severity != "info" {
			t.Errorf("expected info-only diagnostics, got %s: %s", d.Severity, d.Message)
		}
	}
}

func TestRun_RewritesServiceDst(t *testing.T) {
	raw := `{
		"acls": [
			{ "action": "accept", "src": ["tag:client"], "dst": ["svc:web:443", "svc:ghost:80"] },
			{ "action": "accept", "src": ["svc:web"], "dst": ["tag:srv:22"] }
		],
		"grants": [
			{ "src": ["tag:client"], "dst": ["svc:web"], "ip": ["tcp:443"] }
		]
	}`
	opt := Options{Services: []eval.Service{
		{Name: "svc:web", VIP: "100.96.0.5", Backends: []uint64{2}},
	}}
	r := RunWithOptions(raw, opt)
	if r.Fatal {
		t.Fatalf("unexpected fatal: %v", diagCodes(r))
	}

	var doc struct {
		Acls []struct {
			Src []string `json:"src"`
			Dst []string `json:"dst"`
		} `json:"acls"`
		Grants []struct {
			Dst []string `json:"dst"`
		} `json:"grants"`
	}
	if err := json.Unmarshal([]byte(r.Policy), &doc); err != nil {
		t.Fatalf("stripped policy is not valid JSON: %v", err)
	}
	if got := strings.Join(doc.Acls[0].Dst, ","); got != "100.96.0.5:443" {
		t.Errorf("acls[0].dst = %q, want the VIP with :443 (svc:ghost dropped)", got)
	}
	if len(doc.Acls[1].Src) != 0 {
		t.Errorf("svc: in src should be dropped, got %v", doc.Acls[1].Src)
	}
	if got := strings.Join(doc.Grants[0].Dst, ","); got != "100.96.0.5" {
		t.Errorf("grants[0].dst = %q, want the bare VIP (no ports)", got)
	}
	var warned bool
	for _, d := range r.Diagnostics {
		if d.Severity == "warning" {
			warned = true
		}
	}
	if !warned {
		t.Error("expected a warning for the undefined service and the svc: in src")
	}
}

func TestRun_NoWildcardWithoutUsers(t *testing.T) {
	// Plain Run has no Users; a wildcard resolves to nothing + a snapshot note
	// (the rule now matches nobody — add a user in that domain).
	raw := `{"acls":[{"action":"accept","src":["user:*@corp.example"],"dst":["*:*"]}]}`
	r := Run(raw)
	if r.Fatal {
		t.Fatalf("unexpected fatal: %v", diagCodes(r))
	}
	if got := strings.Join(diagCodes(r), ","); got != "info/snapshot" {
		t.Errorf("diagnostics = %s, want info/snapshot for the unresolved wildcard", got)
	}
}

func TestRun_InertKeysAreInfoOnly(t *testing.T) {
	raw := `{
		"derpMap": { "regions": {} },
		"disableIPv4": true,
		"oneCGNATRoute": true,
		"extraDNSRecords": [ { "name": "a", "type": "A", "value": "1.2.3.4" } ],
		"acls": [ { "action": "accept", "src": ["*"], "dst": ["*:*"] } ]
	}`
	r := Run(raw)
	if r.Fatal {
		t.Fatalf("unexpected fatal: %v", diagCodes(r))
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(r.Policy), &doc); err != nil {
		t.Fatalf("stripped policy invalid: %v", err)
	}
	for _, k := range []string{"derpMap", "disableIPv4", "oneCGNATRoute", "extraDNSRecords"} {
		if _, ok := doc[k]; ok {
			t.Errorf("key %q survived preflight", k)
		}
	}

	// They have no bearing on reachability — nothing to fix, so no message.
	if len(r.Diagnostics) != 0 {
		t.Errorf("inert keys should be stripped silently, got %v", diagCodes(r))
	}
}

func TestRun_AutoApproverServicesIsStripped(t *testing.T) {
	raw := `{
		"tagOwners": { "tag:border0-managed": ["autogroup:admin"] },
		"autoApprovers": {
			"routes": { "10.0.0.0/24": ["tag:border0-managed"] },
			"services": { "tag:border0-managed": ["tag:border0-managed", "autogroup:member"] }
		},
		"acls": [ { "action": "accept", "src": ["*"], "dst": ["*:*"] } ]
	}`
	r := Run(raw)
	if r.Fatal {
		t.Fatalf("unexpected fatal: %v", diagCodes(r))
	}
	var doc struct {
		AutoApprovers map[string]json.RawMessage `json:"autoApprovers"`
	}
	if err := json.Unmarshal([]byte(r.Policy), &doc); err != nil {
		t.Fatalf("stripped policy invalid: %v", err)
	}
	if _, ok := doc.AutoApprovers["services"]; ok {
		t.Errorf("autoApprovers.services survived preflight")
	}
	if _, ok := doc.AutoApprovers["routes"]; !ok {
		t.Errorf("autoApprovers.routes was dropped")
	}
	for _, d := range r.Diagnostics {
		if d.Severity == "error" {
			t.Errorf("unexpected error diagnostic: %+v", d)
		}
	}
}

func TestRun_ParsesAttrConfig(t *testing.T) {
	raw := `{
		"attrConfig": {
			"custom:env":    { "type": "string" },
			"custom:score":  { "type": "number", "allowSetByNode": true },
			"custom:secure": { "type": "bool", "allowSetByNode": false, "broadcastToPeers": ["tag:admin"] }
		},
		"postures": { "posture:prod": ["custom:env == 'prod'", "custom:score >= '80'"] },
		"acls": [
			{ "action": "accept", "src": ["*"], "srcPosture": ["posture:prod"], "dst": ["*:*"] }
		]
	}`
	r := Run(raw)
	if r.Fatal {
		t.Fatalf("unexpected fatal: %v", diagCodes(r))
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(r.Policy), &doc); err != nil {
		t.Fatalf("stripped policy invalid: %v", err)
	}
	if _, ok := doc["attrConfig"]; ok {
		t.Error("attrConfig survived preflight")
	}
	if r.Posture == nil {
		t.Fatal("Posture config not populated")
	}
	if r.Posture.CustomAttrs["custom:env"] != "string" ||
		r.Posture.CustomAttrs["custom:score"] != "number" ||
		r.Posture.CustomAttrs["custom:secure"] != "bool" {
		t.Errorf("CustomAttrs = %v", r.Posture.CustomAttrs)
	}
	for _, d := range r.Diagnostics {
		if d.Severity == "error" {
			t.Errorf("unexpected error diagnostic: %s", d.Message)
		}
	}
}

func TestRun_PostureIntegrationsWarnAndStrip(t *testing.T) {
	raw := `{
		"postureIntegrations": [
			{ "provider": "intune", "cloudId": "global", "clientId": "x", "tenantId": "y", "clientSecret": "z" }
		],
		"acls": [ { "action": "accept", "src": ["*"], "dst": ["*:*"] } ]
	}`
	r := Run(raw)
	if r.Fatal {
		t.Fatalf("unexpected fatal: %v", diagCodes(r))
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(r.Policy), &doc); err != nil {
		t.Fatalf("stripped policy invalid: %v", err)
	}
	if _, ok := doc["postureIntegrations"]; ok {
		t.Error("postureIntegrations survived preflight")
	}

	var warned bool
	for _, d := range r.Diagnostics {
		if d.Severity == "warning" && d.Code == "limit" &&
			strings.Contains(d.Message, "postureIntegrations") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("want a postureIntegrations warning, got %v", diagCodes(r))
	}
}

func TestRun_LiftsNodeAttrsApp(t *testing.T) {
	raw := `{
		"acls": [ { "action": "accept", "src": ["*"], "dst": ["*:*"] } ],
		"nodeAttrs": [
			{ "target": ["tag:conn"], "app": { "tailscale.com/app-connectors": [ { "name": "gh" } ] } },
			{ "target": ["alice@example.com"], "attr": ["custom/x"] }
		]
	}`
	r := Run(raw)
	if r.Fatal {
		t.Fatalf("unexpected fatal: %v", diagCodes(r))
	}

	caps, ok := r.AppNodeCaps[0]
	if !ok {
		t.Fatalf("nodeAttrs[0].app not lifted: %+v", r.AppNodeCaps)
	}
	if got := caps["tailscale.com/app-connectors"]; len(got) != 1 || got[0] != `{"name":"gh"}` {
		t.Errorf("app cap value = %v", got)
	}
	if _, ok := r.AppNodeCaps[1]; ok {
		t.Error("entry 1 has no app key but was recorded")
	}

	var doc struct {
		NodeAttrs []struct {
			App  map[string]any `json:"app"`
			Attr []string       `json:"attr"`
		} `json:"nodeAttrs"`
	}
	if err := json.Unmarshal([]byte(r.Policy), &doc); err != nil {
		t.Fatalf("stripped policy invalid: %v", err)
	}
	if doc.NodeAttrs[0].App != nil {
		t.Error("app key survived into the engine copy")
	}
	if len(doc.NodeAttrs[0].Attr) != 1 || doc.NodeAttrs[0].Attr[0] != AppTargetCapPrefix+"0" {
		t.Errorf("entry 0 marker attr = %v", doc.NodeAttrs[0].Attr)
	}
}

func TestRun_ExtractsPosture(t *testing.T) {
	raw := `{
		"postures": {
			"posture:corp": ["node:os == 'macos'", "node:tsVersion >= '1.80'"]
		},
		"acls": [
			{ "action": "accept", "src": ["a@e.com"], "srcPosture": ["posture:corp"], "dst": ["*:*"] }
		]
	}`
	r := Run(raw)
	if r.Fatal {
		t.Fatalf("unexpected fatal")
	}
	if r.Posture == nil {
		t.Fatal("posture not extracted")
	}
	if got := r.Posture.Definitions["posture:corp"]; len(got) != 2 {
		t.Errorf("posture:corp conditions = %v", got)
	}
	if got := r.Posture.ACLSrcPosture[0]; len(got) != 1 || got[0] != "posture:corp" {
		t.Errorf("acls[0] srcPosture = %v", got)
	}
	if strings.Contains(r.Policy, "srcPosture") || strings.Contains(r.Policy, "postures") {
		t.Errorf("posture keys leaked into engine policy:\n%s", r.Policy)
	}
}

func TestRun_ExtractsDefaultSrcPosture(t *testing.T) {
	raw := `{
		"postures": {
			"posture:corp": ["node:os == 'macos'"]
		},
		"defaultSrcPosture": ["posture:corp"],
		"acls": [
			{ "action": "accept", "src": ["a@e.com"], "dst": ["*:*"] }
		]
	}`
	r := Run(raw)
	if r.Fatal {
		t.Fatalf("unexpected fatal: %+v", r.Diagnostics)
	}
	if r.Posture == nil {
		t.Fatal("posture not extracted")
	}
	if got := r.Posture.DefaultSrcPosture; len(got) != 1 || got[0] != "posture:corp" {
		t.Errorf("DefaultSrcPosture = %v", got)
	}
	if strings.Contains(r.Policy, "defaultSrcPosture") {
		t.Errorf("defaultSrcPosture leaked into engine policy:\n%s", r.Policy)
	}
	if len(r.Diagnostics) != 0 {
		t.Errorf("applying defaultSrcPosture should be silent, got %+v", r.Diagnostics)
	}
}

func TestRun_UnknownAutogroupIsFatal(t *testing.T) {
	for _, tok := range []string{"autogroup:bogus", "autogroup:frobnicate:*"} {
		raw := `{ "acls": [ { "action": "accept", "src": ["` + tok + `"], "dst": ["*:*"] } ] }`
		r := Run(raw)
		if !r.Fatal {
			t.Errorf("%s: expected fatal, got %v", tok, diagCodes(r))
		}
	}
}

func TestRun_RoleAutogroupsResolve(t *testing.T) {
	raw := `{
		"tagOwners": { "tag:srv": ["autogroup:admin"] },
		"acls": [
			{ "action": "accept", "src": ["autogroup:admin"], "dst": ["autogroup:auditor:22"] },
			{ "action": "accept", "src": ["autogroup:shared"], "dst": ["tag:srv:80"] },
			{ "action": "accept", "src": ["autogroup:billing-admin"], "dst": ["*:*"] }
		]
	}`
	opt := Options{Users: []eval.User{
		{ID: 1, Email: "owner@corp.example", Role: "owner"},
		{ID: 2, Email: "admin@corp.example", Role: "admin"},
		{ID: 3, Email: "audit@corp.example", Role: "auditor"},
		{ID: 4, Email: "guest@x.example", Shared: true},
	}}
	r := RunWithOptions(raw, opt)
	if r.Fatal {
		t.Fatalf("unexpected fatal: %v", diagCodes(r))
	}

	var doc struct {
		TagOwners map[string][]string `json:"tagOwners"`
		Acls      []struct {
			Src []string `json:"src"`
			Dst []string `json:"dst"`
		} `json:"acls"`
	}
	if err := json.Unmarshal([]byte(r.Policy), &doc); err != nil {
		t.Fatalf("stripped policy is not valid JSON: %v", err)
	}
	if got := strings.Join(doc.TagOwners["tag:srv"], ","); got != "admin@corp.example,owner@corp.example" {
		t.Errorf("tagOwners expansion = %q", got)
	}
	if got := strings.Join(doc.Acls[0].Src, ","); got != "admin@corp.example,owner@corp.example" {
		t.Errorf("acls[0].src = %q, want admin + owner", got)
	}
	if got := strings.Join(doc.Acls[0].Dst, ","); got != "audit@corp.example:22" {
		t.Errorf("acls[0].dst = %q, want the auditor with :22 kept", got)
	}
	if got := strings.Join(doc.Acls[1].Src, ","); got != "guest@x.example" {
		t.Errorf("acls[1].src (shared) = %q", got)
	}
	if len(doc.Acls[2].Src) != 0 {
		t.Errorf("acls[2].src (billing-admin, no match) should be empty, got %v", doc.Acls[2].Src)
	}
	for _, d := range r.Diagnostics {
		if d.Severity == "error" {
			t.Errorf("unexpected error diagnostic: %s", d.Message)
		}
	}
}

func TestRun_RoleAutogroupsInAutoApprovers(t *testing.T) {
	raw := `{
		"acls": [ { "action": "accept", "src": ["*"], "dst": ["*:*"] } ],
		"autoApprovers": {
			"routes": { "10.0.0.0/24": ["autogroup:admin", "group:net"] },
			"exitNode": ["autogroup:owner"]
		},
		"groups": { "group:net": ["net@corp.example"] }
	}`
	opt := Options{Users: []eval.User{
		{ID: 1, Email: "owner@corp.example", Role: "owner"},
		{ID: 2, Email: "admin@corp.example", Role: "admin"},
	}}
	r := RunWithOptions(raw, opt)
	if r.Fatal {
		t.Fatalf("unexpected fatal: %v", diagCodes(r))
	}
	var doc struct {
		AutoApprovers struct {
			Routes   map[string][]string `json:"routes"`
			ExitNode []string            `json:"exitNode"`
		} `json:"autoApprovers"`
	}
	if err := json.Unmarshal([]byte(r.Policy), &doc); err != nil {
		t.Fatalf("stripped policy is not valid JSON: %v", err)
	}
	if got := strings.Join(doc.AutoApprovers.Routes["10.0.0.0/24"], ","); got != "admin@corp.example,owner@corp.example,group:net" {
		t.Errorf("routes approver expansion = %q", got)
	}
	if got := strings.Join(doc.AutoApprovers.ExitNode, ","); got != "owner@corp.example" {
		t.Errorf("exitNode approver expansion = %q", got)
	}
}

func TestRun_HeadscaleAutogroupInAutoApproversIsFatal(t *testing.T) {
	// autogroup:member is fine in a rule but Headscale's autoApprovers parser
	// rejects any autogroup — Tailground reports that rather than letting the
	// compile fail cryptically.
	raw := `{ "autoApprovers": { "exitNode": ["autogroup:member"] } }`
	r := Run(raw)
	if !r.Fatal {
		t.Errorf("expected fatal for autogroup:member in autoApprovers, got %v", diagCodes(r))
	}
}

func TestRun_SharedAutogroupOutsideSrcIsDropped(t *testing.T) {
	raw := `{ "acls": [ { "action": "accept", "src": ["*"], "dst": ["autogroup:shared:*"] } ] }`
	r := RunWithOptions(raw, Options{Users: []eval.User{{ID: 1, Email: "g@x.example", Shared: true}}})
	if r.Fatal {
		t.Fatalf("unexpected fatal: %v", diagCodes(r))
	}
	var warned bool
	for _, d := range r.Diagnostics {
		if d.Severity == "warning" && strings.Contains(d.Message, "only valid in a rule's src") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("expected a warning about autogroup:shared in dst, got %v", diagCodes(r))
	}
}

func TestRun_SupportedAutogroupWithPortIsFine(t *testing.T) {
	raw := `{ "acls": [ { "action": "accept", "src": ["autogroup:member"], "dst": ["autogroup:self:*"] } ] }`
	r := Run(raw)
	if r.Fatal {
		t.Fatalf("false fatal on supported autogroups: %v", diagCodes(r))
	}
}

func TestRun_NodeAttrsFunnelDropped(t *testing.T) {
	raw := `{ "nodeAttrs": [ { "target": ["*"], "attr": ["funnel", "mullvad"] } ] }`
	r := Run(raw)
	if r.Fatal {
		t.Fatal("unexpected fatal")
	}
	if strings.Contains(r.Policy, "funnel") {
		t.Errorf("funnel survived:\n%s", r.Policy)
	}
	if !strings.Contains(r.Policy, "mullvad") {
		t.Errorf("mullvad was wrongly dropped:\n%s", r.Policy)
	}
}

func TestRun_MalformedHuJSONIsFatal(t *testing.T) {
	r := Run(`{ "acls": [`)
	if !r.Fatal {
		t.Fatal("expected fatal on malformed input")
	}
}

func TestRun_CommentsAndTrailingCommasAccepted(t *testing.T) {
	raw := `{
		// a comment
		"acls": [
			{ "action": "accept", "src": ["*"], "dst": ["*:*"] }, // trailing comma next
		],
	}`
	r := Run(raw)
	if r.Fatal {
		t.Fatalf("HuJSON sugar rejected: %v", diagCodes(r))
	}
	if !json.Valid([]byte(r.Policy)) {
		t.Errorf("output not standard JSON: %s", r.Policy)
	}
}
