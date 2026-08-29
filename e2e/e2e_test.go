//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"testing"
	"time"
)

func TestEngineCases(t *testing.T) {
	cfg := LoadConfig()
	cases, err := DiscoverCases(cfg.CaseDir, cfg.Case)
	if err != nil {
		t.Fatalf("discover cases: %v", err)
	}
	if len(cases) == 0 {
		t.Fatalf("no cases matched %q under %s", cfg.Case, cfg.CaseDir)
	}
	for _, c := range cases {
		t.Run(c.Slug, func(t *testing.T) {
			if r := c.EngineCantModel(); r != "" {
				t.Skip(r)
			}
			req := c.Request
			req.Probes = probesFromAssertions(c.Assertions)
			req.SSHUsers = sshLoginUsers(c.Assertions)
			res, _, err := EvaluateForEngine(req)
			if err != nil {
				t.Fatalf("engine: %v", err)
			}
			for _, d := range res.Diagnostics {
				if d.Severity == "error" {
					t.Errorf("engine diagnostic: [%s] %s", d.Code, d.Message)
				}
			}
			byID := actorIndex(res)
			for _, a := range c.Assertions {
				var got string
				switch {
				case a.SSH:
					got = string(engineSSH(res, a.User, byID[a.From], byID[a.To]))
				case a.ToRoute != "":
					p, perr := netip.ParsePrefix(a.ToRoute)
					if perr != nil {
						t.Errorf("%s: bad toRoute: %v", a.key(), perr)
						continue
					}
					got = engineRouteVerdict(res, probeName(a), byID[a.From], byID[a.To], p.Masked())
				case a.ToService != "":
					got = engineServiceVerdict(res, probeName(a), byID[a.From], a.ToService)
				default:
					got = verdict(res, probeName(a), byID[a.From], byID[a.To])
				}
				if got != a.Expect {
					t.Errorf("%s: engine=%s authored=%s", a.key(), got, a.Expect)
				}
			}
		})
	}
}

// Probed with real L4 connections, diffed against the engine.
func TestE2E(t *testing.T) {
	cfg := LoadConfig()
	if err := cfg.RequireLive(); err != nil {
		t.Skipf("skipping live e2e: %v", err)
	}
	switch cfg.Backend {
	case "docker", "docker-userspace", "tsnet":
	default:
		t.Skipf("unknown E2E_BACKEND %q (want docker|docker-userspace|tsnet)", cfg.Backend)
	}

	cases, err := DiscoverCases(cfg.CaseDir, cfg.Case)
	if err != nil {
		t.Fatalf("discover cases: %v", err)
	}
	if len(cases) == 0 {
		t.Fatalf("no cases matched %q under %s", cfg.Case, cfg.CaseDir)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Minute)
	defer cancel()

	org := NewOrgClient(cfg)
	runID := NewRunID()

	if cfg.Reuse == "" {
		if rep, err := Reap(ctx, cfg, org, cfg.TailnetMaxAge); err != nil {
			t.Logf("reaper: %v", err)
		} else if len(rep.Deleted)+len(rep.Orphaned)+len(rep.Failed) > 0 {
			t.Logf("reaper:\n%s", rep.String())
		}
	}

	run, err := ProvisionOrReuse(ctx, cfg, org, runID)
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	t.Logf("child tailnet %s (id=%s dnsName=%s)", run.Child.DisplayName, run.ID, run.Child.DNSName)
	defer func() {
		if cfg.Keep || t.Failed() {
			run.Keep()
		}
		if err := run.Teardown(context.Background()); err != nil {
			t.Errorf("teardown: %v", err)
		}
	}()

	suite := Suite{Skipped: map[string]string{}}
	for _, c := range cases {
		c := c
		if reason := c.SkipReason(cfg); reason != "" {
			suite.Skipped[c.Slug] = reason
			t.Run(c.Slug, func(t *testing.T) { t.Skip(reason) })
			continue
		}
		t.Run(c.Slug, func(t *testing.T) {
			rep := RunCase(ctx, cfg, run, c, func(f string, a ...any) { t.Logf(f, a...) })
			suite.Reports = append(suite.Reports, rep)
			t.Log("\n" + rep.Table())
			if rep.Failed() {
				t.Fail()
			}
		})
	}

	tally := suite.Tally()
	t.Log("\n" + tally)
	if cfg.Report != "" {
		if err := appendReport(cfg.Report, runID, suite); err != nil {
			t.Errorf("write E2E_REPORT: %v", err)
		}
	}
}

// TestReap deletes every child tailnet the harness owns (via its persisted
// creds) and reports any it can't reach. Gated on E2E_REAP=1; `make e2e-down`.
func TestReap(t *testing.T) {
	if !envBool("E2E_REAP") {
		t.Skip("set E2E_REAP=1 (or run `make e2e-down`) to reap child tailnets")
	}
	cfg := LoadConfig()
	if err := cfg.RequireLive(); err != nil {
		t.Skipf("no org credentials: %v", err)
	}
	age := time.Duration(0)
	if os.Getenv("E2E_TAILNET_MAX_AGE") != "" {
		age = cfg.TailnetMaxAge
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	rep, err := Reap(ctx, cfg, NewOrgClient(cfg), age)
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	t.Logf("\n%s", rep.String())
	if len(rep.Failed) > 0 {
		t.Errorf("%d tailnet(s) could not be deleted", len(rep.Failed))
	}
}

func appendReport(path, runID string, s Suite) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "=== e2e run %s @ %s ===\n", runID, time.Now().UTC().Format(time.RFC3339))
	for _, r := range s.Reports {
		fmt.Fprintln(f, r.Table())
	}
	fmt.Fprintln(f, s.Tally())
	return nil
}
