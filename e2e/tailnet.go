//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Run is one provisioned child tailnet plus everything scoped to it. Create it
// with Provision (or ProvisionOrReuse), always defer Teardown.
type Run struct {
	cfg    Config
	org    *OrgClient
	ID     string
	RunID  string
	Child  *ChildTailnet
	ts     *tsClient
	creds  string
	kept   bool
	reused bool // came from E2E_REUSE — Teardown must not delete it
}

// ProvisionOrReuse returns a Run against a fresh child tailnet, or — when
// cfg.Reuse is set — against an already-created one whose persisted creds it
// loads (see Config.Reuse). A reused tailnet is never deleted or cleared by
// Teardown.
func ProvisionOrReuse(ctx context.Context, cfg Config, org *OrgClient, runID string) (*Run, error) {
	if cfg.Reuse == "" {
		return Provision(ctx, cfg, org, runID)
	}
	rec, path, err := loadReuseCreds(cfg)
	if err != nil {
		return nil, fmt.Errorf("E2E_REUSE: %w", err)
	}
	child := &ChildTailnet{
		ID:          rec.TailnetID,
		DisplayName: rec.DisplayName,
		DNSName:     rec.DNSName,
		OAuthClient: rec.OAuthClient,
	}
	if err := guardName(child.DisplayName); err != nil {
		return nil, err
	}
	ts, err := newChildClient(cfg, child.OAuthClient)
	if err != nil {
		return nil, err
	}
	return &Run{
		cfg: cfg, org: org, ID: child.ID, RunID: rec.RunID,
		Child: child, ts: ts, creds: path, reused: true,
	}, nil
}

// loadReuseCreds resolves cfg.Reuse to a persisted creds record: a path, or
// "1"/"true" meaning the newest *.json in RunDir.
func loadReuseCreds(cfg Config) (persistedCreds, string, error) {
	path := cfg.Reuse
	if path == "1" || path == "true" {
		entries, err := os.ReadDir(cfg.RunDir)
		if err != nil {
			return persistedCreds{}, "", fmt.Errorf("read %s: %w", cfg.RunDir, err)
		}
		var names []string
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
				names = append(names, e.Name())
			}
		}
		if len(names) == 0 {
			return persistedCreds{}, "", fmt.Errorf("no *.json in %s", cfg.RunDir)
		}
		sort.Strings(names) // run ids are timestamps → lexical == chronological
		path = filepath.Join(cfg.RunDir, names[len(names)-1])
	}
	return readCredsFile(path)
}

type persistedCreds struct {
	RunID       string          `json:"runId"`
	TailnetID   string          `json:"tailnetId"`
	DisplayName string          `json:"displayName"`
	DNSName     string          `json:"dnsName"`
	OAuthClient ChildOAuthCreds `json:"oauthClient"`
	CreatedAt   time.Time       `json:"createdAt"`
}

// Provision creates a fresh child tailnet, writes its creds to
// ${E2E_RUN_DIR}/<runid>.json, and returns a Run scoped to it.
func Provision(ctx context.Context, cfg Config, org *OrgClient, runID string) (*Run, error) {
	display := TailnetPrefix + runID
	child, err := org.CreateTailnet(ctx, display)
	if err != nil {
		return nil, err
	}

	r := &Run{cfg: cfg, org: org, ID: child.ID, RunID: runID, Child: child}

	if err := r.writeCreds(); err != nil {
		// A tailnet exists but we couldn't record how to delete it — tear it
		// down immediately rather than leak it.
		_ = org.DeleteTailnet(ctx, cfg, child.ID, child.OAuthClient)
		return nil, fmt.Errorf("persist creds: %w", err)
	}

	ts, err := newChildClient(cfg, child.OAuthClient)
	if err != nil {
		_ = r.Teardown(ctx)
		return nil, err
	}
	r.ts = ts
	return r, nil
}

// TS returns the tailnet-scoped API client.
func (r *Run) TS() *tsClient { return r.ts }

func (r *Run) Keep() { r.kept = true }

func (r *Run) Teardown(ctx context.Context) error {
	if r.reused {
		fmt.Fprintf(os.Stderr, "E2E_REUSE: leaving tailnet %s (%s) and creds %s in place\n",
			r.ID, r.Child.DisplayName, r.creds)
		return nil
	}
	if r.kept {
		fmt.Fprintf(os.Stderr, "E2E_KEEP: leaving tailnet %s (%s); reuse with E2E_REUSE=%s\n",
			r.ID, r.Child.DisplayName, r.creds)
		return nil
	}
	if delErr := r.org.DeleteTailnet(ctx, r.cfg, r.ID, r.Child.OAuthClient); delErr != nil {
		// Leave the creds file so the tailnet can be deleted by hand.
		return fmt.Errorf("delete tailnet %s (creds kept at %s): %w", r.ID, r.creds, delErr)
	}
	if r.creds != "" {
		if err := os.Remove(r.creds); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove creds file: %w", err)
		}
	}
	return nil
}

func (r *Run) writeCreds() error {
	if err := os.MkdirAll(r.cfg.RunDir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(r.cfg.RunDir, r.RunID+".json")
	rec := persistedCreds{
		RunID:       r.RunID,
		TailnetID:   r.Child.ID,
		DisplayName: r.Child.DisplayName,
		DNSName:     r.Child.DNSName,
		OAuthClient: r.Child.OAuthClient,
		CreatedAt:   time.Now().UTC(),
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return err
	}
	r.creds = path
	return nil
}

// NewRunID returns a short, filesystem- and displayName-safe run identifier.
func NewRunID() string {
	return time.Now().UTC().Format("20060102-150405")
}
