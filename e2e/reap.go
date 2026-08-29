//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ReapReport summarizes a reaper pass.
type ReapReport struct {
	Deleted   []string // displayNames deleted via their persisted creds
	Failed    []string // "displayName: error"
	Orphaned  []string // tg-e2e- tailnets in the org with no local creds file
	StaleKept []string // creds files for tailnets that no longer exist (removed)
}

func Reap(ctx context.Context, cfg Config, org *OrgClient, olderThan time.Duration) (ReapReport, error) {
	var rep ReapReport

	files, err := credsFiles(cfg.RunDir)
	if err != nil {
		return rep, err
	}

	knownIDs := map[string]bool{}
	for _, path := range files {
		rec, _, err := readCredsFile(path)
		if err != nil {
			rep.Failed = append(rep.Failed, fmt.Sprintf("%s: %v", filepath.Base(path), err))
			continue
		}
		knownIDs[rec.TailnetID] = true

		if olderThan > 0 && !rec.CreatedAt.IsZero() && time.Since(rec.CreatedAt) < olderThan {
			continue
		}
		if err := guardName(rec.DisplayName); err != nil {
			rep.Failed = append(rep.Failed, fmt.Sprintf("%s: %v", rec.DisplayName, err))
			continue
		}

		delErr := org.DeleteTailnet(ctx, cfg, rec.TailnetID, rec.OAuthClient)
		switch {
		case delErr == nil:
			rep.Deleted = append(rep.Deleted, rec.DisplayName)
			_ = os.Remove(path)
		case isNotFound(delErr):
			// Tailnet already gone; the creds file is stale.
			rep.StaleKept = append(rep.StaleKept, rec.DisplayName)
			_ = os.Remove(path)
		default:
			rep.Failed = append(rep.Failed, fmt.Sprintf("%s: %v", rec.DisplayName, delErr))
		}
	}

	// Cross-check against the org list for orphans.
	summaries, err := org.ListTailnets(ctx)
	if err != nil {
		return rep, fmt.Errorf("list org tailnets: %w", err)
	}
	for _, s := range summaries {
		if !strings.HasPrefix(s.DisplayName, TailnetPrefix) || knownIDs[s.ID] {
			continue
		}
		if olderThan > 0 && !s.CreatedAt.IsZero() && time.Since(s.CreatedAt) < olderThan {
			continue
		}
		rep.Orphaned = append(rep.Orphaned,
			fmt.Sprintf("%s (id=%s, created %s ago) — delete from the Tailscale console",
				s.DisplayName, s.ID, time.Since(s.CreatedAt).Round(time.Minute)))
	}
	return rep, nil
}

func credsFiles(runDir string) ([]string, error) {
	entries, err := os.ReadDir(runDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			out = append(out, filepath.Join(runDir, e.Name()))
		}
	}
	return out, nil
}

func readCredsFile(path string) (persistedCreds, string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return persistedCreds{}, path, err
	}
	var rec persistedCreds
	if err := json.Unmarshal(b, &rec); err != nil {
		return persistedCreds{}, path, fmt.Errorf("parse %s: %w", path, err)
	}
	if rec.TailnetID == "" || rec.OAuthClient.Secret == "" {
		return persistedCreds{}, path, fmt.Errorf("%s missing tailnetId or oauthClient", path)
	}
	return rec, path, nil
}

func isNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "404")
}

func (r ReapReport) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "reap: %d deleted, %d stale creds removed, %d orphaned, %d failed\n",
		len(r.Deleted), len(r.StaleKept), len(r.Orphaned), len(r.Failed))
	for _, d := range r.Deleted {
		fmt.Fprintf(&b, "  deleted   %s\n", d)
	}
	for _, d := range r.StaleKept {
		fmt.Fprintf(&b, "  stale     %s (already gone; creds file removed)\n", d)
	}
	for _, o := range r.Orphaned {
		fmt.Fprintf(&b, "  ORPHAN    %s\n", o)
	}
	for _, f := range r.Failed {
		fmt.Fprintf(&b, "  FAILED    %s\n", f)
	}
	return b.String()
}
