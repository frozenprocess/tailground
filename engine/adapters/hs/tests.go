package hs

import (
	"encoding/json"
	"regexp"
	"strings"

	hspolicy "github.com/juanfont/headscale/hscontrol/policy"
	policyv2 "github.com/juanfont/headscale/hscontrol/policy/v2"

	"github.com/frozenprocess/tailground/engine/eval"
)

// policyTests mirrors the `tests` / `sshTests` blocks well enough to enumerate
// each assertion for the tutorial pane.
type policyTests struct {
	Tests []struct {
		Src    string   `json:"src"`
		Proto  string   `json:"proto"`
		Accept []string `json:"accept"`
		Deny   []string `json:"deny"`
	} `json:"tests"`
	SSHTests []struct {
		Src    string   `json:"src"`
		Dst    []string `json:"dst"`
		Accept []string `json:"accept"`
		Deny   []string `json:"deny"`
		Check  []string `json:"check"`
	} `json:"sshTests"`
}

// evalTests runs the policy's own `tests` / `sshTests` blocks (D6) and records a
// per-assertion pass/fail report. It only runs when the policy declares tests.
func evalTests(pm hspolicy.PolicyManager, cleanedPolicy string, res *eval.Result) {
	var pt policyTests
	if err := json.Unmarshal([]byte(cleanedPolicy), &pt); err != nil {
		return
	}
	if len(pt.Tests) == 0 && len(pt.SSHTests) == 0 {
		return
	}

	v2pm, ok := pm.(*policyv2.PolicyManager)
	if !ok {
		return
	}

	report := &eval.TestReport{}

	if len(pt.Tests) > 0 {
		testErr := v2pm.RunTests()
		g := &eval.TestGroup{Passed: testErr == nil}
		for _, t := range pt.Tests {
			for _, dst := range t.Accept {
				g.Cases = append(g.Cases, eval.TestCase{
					Label: t.Src + " → " + dst, Expect: "reachable", Pass: true,
				})
			}
			for _, dst := range t.Deny {
				g.Cases = append(g.Cases, eval.TestCase{
					Label: t.Src + " → " + dst, Expect: "blocked", Pass: true,
				})
			}
		}
		if !g.Passed {
			attributeACLFailures(testErr, g)
		}
		report.ACL = g
	}

	if len(pt.SSHTests) > 0 {
		sshErr := v2pm.RunSSHTests()
		g := &eval.TestGroup{Passed: sshErr == nil}
		for _, t := range pt.SSHTests {
			dstLabel := strings.Join(t.Dst, ", ")
			add := func(users []string, expect string) {
				for _, u := range users {
					g.Cases = append(g.Cases, eval.TestCase{
						Label:  t.Src + " → " + dstLabel + " as " + displayUser(u),
						Expect: expect, Pass: true,
					})
				}
			}
			add(t.Accept, "reachable")
			add(t.Deny, "blocked")
			add(t.Check, "via check")
		}
		if !g.Passed {
			attributeSSHFailures(sshErr, g)
		}
		// RunSSHTests evaluates the ssh[] block in isolation; it doesn't know
		// that Tailscale SSH still needs the L4 filter to admit tcp/22. When the
		// gated SSH matrix (res.SSH) came back all-deny, no `reachable`/`via
		// check` assertion can actually hold — the port is closed. (Matches the
		// evalSSH tcp/22 gate; see e2e case 35-ssh-needs-l4-port.)
		if g.Passed && sshMatrixAllDeny(res.SSH) {
			g.Passed = false
			for i := range g.Cases {
				if g.Cases[i].Expect != "blocked" {
					g.Cases[i].Pass = false
					g.Cases[i].Detail = "no rule opens tcp/22 to this destination — Tailscale SSH still needs an L4 grant for port 22"
				}
			}
		}
		report.SSH = g
	}

	res.Tests = report
}

// sshMatrixAllDeny reports whether the gated SSH matrix has at least one entry
// and every one of them is "deny" — i.e. the L4 tcp/22 gate (or the ssh[] block
// itself) blocked every probed login.
func sshMatrixAllDeny(m map[string]string) bool {
	if len(m) == 0 {
		return false
	}
	for _, v := range m {
		if v != eval.SSHDeny {
			return false
		}
	}
	return true
}

func displayUser(u string) string {
	if u == "" {
		return `""`
	}
	return u
}

// Failure-line formats come from PolicyTestResults.Errors() /
// SSHPolicyTestResults.Errors() in the vendored v2 package.
var (
	aclFailRe    = regexp.MustCompile(`^(.+?) -> (.+?)(?: \([^)]*\))?: expected (ALLOWED|DENIED), got (ALLOWED|DENIED)$`)
	aclSrcErrRe  = regexp.MustCompile(`^(.+?)(?: \([^)]*\))?: (.+)$`)
	testingDstRe = regexp.MustCompile(`^error testing "(.+?)": (.+)$`)
	sshFailRe    = regexp.MustCompile(`^(.+?)/(.+?) -> (.+?): (.+)$`)
)

func failureLines(err error) []string {
	if err == nil {
		return nil
	}
	var out []string
	for _, ln := range strings.Split(err.Error(), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasSuffix(ln, "failed:") || ln == "test(s) failed" {
			continue
		}
		out = append(out, ln)
	}
	return out
}

func attributeACLFailures(err error, g *eval.TestGroup) {
	failCase := func(label, detail string) bool {
		hit := false
		for i := range g.Cases {
			if g.Cases[i].Label == label {
				g.Cases[i].Pass = false
				g.Cases[i].Detail = detail
				hit = true
			}
		}
		return hit
	}
	failSrc := func(src, detail string) bool {
		hit := false
		for i := range g.Cases {
			if strings.HasPrefix(g.Cases[i].Label, src+" → ") {
				g.Cases[i].Pass = false
				g.Cases[i].Detail = detail
				hit = true
			}
		}
		return hit
	}

	for _, ln := range failureLines(err) {
		if m := aclFailRe.FindStringSubmatch(ln); m != nil {
			if failCase(m[1]+" → "+m[2], "expected "+strings.ToLower(m[3])+", got "+strings.ToLower(m[4])) {
				continue
			}
		} else if m := aclSrcErrRe.FindStringSubmatch(ln); m != nil {
			src, rest := m[1], m[2]
			if dm := testingDstRe.FindStringSubmatch(rest); dm != nil {
				if failCase(src+" → "+dm[1], dm[2]) {
					continue
				}
			} else if failSrc(src, rest) {
				continue
			}
		}
		g.RawFailures = append(g.RawFailures, ln)
	}
}

func attributeSSHFailures(err error, g *eval.TestGroup) {
	for _, ln := range failureLines(err) {
		m := sshFailRe.FindStringSubmatch(ln)
		if m == nil {
			g.RawFailures = append(g.RawFailures, ln)
			continue
		}
		src, user, detail := m[1], m[2], m[4]
		// The failure line's dst is a node hostname, not the test's dst alias,
		// so attribute by (src, user) across every case that shares them.
		matched := false
		for i := range g.Cases {
			c := &g.Cases[i]
			if strings.HasPrefix(c.Label, src+" → ") && strings.HasSuffix(c.Label, " as "+user) {
				c.Pass = false
				c.Detail = detail
				matched = true
			}
		}
		if !matched {
			g.RawFailures = append(g.RawFailures, ln)
		}
	}
}
