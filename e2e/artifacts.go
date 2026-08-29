//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func DumpArtifacts(ctx context.Context, dir string, backend NodeBackend, nodes []*Node, rep CaseReport) {
	if dir == "" {
		return
	}
	base := filepath.Join(dir, rep.Slug)
	if err := os.MkdirAll(base, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "artifacts: %v\n", err)
		return
	}

	write := func(name string, b []byte) {
		if err := os.WriteFile(filepath.Join(base, name), b, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "artifacts %s: %v\n", name, err)
		}
	}

	write("table.txt", []byte(rep.Table()))
	if rep.pushedPolicy != "" {
		write("pushed-acl.hujson", []byte(rep.pushedPolicy))
	}
	if rep.storedPolicy != "" {
		write("stored-acl.hujson", []byte(rep.storedPolicy))
	}
	if rep.enginePolicy != "" {
		write("engine-policy.json", []byte(rep.enginePolicy))
	}
	if b, err := json.MarshalIndent(rep.engineResult, "", "  "); err == nil {
		write("engine-result.json", b)
	}
	for _, n := range nodes {
		if raw, err := backend.Netmap(ctx, n); err == nil {
			write(n.Name+".netmap.json", raw)
		}
	}
}
