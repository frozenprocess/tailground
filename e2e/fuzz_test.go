//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestE2EFuzz(t *testing.T) {
	cfg := LoadConfig()
	n := envInt("E2E_FUZZ_N", 300)
	baseSeed := envInt64("E2E_FUZZ_SEED", time.Now().UnixNano())
	params := DefaultFuzzParams()

	if envBool("E2E_FUZZ_LIVE") {
		runLiveFuzz(t, cfg, params, n, baseSeed)
		return
	}

	t.Logf("fuzz sweep (engine-only): n=%d baseSeed=%d", n, baseSeed)
	var findings int
	for i := 0; i < n; i++ {
		seed := baseSeed + int64(i)
		g := GenerateCase(seed, params)
		func() {
			defer func() {
				if r := recover(); r != nil {
					findings++
					t.Errorf("PANIC — %s\n  %v\n  policy=%s", g.Summary(), r, g.Request.Policy)
				}
			}()
			res, enginePol, err := EvaluateForEngine(g.Request)
			if err != nil {
				findings++
				t.Errorf("engine error — %s\n  %v\n  policy=%s", g.Summary(), err, g.Request.Policy)
				return
			}
			for _, d := range res.Diagnostics {
				if d.Severity == "error" {
					findings++
					t.Errorf("error diagnostic — %s\n  [%s] %s (%s)\n  engine-policy=%s",
						g.Summary(), d.Code, d.Message, d.Pointer, enginePol)
				}
			}
			if len(res.Matrices) == 0 && len(g.Request.Probes) > 0 {
				findings++
				t.Errorf("empty result — %s\n  policy=%s", g.Summary(), g.Request.Policy)
			}
		}()
	}
	t.Logf("fuzz sweep: %d pairs evaluated, %d findings", n, findings)
}

func runLiveFuzz(t *testing.T, cfg Config, params FuzzParams, n int, baseSeed int64) {
	if err := cfg.RequireLive(); err != nil {
		t.Skipf("live fuzz needs credentials: %v", err)
	}
	if cfg.Backend != "tsnet" {
		t.Skipf("live fuzz is tsnet-only (set E2E_BACKEND=tsnet), got %q", cfg.Backend)
	}
	budget := time.Duration(envInt("E2E_FUZZ_SECONDS", 0)) * time.Second
	overall := 90 * time.Minute
	if budget > 0 && budget < overall {
		overall = budget + 15*time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), overall)
	defer cancel()

	org := NewOrgClient(cfg)
	shards := cfg.Shards
	if cfg.Reuse != "" {
		shards = 1 // a reused tailnet is a single shard
	}
	if cfg.Reuse == "" {
		if rep, err := Reap(ctx, cfg, org, cfg.TailnetMaxAge); err == nil && len(rep.Deleted) > 0 {
			t.Logf("reaper: %s", rep.String())
		}
	}

	var acc fuzzAcc
	deadline := time.Time{}
	if budget > 0 {
		deadline = time.Now().Add(budget)
	}
	baseRunID := NewRunID()
	t.Logf("live fuzz: n=%d baseSeed=%d shards=%d budget=%s", n, baseSeed, shards, budget)

	var wg sync.WaitGroup
	for s := 0; s < shards; s++ {
		wg.Add(1)
		go func(s int) {
			defer wg.Done()
			runID := baseRunID
			if shards > 1 {
				runID = fmt.Sprintf("%s-s%d", baseRunID, s)
			}
			run, err := ProvisionOrReuse(ctx, cfg, org, runID)
			if err != nil {
				t.Errorf("shard %d provision: %v", s, err)
				return
			}
			t.Logf("shard %d: child tailnet %s (id=%s)", s, run.Child.DisplayName, run.ID)
			defer func() {
				if cfg.Keep || t.Failed() {
					run.Keep()
				}
				if err := run.Teardown(context.Background()); err != nil {
					t.Errorf("shard %d teardown: %v", s, err)
				}
			}()

			for i := s; i < n; i += shards {
				if ctx.Err() != nil || (!deadline.IsZero() && time.Now().After(deadline)) {
					t.Logf("shard %d: stopping (budget/context)", s)
					return
				}
				fuzzOne(ctx, t, cfg, run, params, baseSeed+int64(i), s, &acc)
			}
		}(s)
	}
	wg.Wait()

	c, p, d, b := acc.totals()
	t.Logf("live fuzz: %d cases, %d probes, %d divergences, %d rejected policies", c, p, d, b)
}

// fuzzAcc accumulates sweep totals across shards.
type fuzzAcc struct {
	mu                            sync.Mutex
	cases, probes, divs, badPolls int
}

func (a *fuzzAcc) add(cases, probes, divs, bad int) {
	a.mu.Lock()
	a.cases += cases
	a.probes += probes
	a.divs += divs
	a.badPolls += bad
	a.mu.Unlock()
}

func (a *fuzzAcc) totals() (int, int, int, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cases, a.probes, a.divs, a.badPolls
}

// fuzzOne runs and reports one generated case. testing.T methods are safe for
// concurrent use, so shards call this directly.
func fuzzOne(ctx context.Context, t *testing.T, cfg Config, run *Run, params FuzzParams, seed int64, shard int, acc *fuzzAcc) {
	g := GenerateCase(seed, params)
	tag := fmt.Sprintf("s%d seed %d", shard, seed)
	t.Logf("[%s] %s …", tag, g.Summary())
	started := time.Now()
	phase := func(f string, a ...any) { t.Logf("    [%s] "+f, append([]any{tag}, a...)...) }

	rep := RunFuzzCase(ctx, cfg, run, g, nil, phase)
	if rep.SetupErr != "" {
		if strings.Contains(rep.SetupErr, "policy rejected") || strings.Contains(rep.SetupErr, "push policy") {
			acc.add(1, rep.Probed, 0, 1)
			t.Errorf("[%s] generator produced a policy Tailscale rejected: %s\n  policy=%s", tag, rep.SetupErr, g.Request.Policy)
		} else {
			acc.add(1, rep.Probed, 0, 0)
			t.Logf("[%s] SETUP FAILED after %s: %s", tag, time.Since(started).Round(time.Second), rep.SetupErr)
		}
		return
	}
	acc.add(1, rep.Probed, len(rep.Divergences), 0)
	t.Logf("[%s] done in %s — nodes=[%s] filter-landed=%v probes=%d divergences=%d errors=%d",
		tag, time.Since(started).Round(time.Second),
		strings.Join(rep.Nodes, " "), rep.FilterLanded, rep.Probed, len(rep.Divergences), len(rep.Errors))
	for _, e := range rep.Errors {
		t.Logf("    [%s] probe error: %s", tag, e)
	}
	if len(rep.Divergences) > 0 {
		var b strings.Builder
		for _, d := range rep.Divergences {
			b.WriteString("    " + d.String() + "\n")
		}
		frozen := ""
		if cfg.Artifacts != "" {
			if p, err := FreezeFound(cfg.Artifacts, g, rep); err == nil {
				frozen = " (frozen: " + p + ")"
			}
		}
		t.Errorf("[%s] DIVERGENCE — %s%s\n%s  policy=%s", tag, g.Summary(), frozen, b.String(), rep.Policy)
	}
}

// TestFuzzPoliciesValid checks generated policies against Tailscale's own ACL
// validator — no nodes, ~200ms/policy. Gated on creds; `E2E_FUZZ_N` policies
// from `E2E_FUZZ_SEED`. Catches generator bugs the engine is too lenient to
// flag (the engine accepts constructs real Tailscale rejects).
func TestFuzzPoliciesValid(t *testing.T) {
	cfg := LoadConfig()
	if err := cfg.RequireLive(); err != nil {
		t.Skipf("needs credentials: %v", err)
	}
	n := envInt("E2E_FUZZ_N", 60)
	baseSeed := envInt64("E2E_FUZZ_SEED", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	org := NewOrgClient(cfg)
	run, err := ProvisionOrReuse(ctx, cfg, org, NewRunID())
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	defer func() {
		if !cfg.Keep {
			_ = run.Teardown(context.Background())
		}
	}()
	t.Logf("validating %d generated policies from seed %d against tailnet %s", n, baseSeed, run.ID)

	params := DefaultFuzzParams()
	var bad int
	for i := 0; i < n; i++ {
		seed := baseSeed + int64(i)
		g := GenerateCase(seed, params)
		if err := run.TS().ValidateACL(ctx, g.Request.Policy); err != nil {
			bad++
			t.Errorf("seed %d: %v\n  %s\n  policy=%s", seed, err, g.Summary(), g.Request.Policy)
		}
	}
	t.Logf("%d policies validated, %d rejected", n, bad)
}

// TestE2EShrink minimizes a known-diverging seed: generate it, find the
// divergence live, delta-debug the (policy, snapshot) down while it still
// reproduces, and freeze the result under E2E_ARTIFACTS/_found/seed-<n>-min/.
// Gated on E2E_SHRINK_SEED (a seed a sweep flagged).
func TestE2EShrink(t *testing.T) {
	cfg := LoadConfig()
	seed := envInt64("E2E_SHRINK_SEED", 0)
	if seed == 0 {
		t.Skip("set E2E_SHRINK_SEED=<a seed the sweep flagged>")
	}
	if err := cfg.RequireLive(); err != nil {
		t.Skipf("needs credentials: %v", err)
	}
	if cfg.Backend != "tsnet" {
		t.Skipf("shrink is tsnet-only (set E2E_BACKEND=tsnet), got %q", cfg.Backend)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
	defer cancel()

	org := NewOrgClient(cfg)
	run, err := ProvisionOrReuse(ctx, cfg, org, NewRunID())
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	defer func() {
		if cfg.Keep {
			run.Keep()
		}
		_ = run.Teardown(context.Background())
	}()

	g := GenerateCase(seed, DefaultFuzzParams())
	t.Logf("seed %d — %s", seed, g.Summary())
	rep := RunFuzzCase(ctx, cfg, run, g, nil, func(f string, a ...any) { t.Logf("  "+f, a...) })
	if rep.SetupErr != "" {
		t.Fatalf("seed %d setup failed: %s", seed, rep.SetupErr)
	}
	if len(rep.Divergences) == 0 {
		t.Fatalf("seed %d did not diverge (%d probes) — nothing to shrink", seed, rep.Probed)
	}
	want := rep.Divergences[0]
	t.Logf("shrinking: %s", want)

	sr, err := Shrink(ctx, cfg, run, g, want, func(f string, a ...any) { t.Logf("  "+f, a...) })
	if err != nil {
		t.Fatalf("shrink: %v", err)
	}
	t.Logf("minimal policy: %s", sr.Minimal.Policy)
	t.Logf("minimal devices: %d", len(sr.Minimal.Snapshot.Devices))
	if cfg.Artifacts != "" {
		if p, err := FreezeShrunk(cfg.Artifacts, sr); err != nil {
			t.Errorf("freeze: %v", err)
		} else {
			t.Logf("frozen: %s", p)
		}
	}
}

// TestShrinkCandidates checks the reducers offline: every candidate parses,
// is no larger than the input, and keeps the diverging pair's devices.
func TestShrinkCandidates(t *testing.T) {
	for _, seed := range []int64{7, 100, 2009, 55} {
		g := GenerateCase(seed, DefaultFuzzParams())
		devs := g.Request.Snapshot.Devices
		if len(devs) < 2 {
			continue
		}
		want := FuzzDivergence{From: devs[0].Name, To: devs[1].Name, Proto: "tcp", Port: 443, Engine: "allow", Tailnet: "deny"}
		baseRules := ruleCount(g.Request.Policy)
		cands := shrinkCandidates(g, want)
		for _, c := range cands {
			if _, err := parsePolicy(c.gc.Request.Policy); err != nil {
				t.Errorf("seed %d: candidate %q does not parse: %v", seed, c.what, err)
			}
			if rc := ruleCount(c.gc.Request.Policy); rc > baseRules {
				t.Errorf("seed %d: candidate %q grew rules %d→%d", seed, c.what, baseRules, rc)
			}
			names := map[string]bool{}
			for _, d := range c.gc.Request.Snapshot.Devices {
				names[d.Name] = true
			}
			if !names[want.From] || !names[want.To] {
				t.Errorf("seed %d: candidate %q dropped a diverging-pair device", seed, c.what)
			}
		}
	}
}

// TestFuzzDeterministic checks the generator is a pure function of its seed —
// shrink.go and CI reproduction depend on it.
func TestFuzzDeterministic(t *testing.T) {
	p := DefaultFuzzParams()
	for _, seed := range []int64{1, 42, 1 << 20, -7} {
		a := GenerateCase(seed, p)
		b := GenerateCase(seed, p)
		if a.Request.Policy != b.Request.Policy {
			t.Errorf("seed %d: policy not deterministic\n  a=%s\n  b=%s", seed, a.Request.Policy, b.Request.Policy)
		}
		if len(a.Request.Snapshot.Devices) != len(b.Request.Snapshot.Devices) {
			t.Errorf("seed %d: device count not deterministic", seed)
		}
	}
}
