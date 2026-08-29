//go:build e2e

package e2e

import (
	"fmt"
	"strings"

	"github.com/frozenprocess/tailground/engine/eval"
)

// RowStatus is the outcome of one assertion.
type RowStatus string

const (
	// RowOK: engine == tailnet == authored expectation.
	RowOK         RowStatus = "ok"
	RowDivergence RowStatus = "DIVERGENCE"
	// RowQuestionable: engine == tailnet but != authored expectation. Not fatal
	// — the assertion is probably wrong.
	RowQuestionable RowStatus = "questionable"
	// RowSkipped: skip.json flow, SSH assertion, or unmet need.
	RowSkipped RowStatus = "skip"
	// RowError: the probe or engine call failed.
	RowError RowStatus = "error"
)

// Row is one assertion's result.
type Row struct {
	Assertion Assertion
	Engine    string // allow | deny
	Tailnet   string // allow | deny
	Status    RowStatus
	Detail    string
}

// classify compares the three verdicts for an assertion. For SSH it folds
// `reject` into `deny`: Tailscale SSH has no reject *action* (only accept /
// check), so the engine's distinct `reject` verdict is unreachable from a real
// policy and a live "not permitted" rejection is just "blocked".
func classify(a Assertion, engineV, tailnetV string) Row {
	r := Row{Assertion: a, Engine: engineV, Tailnet: tailnetV}
	e, tn, exp := engineV, tailnetV, a.Expect
	if a.SSH {
		e, tn, exp = foldReject(e), foldReject(tn), foldReject(exp)
	}
	switch {
	case e != tn:
		r.Status = RowDivergence
	case e != exp:
		r.Status = RowQuestionable
		r.Detail = "authored=" + a.Expect
	default:
		r.Status = RowOK
	}
	return r
}

func foldReject(v string) string {
	if v == string(SSHReject) {
		return string(SSHDeny)
	}
	return v
}

// CaseReport is the result of one case.
type CaseReport struct {
	Slug         string
	Title        string
	Nodes        []string
	FilterLanded bool
	Rows         []Row
	// LoadErr / SetupErr carry a failure before assertions could run.
	SetupErr string

	pushedPolicy string
	storedPolicy string // what Tailscale returns for the pushed ACL (GET /acl)
	enginePolicy string
	engineResult eval.Result
}

// Failed reports whether the case counts as a failure: a setup error, any
// divergence, or any errored row.
func (r CaseReport) Failed() bool {
	if r.SetupErr != "" {
		return true
	}
	for _, row := range r.Rows {
		if row.Status == RowDivergence || row.Status == RowError {
			return true
		}
	}
	return false
}

// Counts tallies row statuses.
func (r CaseReport) Counts() (ok, diverged, questionable, skipped, errored int) {
	for _, row := range r.Rows {
		switch row.Status {
		case RowOK:
			ok++
		case RowDivergence:
			diverged++
		case RowQuestionable:
			questionable++
		case RowSkipped:
			skipped++
		case RowError:
			errored++
		}
	}
	return
}

// Table renders the per-assertion comparison for one case.
func (r CaseReport) Table() string {
	var b strings.Builder
	fmt.Fprintf(&b, "── %s  %s\n", r.Slug, r.Title)
	if len(r.Nodes) > 0 {
		fmt.Fprintf(&b, "   nodes: %s   filter-landed: %v\n", strings.Join(r.Nodes, " "), r.FilterLanded)
	}
	if r.SetupErr != "" {
		fmt.Fprintf(&b, "   SETUP FAILED: %s\n", r.SetupErr)
		return b.String()
	}
	fmt.Fprintf(&b, "   %-22s  %-7s  %-7s  %-7s  %s\n", "flow", "engine", "tailnet", "expect", "status")
	for _, row := range r.Rows {
		a := row.Assertion
		flow := fmt.Sprintf("%s→%s %s/%d", a.From, a.To, a.Proto, a.Port)
		if a.SSH {
			flow = fmt.Sprintf("%s→%s ssh:%s", a.From, a.To, a.User)
		}
		fmt.Fprintf(&b, "   %-22s  %-7s  %-7s  %-7s  %s",
			flow, dash(row.Engine), dash(row.Tailnet), dash(a.Expect), row.Status)
		if row.Detail != "" {
			fmt.Fprintf(&b, "  (%s)", row.Detail)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// Suite aggregates every case report for the end-of-run tally.
type Suite struct {
	Reports []CaseReport
	Skipped map[string]string // slug → reason (cases that never ran)
}

func (s Suite) Tally() string {
	var b strings.Builder
	var tOK, tDiv, tQ, tSkip, tErr, casesFailed int
	for _, r := range s.Reports {
		ok, div, q, sk, er := r.Counts()
		tOK, tDiv, tQ, tSkip, tErr = tOK+ok, tDiv+div, tQ+q, tSkip+sk, tErr+er
		verdict := "pass"
		if r.Failed() {
			verdict = "FAIL"
			casesFailed++
		}
		fmt.Fprintf(&b, "  %-24s %-5s  ok=%d div=%d quest=%d skip=%d err=%d\n",
			r.Slug, verdict, ok, div, q, sk, er)
	}
	for slug, reason := range s.Skipped {
		fmt.Fprintf(&b, "  %-24s skip   %s\n", slug, reason)
	}
	fmt.Fprintf(&b, "\n%d case(s): %d failed, %d skipped entirely\n",
		len(s.Reports)+len(s.Skipped), casesFailed, len(s.Skipped))
	fmt.Fprintf(&b, "assertions: ok=%d divergence=%d questionable=%d skipped=%d error=%d\n",
		tOK, tDiv, tQ, tSkip, tErr)
	return b.String()
}
