package hs_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frozenprocess/tailground/engine/adapters/hs"
	"github.com/frozenprocess/tailground/engine/eval"
)

var update = flag.Bool("update", false, "regenerate testdata/golden/*.want.json")

// TestGolden runs every testdata/golden/<name>.json Request through the engine
// and compares the Result to <name>.want.json. Run `go test ./engine/adapters/hs
// -run TestGolden -update` to regenerate after an intentional change.
func TestGolden(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "golden")
	reqs, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, reqPath := range reqs {
		if strings.HasSuffix(reqPath, ".want.json") {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(reqPath), ".json")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(reqPath)
			if err != nil {
				t.Fatal(err)
			}
			var req eval.Request
			if err := json.Unmarshal(raw, &req); err != nil {
				t.Fatalf("bad request json: %v", err)
			}

			got := hs.New().Evaluate(req)
			got.Engine = eval.EngineInfo{} // build-specific; not compared

			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(false)
			enc.SetIndent("", "  ")
			if err := enc.Encode(got); err != nil {
				t.Fatal(err)
			}
			gotJSON := buf.Bytes()

			wantPath := strings.TrimSuffix(reqPath, ".json") + ".want.json"
			if *update {
				if err := os.WriteFile(wantPath, gotJSON, 0o644); err != nil {
					t.Fatal(err)
				}
				t.Logf("updated %s", wantPath)
				return
			}

			want, err := os.ReadFile(wantPath)
			if err != nil {
				t.Fatalf("missing golden (%v); run with -update", err)
			}
			if string(want) != string(gotJSON) {
				t.Errorf("result mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, gotJSON, want)
			}
		})
	}
}
