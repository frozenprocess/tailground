package preflight_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/frozenprocess/tailground/engine/eval"
	"github.com/frozenprocess/tailground/engine/preflight"
)

var update = flag.Bool("update", false, "regenerate testdata/preflight.golden.json")

// preflightInputs gathers every policy + snapshot pair the repo ships: the
// engine goldens, and each tutorial lesson's starting scenario and solution.
func preflightInputs(t *testing.T) map[string]eval.Request {
	t.Helper()
	in := map[string]eval.Request{}
	load := func(path string) eval.Request {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var req eval.Request
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		return req
	}

	goldens, _ := filepath.Glob(filepath.Join("..", "testdata", "golden", "*.json"))
	for _, p := range goldens {
		if filepath.Ext(p) == ".json" && filepath.Base(p) != "" && !isWant(p) {
			in["golden/"+filepath.Base(p)] = load(p)
		}
	}

	lessons, _ := filepath.Glob(filepath.Join("..", "..", "web", "src", "content", "tutorials", "*"))
	for _, dir := range lessons {
		name := filepath.Base(dir)
		scenario := filepath.Join(dir, "scenario.json")
		if _, err := os.Stat(scenario); err != nil {
			continue
		}
		req := load(scenario)
		in["lesson/"+name+"/scenario"] = req
		if sol, err := os.ReadFile(filepath.Join(dir, "solution.hujson")); err == nil {
			req.Policy = string(sol)
			in["lesson/"+name+"/solution"] = req
		}
	}
	if len(in) == 0 {
		t.Fatal("no inputs found")
	}
	return in
}

func isWant(p string) bool {
	return len(p) > len(".want.json") && p[len(p)-len(".want.json"):] == ".want.json"
}

// TestGolden pins preflight's full output (rewritten policy, diagnostics,
// posture, app caps, external share) across every shipped policy, so structural
// refactors of the rewriters can be shown to change nothing. Regenerate with
// `go test ./engine/preflight -run TestGolden -update` after an intentional change.
func TestGolden(t *testing.T) {
	inputs := preflightInputs(t)
	names := make([]string, 0, len(inputs))
	for n := range inputs {
		names = append(names, n)
	}
	sort.Strings(names)

	var buf bytes.Buffer
	for _, n := range names {
		req := inputs[n]
		res := preflight.RunWithOptions(req.Policy, preflight.Options{
			Users:            req.Snapshot.Users,
			Services:         req.Snapshot.Services,
			ExternalTailnets: req.Snapshot.ExternalTailnets,
		})
		buf.WriteString("=== " + n + "\n")
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			t.Fatal(err)
		}
	}

	path := filepath.Join("..", "testdata", "preflight.golden.txt")
	if *update {
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden (%v); run with -update", err)
	}
	if !bytes.Equal(want, buf.Bytes()) {
		t.Errorf("preflight output changed; diff %s against a -update run", path)
	}
}
