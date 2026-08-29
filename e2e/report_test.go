//go:build e2e

package e2e

import (
	"strings"
	"testing"

	"github.com/frozenprocess/tailground/engine/eval"
)

func TestClassifyRows(t *testing.T) {
	a := Assertion{From: "node-a", To: "node-b", Proto: "tcp", Port: 22}

	cases := []struct {
		name                    string
		engine, tailnet, expect string
		want                    RowStatus
	}{
		{"all agree", eval.VerdictAllow, eval.VerdictAllow, eval.VerdictAllow, RowOK},
		{"engine != tailnet", eval.VerdictAllow, eval.VerdictDeny, eval.VerdictAllow, RowDivergence},
		{"both != authored", eval.VerdictDeny, eval.VerdictDeny, eval.VerdictAllow, RowQuestionable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ac := a
			ac.Expect = c.expect
			row := classify(ac, c.engine, c.tailnet)
			if row.Status != c.want {
				t.Fatalf("status = %s, want %s", row.Status, c.want)
			}
			rep := CaseReport{Slug: "demo", Rows: []Row{row}}
			table := rep.Table()
			if !strings.Contains(table, string(c.want)) {
				t.Errorf("table missing status %q:\n%s", c.want, table)
			}
			if !strings.Contains(table, c.engine) || !strings.Contains(table, c.tailnet) {
				t.Errorf("table missing a verdict:\n%s", table)
			}
		})
	}

	// A divergence or an error makes the case fail; questionable does not.
	if !(CaseReport{Rows: []Row{{Status: RowDivergence}}}).Failed() {
		t.Error("divergence should fail the case")
	}
	if (CaseReport{Rows: []Row{{Status: RowQuestionable}}}).Failed() {
		t.Error("questionable should not fail the case")
	}
}
