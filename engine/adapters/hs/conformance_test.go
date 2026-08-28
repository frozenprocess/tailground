package hs_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/frozenprocess/tailground/engine/adapters/hs"
	"github.com/frozenprocess/tailground/engine/eval"
)

// conformanceCase is one policy that was POSTed to Tailscale's live policy API
// (POST /api/v2/tailnet/-/acl) on two real tailnets, with the result it got.
// Tailnet IDs and DNS names are replaced with placeholders.
type conformanceCase struct {
	Case    int    `json:"case"`
	Title   string `json:"title"`
	Tailnet string `json:"tailnet"` // "sandbox-a" | "sandbox-b": which tailnet it was applied to
	Status  int    `json:"status"`  // 200 accepted, 400 rejected
	Message string `json:"message,omitempty"`
	// SameWording is true when Tailground's error contains Tailscale's
	// message verbatim; otherwise only the rejection itself is checked.
	SameWording bool            `json:"sameWording,omitempty"`
	Policy      json.RawMessage `json:"policy"`
}

// TestConformance_DeclarativeSharing replays every declarative node sharing
// case Tailscale was asked to judge: a policy it rejected must be fatal here,
// and one it accepted must not be. Each case's tailnet shares one node, tagged
// tsnet-app and network and in group:ops, under every alias the policy names
// (or the other sandbox's alias when it names none).
func TestConformance_DeclarativeSharing(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "conformance", "declarative-sharing.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []conformanceCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no cases")
	}
	for _, c := range cases {
		t.Run(strconv.Itoa(c.Case)+"-"+c.Title, func(t *testing.T) {
			var pol struct {
				ExternalTailnets map[string]json.RawMessage `json:"externalTailnets"`
			}
			if err := json.Unmarshal(c.Policy, &pol); err != nil {
				t.Fatal(err)
			}
			aliases := make([]string, 0, len(pol.ExternalTailnets))
			for a := range pol.ExternalTailnets {
				aliases = append(aliases, a)
			}
			if len(aliases) == 0 {
				other := "sandbox-b"
				if c.Tailnet == "sandbox-b" {
					other = "sandbox-a"
				}
				aliases = []string{other}
			}
			snap := eval.Snapshot{
				Users: []eval.User{{ID: 1, Email: "admin@example.com"}, {ID: 2, Email: "ops@example.com"}},
				Devices: []eval.Device{
					{ID: 1, Name: "local-app", Tags: []string{"tag:tsnet-app"}, IPv4: "100.64.0.1"},
					{ID: 2, Name: "ops-laptop", User: "ops@example.com", IPv4: "100.64.0.2"},
				},
			}
			for _, a := range aliases {
				snap.ExternalTailnets = append(snap.ExternalTailnets, eval.ExternalTailnet{
					Alias: a,
					Nodes: []eval.ExternalNode{{Name: "ext-app", Tags: []string{"tsnet-app", "network"}, Groups: []string{"ops"}}},
				})
			}

			res := hs.New().Evaluate(eval.Request{
				Policy:   string(c.Policy),
				Snapshot: snap,
				Probes:   []eval.Probe{{Name: "https", Proto: "tcp", Port: 443}},
			})
			var errs []string
			for _, d := range res.Diagnostics {
				if d.Severity == "error" {
					errs = append(errs, d.Message)
				}
			}

			switch c.Status {
			case 200:
				if len(errs) > 0 {
					t.Errorf("Tailscale accepts this policy; Tailground rejects it: %v", errs)
				}
			case 400:
				if len(errs) == 0 {
					t.Fatalf("Tailscale rejects this policy (%q); Tailground accepts it", c.Message)
				}
				if c.SameWording && !strings.Contains(strings.Join(errs, "\n"), c.Message) {
					t.Errorf("want an error containing Tailscale's %q, got %v", c.Message, errs)
				}
			default:
				t.Fatalf("bad status %d", c.Status)
			}
		})
	}
}
