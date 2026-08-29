//go:build e2e

package e2e

// Delta-debugging minimizer for a fuzz divergence.
//
// Given a generated case whose engine and tailnet verdicts disagree on one
// (src, dst, proto, port) cell, Shrink repeatedly proposes a smaller
// (policy, snapshot) and keeps any that still reproduces the *same*
// disagreement, until a full pass finds nothing smaller. Each check is one
// tailnet round-trip (a single focused probe, not the matrix), so the reducers
// are ordered biggest-cut-first and the loop is greedy (adopt + restart).

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/frozenprocess/tailground/engine/eval"
)

// ShrinkResult is a minimized divergence.
type ShrinkResult struct {
	Seed       int64
	Divergence FuzzDivergence
	Original   eval.Request
	Minimal    eval.Request
	Rounds     int // full passes
	Checks     int // tailnet reproductions run
}

// Shrink minimizes g while `want` still reproduces. reproduce is called for
// every candidate; a candidate whose policy Tailscale rejects, or that no
// longer diverges, is discarded.
func Shrink(ctx context.Context, cfg Config, run *Run, g GenCase, want FuzzDivergence, logf func(string, ...any)) (ShrinkResult, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	res := ShrinkResult{Seed: g.Seed, Divergence: want, Original: g.Request}

	reproduces := func(cand GenCase) bool {
		res.Checks++
		rep := RunFuzzCase(ctx, cfg, run, cand, &want, nil)
		if rep.SetupErr != "" {
			return false
		}
		for _, d := range rep.Divergences {
			if d == want {
				return true
			}
		}
		return false
	}

	// Confirm it still reproduces at all (filter timing / flakiness guard).
	cur := g
	if !reproduces(cur) {
		return res, fmt.Errorf("divergence %s did not reproduce on the first check", want)
	}

	for {
		res.Rounds++
		cands := shrinkCandidates(cur, want)
		logf("round %d: %d candidate reduction(s)", res.Rounds, len(cands))
		progressed := false
		for _, c := range cands {
			if ctx.Err() != nil {
				return res, ctx.Err()
			}
			if reproduces(c.gc) {
				logf("  kept: %s (checks=%d)", c.what, res.Checks)
				cur = c.gc
				progressed = true
				break
			}
		}
		if !progressed {
			break
		}
	}

	res.Minimal = cur.Request
	logf("minimized in %d round(s), %d check(s): %d→%d rules, %d→%d devices",
		res.Rounds, res.Checks,
		ruleCount(g.Request.Policy), ruleCount(cur.Request.Policy),
		len(g.Request.Snapshot.Devices), len(cur.Request.Snapshot.Devices))
	return res, nil
}

type candidate struct {
	what string
	gc   GenCase
}

// shrinkCandidates lists reductions to try, biggest cut first. Each keeps the
// diverging pair's two devices and any rule/tag they need.
func shrinkCandidates(g GenCase, want FuzzDivergence) []candidate {
	var out []candidate
	add := func(what string, req eval.Request) {
		gc := g
		gc.Request = req
		out = append(out, candidate{what, gc})
	}

	pol, err := parsePolicy(g.Request.Policy)
	if err != nil {
		return nil
	}
	devs := g.Request.Snapshot.Devices
	keepDev := map[string]bool{want.From: true, want.To: true}

	// 1. Drop a whole rule array, then each element of one.
	for _, key := range []string{"ssh", "grants", "acls"} {
		arr, ok := pol[key].([]any)
		if !ok || len(arr) == 0 {
			continue
		}
		add("drop all "+key, withPolicy(g.Request, cloneMinus(pol, key, -1)))
		for i := range arr {
			add(fmt.Sprintf("drop %s[%d]", key, i), withPolicy(g.Request, cloneMinus(pol, key, i)))
		}
	}

	// 2. Drop a device not in the diverging pair.
	if len(devs) > 2 {
		for i, d := range devs {
			if keepDev[d.Name] {
				continue
			}
			nd := append(append([]eval.Device(nil), devs[:i]...), devs[i+1:]...)
			req := g.Request
			req.Snapshot.Devices = nd
			add("drop device "+d.Name, req)
		}
	}

	// 3. Collapse a rule's port spec to just the diverging port.
	wantPort := strconv.Itoa(want.Port)
	if acls, ok := pol["acls"].([]any); ok {
		for i, ra := range acls {
			r, _ := ra.(map[string]any)
			for _, dst := range strSlice(r["dst"]) {
				if h, p, found := strings.Cut(dst, ":"); found && p != wantPort && p != "*" {
					np := cloneRules(pol, "acls")
					np[i].(map[string]any)["dst"] = []string{h + ":" + wantPort}
					add(fmt.Sprintf("acls[%d] port %s→%s", i, p, wantPort), withPolicy(g.Request, replaceArr(pol, "acls", np)))
				}
			}
		}
	}
	if grants, ok := pol["grants"].([]any); ok {
		for i, ra := range grants {
			r, _ := ra.(map[string]any)
			if ips := strSlice(r["ip"]); len(ips) != 1 || ips[0] != wantPort {
				ng := cloneRules(pol, "grants")
				ng[i].(map[string]any)["ip"] = []string{wantPort}
				add(fmt.Sprintf("grants[%d] ip→%s", i, wantPort), withPolicy(g.Request, replaceArr(pol, "grants", ng)))
			}
		}
	}

	// 4. Narrow a wildcard / autogroup src to the diverging source's tag.
	srcTag := deviceTag(devs, want.From)
	if srcTag != "" {
		for _, key := range []string{"acls", "grants"} {
			arr, ok := pol[key].([]any)
			if !ok {
				continue
			}
			for i, ra := range arr {
				r, _ := ra.(map[string]any)
				for _, s := range strSlice(r["src"]) {
					if s == "*" || s == "autogroup:tagged" {
						na := cloneRules(pol, key)
						na[i].(map[string]any)["src"] = []string{srcTag}
						add(fmt.Sprintf("%s[%d] src %s→%s", key, i, s, srcTag), withPolicy(g.Request, replaceArr(pol, key, na)))
					}
				}
			}
		}
	}

	// 5. Drop the postures block if nothing references srcPosture.
	if _, ok := pol["postures"]; ok && !anySrcPosture(pol) {
		np := map[string]any{}
		for k, v := range pol {
			if k != "postures" {
				np[k] = v
			}
		}
		add("drop postures", withPolicy(g.Request, np))
	}

	// 6. Drop tagOwners / postures entries nothing uses (tidy, keeps it valid).
	if trimmed, changed := trimUnusedTags(pol, g.Request.Snapshot.Devices); changed {
		add("trim unused tagOwners", withPolicy(g.Request, trimmed))
	}

	return out
}

// --- policy surgery helpers ---

func withPolicy(req eval.Request, pol map[string]any) eval.Request {
	b, _ := json.Marshal(pol)
	req.Policy = string(b)
	return req
}

// cloneMinus returns pol with element i of arrayKey removed (i < 0 removes the
// whole key).
func cloneMinus(pol map[string]any, key string, i int) map[string]any {
	out := map[string]any{}
	for k, v := range pol {
		out[k] = v
	}
	if i < 0 {
		delete(out, key)
		return out
	}
	arr, _ := pol[key].([]any)
	na := append(append([]any(nil), arr[:i]...), arr[i+1:]...)
	if len(na) == 0 {
		delete(out, key)
	} else {
		out[key] = na
	}
	return out
}

// cloneRules deep-ish copies pol[key] as []any of fresh maps so an element can
// be mutated without touching the original.
func cloneRules(pol map[string]any, key string) []any {
	arr, _ := pol[key].([]any)
	out := make([]any, len(arr))
	for i, ra := range arr {
		r, _ := ra.(map[string]any)
		nr := map[string]any{}
		for k, v := range r {
			nr[k] = v
		}
		out[i] = nr
	}
	return out
}

func replaceArr(pol map[string]any, key string, arr []any) map[string]any {
	out := map[string]any{}
	for k, v := range pol {
		out[k] = v
	}
	out[key] = arr
	return out
}

func strSlice(v any) []string {
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		out := make([]string, 0, len(s))
		for _, e := range s {
			if str, ok := e.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

func deviceTag(devs []eval.Device, name string) string {
	for _, d := range devs {
		if d.Name == name && len(d.Tags) > 0 {
			return d.Tags[0]
		}
	}
	return ""
}

func anySrcPosture(pol map[string]any) bool {
	for _, key := range []string{"acls", "grants", "ssh"} {
		for _, ra := range asArr(pol[key]) {
			if r, ok := ra.(map[string]any); ok {
				if _, has := r["srcPosture"]; has {
					return true
				}
			}
		}
	}
	return false
}

func asArr(v any) []any {
	a, _ := v.([]any)
	return a
}

func trimUnusedTags(pol map[string]any, devs []eval.Device) (map[string]any, bool) {
	owners, ok := pol["tagOwners"].(map[string]any)
	if !ok {
		return pol, false
	}
	used := map[string]bool{}
	for _, d := range devs {
		for _, t := range d.Tags {
			used[t] = true
		}
	}
	for _, key := range []string{"acls", "grants", "ssh"} {
		for _, ra := range asArr(pol[key]) {
			r, _ := ra.(map[string]any)
			for _, field := range []string{"src", "dst"} {
				for _, s := range strSlice(r[field]) {
					if tag, _, _ := strings.Cut(s, ":"); strings.HasPrefix(tag, "tag:") {
						used[tag] = true
					}
				}
			}
		}
	}
	no := map[string]any{}
	for t, v := range owners {
		if used[t] {
			no[t] = v
		}
	}
	if len(no) == len(owners) {
		return pol, false
	}
	out := map[string]any{}
	for k, v := range pol {
		out[k] = v
	}
	out["tagOwners"] = no
	return out, true
}

func ruleCount(policy string) int {
	m, err := parsePolicy(policy)
	if err != nil {
		return -1
	}
	n := 0
	for _, key := range []string{"acls", "grants", "ssh"} {
		n += len(asArr(m[key]))
	}
	return n
}

// MinimalCaseFiles renders a shrink result as the corpus files for
// cases/_found/, with a note recording the original seed and reduction stats.
func (r ShrinkResult) MinimalCaseFiles() (caseJSON, assertionsJSON, metaJSON []byte) {
	caseObj := map[string]any{
		"policy":   r.Minimal.Policy,
		"snapshot": map[string]any{"devices": r.Minimal.Snapshot.Devices},
	}
	caseJSON, _ = json.MarshalIndent(caseObj, "", "  ")

	a := Assertion{
		From: r.Divergence.From, To: r.Divergence.To,
		Proto: r.Divergence.Proto, Port: r.Divergence.Port,
		Expect: r.Divergence.Engine, // what the engine predicted; the tailnet said otherwise
	}
	assertionsJSON, _ = json.MarshalIndent([]Assertion{a}, "", "  ")

	meta := map[string]any{
		"title": fmt.Sprintf("fuzz divergence, seed %d (minimized)", r.Seed),
		"note": fmt.Sprintf("engine=%s tailnet=%s on %s->%s %s/%d; shrunk from seed %d in %d round(s)/%d check(s)",
			r.Divergence.Engine, r.Divergence.Tailnet, r.Divergence.From, r.Divergence.To,
			r.Divergence.Proto, r.Divergence.Port, r.Seed, r.Rounds, r.Checks),
	}
	metaJSON, _ = json.MarshalIndent(meta, "", "  ")
	return
}

// FreezeShrunk writes the minimized case under <dir>/_found/seed-<n>-min/ as a
// runnable corpus case (it has one real assertion — the engine's prediction,
// which the tailnet contradicts).
func FreezeShrunk(dir string, r ShrinkResult) (string, error) {
	out := filepath.Join(dir, "_found", fmt.Sprintf("seed-%d-min", r.Seed))
	if err := os.MkdirAll(out, 0o755); err != nil {
		return "", err
	}
	caseJSON, assertionsJSON, metaJSON := r.MinimalCaseFiles()
	for name, data := range map[string][]byte{
		"case.json":       caseJSON,
		"assertions.json": assertionsJSON,
		"meta.json":       metaJSON,
	} {
		if err := os.WriteFile(filepath.Join(out, name), data, 0o644); err != nil {
			return out, err
		}
	}
	return out, nil
}
