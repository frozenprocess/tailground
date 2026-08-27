// Package preflight inspects a raw HuJSON policy before it reaches the Headscale
// engine. It has three jobs:
package preflight

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tailscale/hujson"

	"github.com/frozenprocess/tailground/engine/eval"
	"github.com/frozenprocess/tailground/engine/extshare"
	"github.com/frozenprocess/tailground/engine/ipset"
	"github.com/frozenprocess/tailground/engine/service"
)

type Options struct {
	Users            []eval.User
	Services         []eval.Service
	ExternalTailnets []eval.ExternalTailnet
}

// AppTargetCapPrefix is the marker node capability preflight stamps on a
// nodeAttrs entry whose `app` key it lifted out (Headscale rejects `app`). The
// suffix is the entry's index; the hs adapter reads it back off each compiled
// node to fold the real app capabilities onto that node (see foldAppNodeCaps).
const AppTargetCapPrefix = "https://tailground.dev/cap/nodeattr-app/"

// Result is the outcome of a preflight pass.
type Result struct {
	// Policy is the cleaned policy as standard JSON, ready for the engine.
	// Empty when Fatal is true.
	Policy string
	// Diagnostics collects every note, warning and error found.
	Diagnostics []eval.Diagnostic
	Posture     *PostureConfig
	// Fatal is set when a problem can't be safely stripped (an unsupported
	// autogroup token inside a rule) — the caller should not evaluate.
	Fatal bool
	// AppNodeCaps holds the `nodeAttrs[i].app` capabilities lifted out of the
	// policy (Headscale rejects the `app` key), keyed by nodeAttrs index →
	// capability name → raw JSON value strings. The hs adapter resolves the
	// entry's targets via the [AppTargetCapPrefix] marker and folds these onto
	// each target node's Result.NodeCaps.
	AppNodeCaps   map[int]map[string][]string
	ExternalShare *ExternalShare

	// sharedIncoming counts the external shared nodes that may initiate
	// connections — what an `autogroup:shared` src also matches.
	sharedIncoming int
}

// ExternalShare is preflight's resolved view of declarative node sharing.
type ExternalShare struct {
	Tailnets []ExternalShareTailnet
}

// ExternalShareTailnet is one external tailnet with its config and shared nodes.
type ExternalShareTailnet struct {
	Alias                     string
	ExternalID                string
	AllowIncomingConnections  bool
	AllowExternalReferencesTo []string
	Configured                bool // the policy's externalTailnets section named it
	// ExposedUsers is the emails of the snapshot users in the local groups
	// AllowExternalReferencesTo lists — their (untagged) devices are, with the
	// listed tags' devices, what the other tailnet's policy may name as a src.
	ExposedUsers []string
	Nodes        []ExternalShareNode
}

// ExternalShareNode is one shared node with its final (possibly synthetic)
// addresses.
type ExternalShareNode struct {
	Name   string
	Tags   []string // normalized with the "tag:" prefix
	Groups []string // normalized with the "group:" prefix
	IPv4   string
	IPv6   string
}

// PostureConfig is the posture data lifted out of the policy.
type PostureConfig struct {
	// Definitions maps a posture name ("posture:corp-macos") to its raw
	// condition expressions (["node:os == 'macos'", "node:tsVersion >= '1.80'"]).
	Definitions map[string][]string
	// ACLSrcPosture maps an acls[] index to the posture names its srcPosture
	// referenced.
	ACLSrcPosture map[int][]string
	// GrantSrcPosture maps a grants[] index to the posture names its srcPosture
	// referenced.
	GrantSrcPosture map[int][]string
	// SSHSrcPosture maps an ssh[] index to the posture names its srcPosture
	// referenced. Applied by the posture layer as an ordered first-match gate
	// (a miss falls through to the next ssh rule).
	SSHSrcPosture map[int][]string
	// DefaultSrcPosture is the policy's top-level `defaultSrcPosture` list: the
	// baseline posture names applied to every acls/grants/ssh rule that carries
	// no srcPosture of its own. A rule's own srcPosture replaces (does not
	// combine with) this default. Empty ⇒ no policy-wide default.
	DefaultSrcPosture []string
	// CustomAttrs maps a custom device-posture attribute name ("custom:env") to
	// its declared type ("string" | "number" | "boolean"), from the policy's
	// `attrConfig` section. Empty ⇒ no custom attributes declared. The posture
	// evaluator uses the type to pick string / numeric comparison.
	CustomAttrs map[string]string
}

// knownTopLevel is every key the Headscale engine accepts.
var knownTopLevel = map[string]bool{
	"groups": true, "hosts": true, "tagowners": true, "acls": true,
	"grants": true, "nodeattrs": true, "autoapprovers": true, "ssh": true,
	"tests": true, "sshtests": true, "randomizeclientport": true,
}

var notModeledTopLevel = map[string]struct {
	inert bool
	note  string
}{
	"derpmap":         {inert: true},
	"disableipv4":     {inert: true},
	"onecgnatroute":   {inert: true},
	"extradnsrecords": {inert: true},

	"postureintegrations": {note: "postureIntegrations configures external device-posture providers (Intune, Jamf, Kandji, CrowdStrike, SentinelOne, …); Tailground models only the built-in node:os / node:tsVersion / node:tsReleaseTrack conditions, so a srcPosture condition on a custom attribute these would supply will not match"},
}

// supportedAutogroups is the set the Headscale engine resolves. A token
// "autogroup:X" with X outside this set can't be silently dropped without
// changing a rule's meaning, so it's fatal.
var supportedAutogroups = map[string]bool{
	"internet": true, "member": true, "tagged": true,
	"self": true, "nonroot": true, "danger-all": true,
}

// Run preflights raw with no tailnet context. It is shorthand for
// RunWithOptions(raw, Options{}).
func Run(raw string) Result { return RunWithOptions(raw, Options{}) }

// RunWithOptions preflights raw and returns the cleaned policy plus
// diagnostics, using opt for the passes that need mock-tailnet context.
func RunWithOptions(raw string, opt Options) Result {
	var res Result

	root, err := hujson.Parse([]byte(raw))
	if err != nil {
		res.note("error", "parse", hujsonErrPointer(err), "%s", "HuJSON: "+err.Error())
		res.Fatal = true
		return res
	}

	obj, ok := root.Value.(*hujson.Object)
	if !ok {
		res.note("error", "parse", "", "policy must be a JSON object")
		res.Fatal = true
		return res
	}

	sec := newSections()

	// Before any section is stripped, so references in ipsets etc. count too.
	checkExternalRefPlacement(raw, obj, &res)

	sec.take(raw, obj, &res)
	rewriteRules(raw, obj, opt, sec, &res)

	if sec.posture.declared() {
		res.Posture = sec.posture
	}

	sortDiagnostics(res.Diagnostics)

	if res.Fatal {
		return res
	}

	root.Standardize()
	res.Policy = string(root.Pack())
	return res
}

// sections is what take lifts out of the policy's top level: the sections the
// engine doesn't accept, kept for the rule-level passes that follow.
type sections struct {
	posture   *PostureConfig
	ipsetDefs map[string][]string
	extConfig map[string]extshare.TailnetConfig
	extPtr    string // where the externalTailnets key sits; config problems point here
	ipsetPtr  string // where the ipsets key sits
}

func newSections() *sections {
	return &sections{
		posture: &PostureConfig{
			Definitions:     map[string][]string{},
			ACLSrcPosture:   map[int][]string{},
			GrantSrcPosture: map[int][]string{},
			SSHSrcPosture:   map[int][]string{},
			CustomAttrs:     map[string]string{},
		},
		ipsetDefs: map[string][]string{},
		extConfig: map[string]extshare.TailnetConfig{},
	}
}

// declared reports whether the policy declared any posture the evaluator needs.
func (p *PostureConfig) declared() bool {
	return len(p.Definitions) > 0 || len(p.ACLSrcPosture) > 0 ||
		len(p.GrantSrcPosture) > 0 || len(p.SSHSrcPosture) > 0 ||
		len(p.DefaultSrcPosture) > 0
}

// take splits obj's top-level members: the engine-native ones stay, the
// Tailground-only ones move into s, and the rest are dropped (with a note
// unless inert).
func (s *sections) take(raw string, obj *hujson.Object, res *Result) {
	kept := obj.Members[:0]
	for _, m := range obj.Members {
		key := literalString(m.Name.Value)
		lc := lineCol(raw, m.Name.StartOffset)
		lkey := strings.ToLower(key)

		switch {
		case lkey == "postures":
			for name, exprs := range parseNameLists(m.Value) {
				s.posture.Definitions[name] = exprs
			}

		case lkey == "defaultsrcposture":
			for v := range m.Value.All() {
				if str, ok := stringLiteral(v.Value); ok {
					s.posture.DefaultSrcPosture = append(s.posture.DefaultSrcPosture, str)
				}
			}

		case lkey == "ipsets":
			for name, members := range parseNameLists(m.Value) {
				s.ipsetDefs[strings.TrimPrefix(name, "ipset:")] = members
			}
			s.ipsetPtr = lc // ipset definition problems point here

		case lkey == "attrconfig":
			for name, typ := range parseAttrConfig(m.Value) {
				s.posture.CustomAttrs[name] = typ
			}

		case lkey == "externaltailnets":
			for alias, cfg := range parseExternalTailnets(m.Value) {
				s.extConfig[alias] = cfg
			}
			checkExternalTailnetIDs(raw, m.Value, res)
			s.extPtr = lc

		case knownTopLevel[lkey]:
			kept = append(kept, m)

		case notModeledTopLevel[lkey].inert:
			// dropped silently

		case notModeledTopLevel[lkey].note != "":
			res.note("warning", "limit", lc, "%q: %s", key, notModeledTopLevel[lkey].note)

		default:
			res.note("warning", "policy", lc, "unknown key %q was ignored (Tailscale would reject the whole file)", key)
		}
	}
	obj.Members = kept
}

// rewriteRules runs the rule-level passes over what take kept: alias-token
// rewriting, posture extraction, and stripping of un-modeled rule bits.
func rewriteRules(raw string, obj *hujson.Object, opt Options, s *sections, res *Result) {
	if len(s.ipsetDefs) > 0 {
		sets, ipDiags := ipset.Resolve(s.ipsetDefs, parseHosts(obj))
		for i := range ipDiags {
			if ipDiags[i].Pointer == "" {
				ipDiags[i].Pointer = s.ipsetPtr
			}
		}
		res.Diagnostics = append(res.Diagnostics, ipDiags...)
		rewriteIPSets(raw, obj, sets, res)
	}
	svcReg, svcDiags := service.Build(opt.Services)
	res.Diagnostics = append(res.Diagnostics, svcDiags...)
	rewriteServices(raw, obj, svcReg, res)
	resolveExternalShare(raw, obj, opt.ExternalTailnets, opt.Users, s.extConfig, s.extPtr, res)
	stripAutoApproverServices(obj)
	expandUserWildcards(raw, obj, opt.Users, res)
	rewriteRoleAutogroups(raw, obj, opt.Users, res)
	scanAutogroups(raw, obj, res)
	extractRuleSrcPosture(obj, "acls", s.posture.ACLSrcPosture, res, raw)
	extractRuleSrcPosture(obj, "grants", s.posture.GrantSrcPosture, res, raw)
	extractRuleSrcPosture(obj, "ssh", s.posture.SSHSrcPosture, res, raw)
	stripSSHRecorder(obj)
	sanitizeNodeAttrs(raw, obj, res)
}

// scanAutogroups walks every string literal under acls/grants/ssh/tagOwners/
// autoApprovers/nodeAttrs and flags any autogroup: token Tailground can't
// evaluate. The admin-role and `shared` tokens are already gone by now
// (rewriteRoleAutogroups resolved them); what's left is Headscale-native tokens
// (fine everywhere except autoApprovers) and genuinely unknown ones.
func scanAutogroups(raw string, obj *hujson.Object, res *Result) {
	for _, m := range obj.Members {
		key := strings.ToLower(literalString(m.Name.Value))
		switch key {
		case "acls", "grants", "ssh", "tagowners", "nodeattrs":
			for v := range m.Value.All() {
				s, ok := stringLiteral(v.Value)
				if !ok || !strings.HasPrefix(s, "autogroup:") {
					continue
				}
				// In a dst the token is "autogroup:<name>:<ports>"; in a src
				// it's just "autogroup:<name>". Take the name either way.
				name, _, _ := strings.Cut(strings.TrimPrefix(s, "autogroup:"), ":")
				if supportedAutogroups[name] {
					continue
				}
				res.note("error", "limit", lineCol(raw, v.StartOffset), "%q may be valid on Tailscale, but Tailground can't evaluate it — remove or replace it here to see results", s)
				res.Fatal = true
			}
		case "autoapprovers":
			for v := range m.Value.All() {
				s, ok := stringLiteral(v.Value)
				if !ok || !strings.HasPrefix(s, "autogroup:") {
					continue
				}
				res.note("error", "policy", lineCol(raw, v.StartOffset), "%q can't be an auto-approver — Tailscale allows only users, groups and tags", s)
				res.Fatal = true
			}
		}
	}
}

// extractRuleSrcPosture pulls `srcPosture` out of each element of the named
// rule array, recording which posture names each rule referenced and removing
// the key from the engine copy.
func extractRuleSrcPosture(obj *hujson.Object, arrayKey string, into map[int][]string, res *Result, raw string) {
	arr := arrayMember(obj, arrayKey)
	if arr == nil {
		return
	}
	for i := range arr.Elements {
		el, ok := arr.Elements[i].Value.(*hujson.Object)
		if !ok {
			continue
		}
		kept := el.Members[:0]
		for _, mm := range el.Members {
			if strings.ToLower(literalString(mm.Name.Value)) != "srcposture" {
				kept = append(kept, mm)
				continue
			}
			for v := range mm.Value.All() {
				if s, ok := stringLiteral(v.Value); ok {
					into[i] = append(into[i], s)
				}
			}
		}
		el.Members = kept
	}
}

// --- hujson helpers -------------------------------------------------------

func sortDiagnostics(d []eval.Diagnostic) {
	rank := map[string]int{"error": 0, "warning": 1, "info": 2}
	sort.SliceStable(d, func(i, j int) bool {
		if rank[d[i].Severity] != rank[d[j].Severity] {
			return rank[d[i].Severity] < rank[d[j].Severity]
		}
		return d[i].Pointer < d[j].Pointer
	})
}

// autogroupFix says how to give a role / sharing autogroup a member in the
// snapshot, for the "matches no user" note.
func autogroupFix(name string) string {
	if name == "shared" {
		return "mark a user as shared under Users, or share nodes in from an external tailnet"
	}
	return fmt.Sprintf("give a user the %s role under Users", name)
}
