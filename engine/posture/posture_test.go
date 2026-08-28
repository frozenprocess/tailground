package posture

import (
	"strings"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.80", "1.80.0", 0},
		{"1.78.2", "1.80", -1},
		{"1.82.0", "1.80.5", 1},
		{"1.84.1-pre", "1.84.1", 0},
		{"2.0", "1.99", 1},
		{"1.80.0", "1.80.0", 0},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestGate(t *testing.T) {
	set, warn := Parse(map[string][]string{
		"posture:corp":    {"node:os == 'macos'", "node:tsVersion >= '1.80'"},
		"posture:managed": {"node:os in ['macos', 'windows']"},
		"posture:broken":  {"node:os =="}, // unparseable
	}, nil)
	if len(warn) != 1 {
		t.Fatalf("expected 1 warning, got %v", warn)
	}

	macCurrent := map[string]string{"os": "macos", "tsVersion": "1.82.0"}
	macOld := map[string]string{"os": "macos", "tsVersion": "1.70.0"}
	win := map[string]string{"os": "windows", "tsVersion": "1.90"}
	linux := map[string]string{"os": "linux", "tsVersion": "1.90"}

	corp := set.Gate([]string{"posture:corp"})
	if !corp.Allows(macCurrent) {
		t.Errorf("corp should allow up-to-date macOS: %s", corp.Explain(macCurrent))
	}
	if corp.Allows(macOld) {
		t.Errorf("corp should reject old macOS")
	}
	if corp.Allows(linux) {
		t.Errorf("corp should reject linux")
	}

	// OR across two postures.
	either := set.Gate([]string{"posture:corp", "posture:managed"})
	if !either.Allows(win) {
		t.Errorf("managed should allow windows: %s", either.Explain(win))
	}
	if either.Allows(linux) {
		t.Errorf("neither posture allows linux")
	}

	// Empty gate always allows.
	if !(Gate{}).Allows(linux) {
		t.Errorf("empty gate must allow everything")
	}

	// A broken posture never passes.
	if set.Gate([]string{"posture:broken"}).Allows(macCurrent) {
		t.Errorf("broken posture must not pass")
	}

	// Undefined posture name never passes.
	if set.Gate([]string{"posture:ghost"}).Allows(macCurrent) {
		t.Errorf("undefined posture must not pass")
	}
}

func TestCustomAttributes(t *testing.T) {
	types := map[string]string{
		"custom:env":   "string",
		"custom:score": "number",
		"custom:mdm":   "boolean",
	}
	set, warn := Parse(map[string][]string{
		"posture:prod":   {"custom:env == 'prod'"},
		"posture:low":    {"custom:score < '50'"},
		"posture:tier":   {"custom:score >= '80'"},
		"posture:mdm":    {"custom:mdm == 'true'"},
		"posture:envset": {"custom:env in ['prod', 'staging']"},
		"posture:undecl": {"custom:notdeclared == 'x'"},
	}, types)

	// The only warning is the undeclared attribute.
	if len(warn) != 1 || !strings.Contains(warn[0], "custom:notdeclared") {
		t.Fatalf("warnings = %v", warn)
	}

	prod := map[string]string{"custom:env": "prod", "custom:score": "90", "custom:mdm": "true"}
	dev := map[string]string{"custom:env": "dev", "custom:score": "9", "custom:mdm": "false"}

	check := func(name string, attrs map[string]string, want bool) {
		t.Helper()
		if got := set.Gate([]string{name}).Allows(attrs); got != want {
			t.Errorf("%s.Allows(%v) = %v, want %v — %s", name, attrs, got, want, set.Gate([]string{name}).Explain(attrs))
		}
	}
	check("posture:prod", prod, true)
	check("posture:prod", dev, false)
	// numeric: "9" < "50" must be numeric-true (lexically "9" > "50")
	check("posture:low", dev, true)
	check("posture:low", prod, false)
	// numeric: "90" >= "80" true; "9" >= "80" false
	check("posture:tier", prod, true)
	check("posture:tier", dev, false)
	check("posture:mdm", prod, true)
	check("posture:mdm", dev, false)
	check("posture:envset", prod, true)
	check("posture:envset", dev, false)
}

func TestCustomNumericWithoutAttrConfig(t *testing.T) {
	// No attrConfig — a numeric-looking comparison still behaves numerically,
	// with a warning that it's treated as a string type.
	set, warn := Parse(map[string][]string{
		"posture:x": {"custom:score > '80'"},
	}, nil)
	if len(warn) != 1 {
		t.Fatalf("want one warning, got %v", warn)
	}
	if !set.Gate([]string{"posture:x"}).Allows(map[string]string{"custom:score": "100"}) {
		t.Error(`"100" > "80" should hold`)
	}
	if set.Gate([]string{"posture:x"}).Allows(map[string]string{"custom:score": "9"}) {
		t.Error(`"9" > "80" must not hold`)
	}
}

func TestParseErrors(t *testing.T) {
	for _, bad := range []string{
		"node:os",
		"node:os = 'macos'",
		"== 'macos'",
		"node:os in 'macos'",
		"node:os == 'macos' &&",
	} {
		if _, err := parseExpr(bad); err == nil {
			t.Errorf("expected parse error for %q", bad)
		}
	}
	for _, ok := range []string{
		"node:os == 'macos'",
		"node:tsVersion >= '1.80.0'",
		"node:os == 'macos' && node:tsVersion >= '1.80'",
		"node:os in ['macos', 'windows']",
		"(node:os == 'macos') || (node:os == 'windows')",
	} {
		if _, err := parseExpr(ok); err != nil {
			t.Errorf("unexpected parse error for %q: %v", ok, err)
		}
	}
}
