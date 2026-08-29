//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/frozenprocess/tailground/engine/eval"
)

// FuzzParams bounds the generator.
type FuzzParams struct {
	MinDevices int // default 2
	MaxDevices int // default 4
	MaxTags    int // distinct tags in the pool, default 4
	MaxACLs    int // default 4
	MaxGrants  int // default 3
	MaxSSH     int // default 2 (0 disables ssh rules)
	Posture    bool
	Wildcards  bool // allow "*" src and "*" ports
}

// DefaultFuzzParams is the standard sweep envelope.
func DefaultFuzzParams() FuzzParams {
	return FuzzParams{
		MinDevices: 2, MaxDevices: 4, MaxTags: 4,
		MaxACLs: 4, MaxGrants: 3, MaxSSH: 2,
		Posture: true, Wildcards: true,
	}
}

// GenCase is one generated pair plus the metadata a shrinker / serializer needs.
type GenCase struct {
	Seed    int64
	Request eval.Request
	Tags    []string // the tag pool
	Ports   []int    // concrete ports referenced by some rule
	SSH     bool     // policy declares an ssh block
	Posture bool     // policy declares postures + at least one srcPosture
}

var fuzzTagNames = []string{"tag:a", "tag:b", "tag:c", "tag:d", "tag:e"}

var fuzzPostures = map[string]string{
	"posture:linux":  "node:os == 'linux'",
	"posture:mac":    "node:os == 'macos'",
	"posture:cur":    "node:tsVersion >= '1.0.0'",
	"posture:future": "node:tsVersion >= '99.0.0'",
}

// GenerateCase produces the deterministic (policy, snapshot) pair for seed.
func GenerateCase(seed int64, p FuzzParams) GenCase {
	if p.MaxDevices < p.MinDevices {
		p.MaxDevices = p.MinDevices
	}
	rng := rand.New(rand.NewSource(seed))

	nDev := p.MinDevices + rng.Intn(p.MaxDevices-p.MinDevices+1)
	if nDev < 2 {
		nDev = 2
	}

	// --- tag pool: up to one more tag than there are devices, so rules mostly
	//     reference a tag some device carries but sometimes hit an orphan. ---
	nTags := 2
	if p.MaxTags > 2 {
		nTags += rng.Intn(p.MaxTags - 1)
	}
	if nTags > nDev+1 {
		nTags = nDev + 1
	}
	if nTags > len(fuzzTagNames) {
		nTags = len(fuzzTagNames)
	}
	tags := append([]string(nil), fuzzTagNames[:nTags]...)
	devs := make([]eval.Device, nDev)
	for i := range devs {
		tag := tags[rng.Intn(len(tags))]
		if i < len(tags) {
			tag = tags[i] // spread the first devices across distinct tags
		}
		devs[i] = eval.Device{
			ID:      uint64(i + 1),
			Name:    fmt.Sprintf("node-%c", 'a'+i),
			Tags:    []string{tag},
			IPv4:    fmt.Sprintf("100.64.0.%d", i+1),
			IPv6:    fmt.Sprintf("fd7a:115c:a1e0::%d", i+1),
			OS:      "linux",
			Posture: map[string]string{"tsVersion": "1.100.0"},
		}
	}

	// --- policy ---
	pol := map[string]any{}
	owners := map[string]any{}
	for _, t := range tags {
		owners[t] = []string{"autogroup:admin"}
	}
	pol["tagOwners"] = owners

	usePosture := p.Posture && rng.Intn(3) == 0
	if usePosture {
		ps := map[string]any{}
		for name, cond := range fuzzPostures {
			ps[name] = []string{cond}
		}
		pol["postures"] = ps
	}
	postureNames := []string{"posture:linux", "posture:mac", "posture:cur", "posture:future"}

	ports := map[int]bool{}
	srcTerm := func() string {
		switch {
		case p.Wildcards && rng.Intn(4) == 0:
			return "*"
		case rng.Intn(6) == 0:
			return "autogroup:tagged"
		default:
			return tags[rng.Intn(len(tags))]
		}
	}
	// portSpec returns a Tailscale-valid port spec: "*", a single port, one
	// range, or a comma list of distinct single ports. Tailscale rejects a
	// spec with more than one range ("only one port range allowed") or a
	// repeated port, so the list branch shuffles a pool and takes a slice.
	portSpec := func() string {
		switch rng.Intn(6) {
		case 0:
			if p.Wildcards {
				return "*"
			}
			fallthrough
		case 1, 2:
			n := fuzzPort(rng)
			ports[n] = true
			return fmt.Sprint(n)
		case 3:
			lo := 8000 + rng.Intn(1000)
			hi := lo + 1 + rng.Intn(200)
			ports[lo] = true
			return fmt.Sprintf("%d-%d", lo, hi)
		default:
			pool := []int{22, 80, 443, 3306, 5432, 8080, 6379}
			rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
			k := 2 + rng.Intn(2)
			strs := make([]string, k)
			for i, n := range pool[:k] {
				ports[n] = true
				strs[i] = fmt.Sprint(n)
			}
			return strings.Join(strs, ",")
		}
	}
	maybePosture := func(rule map[string]any, src string) {
		if usePosture && src != "*" && rng.Intn(2) == 0 {
			rule["srcPosture"] = []string{postureNames[rng.Intn(len(postureNames))]}
		}
	}

	var acls []any
	for i, n := 0, rng.Intn(p.MaxACLs+1); i < n; i++ {
		src := srcTerm()
		rule := map[string]any{
			"action": "accept",
			"src":    []string{src},
			"dst":    []string{tags[rng.Intn(len(tags))] + ":" + portSpec()},
		}
		maybePosture(rule, src)
		acls = append(acls, rule)
	}
	if len(acls) > 0 {
		pol["acls"] = acls
	}

	grantIP := func() []string {
		if p.Wildcards && rng.Intn(6) == 0 {
			return []string{"*"}
		}
		if rng.Intn(4) == 0 {
			lo := 8000 + rng.Intn(1000)
			hi := lo + 1 + rng.Intn(200)
			ports[lo] = true
			return []string{fmt.Sprintf("%d-%d", lo, hi)}
		}
		pool := []int{22, 80, 443, 3306, 5432, 8080, 6379}
		rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
		k := 1 + rng.Intn(3)
		out := make([]string, k)
		for i, n := range pool[:k] {
			ports[n] = true
			out[i] = fmt.Sprint(n)
		}
		return out
	}

	var grants []any
	for i, n := 0, rng.Intn(p.MaxGrants+1); i < n; i++ {
		src := srcTerm()
		rule := map[string]any{
			"src": []string{src},
			"dst": []string{tags[rng.Intn(len(tags))]},
			"ip":  grantIP(),
		}
		maybePosture(rule, src)
		grants = append(grants, rule)
	}
	if len(grants) > 0 {
		pol["grants"] = grants
	}

	hasSSH := false
	if p.MaxSSH > 0 && rng.Intn(2) == 0 {
		var ssh []any
		for i, n := 0, 1+rng.Intn(p.MaxSSH); i < n; i++ {
			users := []string{"root"}
			if rng.Intn(2) == 0 {
				users = []string{"autogroup:nonroot"}
			}
			ssh = append(ssh, map[string]any{
				"action": "accept",
				"src":    []string{tags[rng.Intn(len(tags))]},
				"dst":    []string{tags[rng.Intn(len(tags))]},
				"users":  users,
			})
		}
		if len(ssh) > 0 {
			pol["ssh"] = ssh
			hasSSH = true
		}
	}

	raw, _ := json.Marshal(pol)

	// --- probes: every concrete port a rule named, plus a fixed baseline and
	//     one port nothing references (a guaranteed deny), tcp + one udp. ---
	for _, base := range []int{22, 80, 443} {
		ports[base] = true
	}
	ports[59999] = true // referenced by nothing
	portList := make([]int, 0, len(ports))
	for pt := range ports {
		portList = append(portList, pt)
	}
	sort.Ints(portList)

	var probes []eval.Probe
	for _, pt := range portList {
		probes = append(probes, eval.Probe{Name: fmt.Sprintf("tcp/%d", pt), Proto: "tcp", Port: pt})
	}
	if len(portList) > 0 {
		udp := portList[rng.Intn(len(portList))]
		probes = append(probes, eval.Probe{Name: fmt.Sprintf("udp/%d", udp), Proto: "udp", Port: udp})
	}

	req := eval.Request{
		Policy:   string(raw),
		Snapshot: eval.Snapshot{Devices: devs},
		Probes:   probes,
	}
	if hasSSH {
		req.SSHUsers = []string{"root", "dev"}
	}

	return GenCase{
		Seed: seed, Request: req, Tags: tags, Ports: portList,
		SSH: hasSSH, Posture: usePosture,
	}
}

func fuzzPort(rng *rand.Rand) int {
	common := []int{22, 80, 443, 3306, 5432, 8080, 6379}
	return common[rng.Intn(len(common))]
}

// Files renders the generated pair as the three corpus files (case.json,
// assertions.json, meta.json) so a divergence can be frozen under
// cases/_found/<slug>/. Assertions are empty — the sweep compares engine vs
// tailnet directly, it has no authored oracle.
func (g GenCase) Files(title string) (caseJSON, assertionsJSON, metaJSON []byte) {
	caseObj := map[string]any{
		"policy":   g.Request.Policy,
		"snapshot": map[string]any{"devices": g.Request.Snapshot.Devices},
	}
	caseJSON, _ = json.MarshalIndent(caseObj, "", "  ")
	assertionsJSON = []byte("[]\n")
	meta := map[string]any{
		"title": title,
		"note":  fmt.Sprintf("generated by fuzz.go, seed %d", g.Seed),
	}
	if g.SSH {
		meta["needs"] = []string{"ssh"}
	}
	metaJSON, _ = json.MarshalIndent(meta, "", "  ")
	return
}

// --- live differential sweep ---

// fuzzMaxProbePorts caps how many ports a live fuzz case probes — the sweep
// value is coverage across many policies, not exhaustive port coverage of one.
const fuzzMaxProbePorts = 3

// fuzzProbeBudget bounds one (src,dst,port) probe. A denied flow otherwise
// burns the full retry backoff; here it costs at most this then reads deny.
// ~2 dial attempts (ProbeTimeout each) fit — enough to shrug off a transient
// hiccup on an allowed flow without paying the full backoff on a denied one.
const fuzzProbeBudget = 4 * time.Second

// FuzzDivergence is one (src,dst,proto,port) where the engine and the tailnet
// disagreed.
type FuzzDivergence struct {
	From, To string
	Proto    string
	Port     int
	Engine   string
	Tailnet  string
}

func (d FuzzDivergence) String() string {
	return fmt.Sprintf("%s->%s %s/%d  engine=%s tailnet=%s", d.From, d.To, d.Proto, d.Port, d.Engine, d.Tailnet)
}

// FuzzReport is the outcome of one live fuzz case.
type FuzzReport struct {
	Seed         int64
	Nodes        []string
	FilterLanded bool
	Probed       int
	Divergences  []FuzzDivergence
	Errors       []string
	SetupErr     string
	Policy       string // the pushed (rewritten) policy
}

// RunFuzzCase pushes one generated pair to the tailnet and diffs every ordered
// (src,dst) pair on every probed port against the engine matrix.
func RunFuzzCase(ctx context.Context, cfg Config, run *Run, g GenCase, focus *FuzzDivergence, logf func(string, ...any)) FuzzReport {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	rep := FuzzReport{Seed: g.Seed}
	fail := func(format string, a ...any) FuzzReport {
		rep.SetupErr = fmt.Sprintf(format, a...)
		return rep
	}

	req := g.Request
	if focus != nil {
		req.Probes = []eval.Probe{{Name: focus.Proto + "/" + fmt.Sprint(focus.Port), Proto: focus.Proto, Port: focus.Port}}
	} else {
		req.Probes = capProbes(req.Probes, fuzzMaxProbePorts)
	}

	// Fail fast on a policy real Tailscale won't accept — before spending a
	// minute bringing up nodes. A rejection here is a generator bug.
	if err := run.TS().ValidateACL(ctx, req.Policy); err != nil {
		return fail("policy rejected: %v", err)
	}

	if err := provision(ctx, run, req.Policy); err != nil {
		return fail("%v", err)
	}
	logf("provisioning policy pushed, minting keys")

	backend, err := newBackend(ctx, cfg, fmt.Sprintf("%s-fz%d", run.RunID, g.Seed))
	if err != nil {
		return fail("node backend: %v", err)
	}
	var nodes []*Node
	defer func() { _ = backend.Stop(context.Background()) }()

	keyByTagSet, err := mintTagSetKeys(ctx, run, req.Snapshot.Devices)
	if err != nil {
		return fail("%v", err)
	}

	specs := make([]NodeSpec, 0, len(req.Snapshot.Devices))
	for _, d := range req.Snapshot.Devices {
		specs = append(specs, NodeSpec{Name: d.Name, Tags: d.Tags, AuthKey: keyByTagSet[tagSetKey(d.Tags)]})
	}

	logf("launching %d node(s)", len(specs))
	nodes, err = backend.Launch(ctx, specs, probePortInts(req.Probes), func(f string, a ...any) { logf("  "+f, a...) })
	if err != nil {
		return fail("launch nodes: %v", err)
	}
	rep.Nodes = nodeAddrs(nodes)

	rw := NewRewriter(req, nodes)
	realReq := rw.Apply(req)
	rep.Policy = realReq.Policy

	logf("nodes up, pushing policy and waiting for the filter")
	pre := digestFilters(ctx, backend, nodes)
	if err := run.TS().SetACLRaw(ctx, realReq.Policy); err != nil {
		return fail("push policy: %v", err)
	}
	rep.FilterLanded = waitFilterLanded(ctx, backend, nodes, pre)

	res, _, err := EvaluateForEngine(realReq)
	if err != nil {
		return fail("engine: %v", err)
	}
	byID := actorIndex(res)

	pairs := len(nodes) * (len(nodes) - 1)
	logf("filter-landed=%v — probing %d port(s) × %d pair(s)", rep.FilterLanded, len(realReq.Probes), pairs)

	for _, pr := range realReq.Probes {
		for _, src := range nodes {
			for _, dst := range nodes {
				if src.Name == dst.Name {
					continue
				}
				if focus != nil && (src.Name != focus.From || dst.Name != focus.To) {
					continue
				}
				if ctx.Err() != nil {
					return rep
				}
				engineV := verdict(res, pr.Name, byID[src.Name], byID[dst.Name])
				if engineV != eval.VerdictAllow && engineV != eval.VerdictDeny {
					rep.Errors = append(rep.Errors, fmt.Sprintf("%s->%s %s: engine verdict %q", src.Name, dst.Name, pr.Name, engineV))
					continue
				}
				dstAddr, ok := rw.RealAddr(dst.Name)
				if !ok {
					rep.Errors = append(rep.Errors, "no address for "+dst.Name)
					continue
				}
				pctx, cancel := context.WithTimeout(ctx, fuzzProbeBudget)
				reached, perr := backend.Probe(pctx, src, dstAddr, pr.Proto, pr.Port)
				cancel()
				if perr != nil && !errors.Is(perr, context.DeadlineExceeded) {
					rep.Errors = append(rep.Errors, fmt.Sprintf("%s->%s %s: %v", src.Name, dst.Name, pr.Name, perr))
					continue
				}
				rep.Probed++
				tailnetV := verdictOf(reached)
				if engineV != tailnetV {
					rep.Divergences = append(rep.Divergences, FuzzDivergence{
						From: src.Name, To: dst.Name, Proto: pr.Proto, Port: pr.Port,
						Engine: engineV, Tailnet: tailnetV,
					})
				}
			}
		}
	}
	return rep
}

// capProbes keeps at most n probes: the ports rules reference first (most
// likely to be `allow` — the interesting direction), then the rest.
func capProbes(probes []eval.Probe, n int) []eval.Probe {
	if len(probes) <= n {
		return probes
	}
	return append([]eval.Probe(nil), probes[:n]...)
}

func probePortInts(probes []eval.Probe) []int {
	seen := map[int]bool{}
	var out []int
	for _, p := range probes {
		if p.Port != 0 && !seen[p.Port] {
			seen[p.Port] = true
			out = append(out, p.Port)
		}
	}
	return out
}

// FreezeFound writes a diverging fuzz case under <dir>/_found/seed-<n>/ as the
// standard corpus files plus a divergence.txt — the seed for shrink.go.
func FreezeFound(dir string, g GenCase, rep FuzzReport) (string, error) {
	out := filepath.Join(dir, "_found", fmt.Sprintf("seed-%d", g.Seed))
	if err := os.MkdirAll(out, 0o755); err != nil {
		return "", err
	}
	caseJSON, assertionsJSON, metaJSON := g.Files(fmt.Sprintf("fuzz divergence, seed %d", g.Seed))
	var b strings.Builder
	fmt.Fprintf(&b, "seed %d — %s\nnodes: %s   filter-landed: %v\n\n", g.Seed, g.Summary(), strings.Join(rep.Nodes, " "), rep.FilterLanded)
	for _, d := range rep.Divergences {
		fmt.Fprintln(&b, d.String())
	}
	fmt.Fprintf(&b, "\npushed policy:\n%s\n", rep.Policy)
	files := map[string][]byte{
		"case.json":       caseJSON,
		"assertions.json": assertionsJSON,
		"meta.json":       metaJSON,
		"divergence.txt":  []byte(b.String()),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(out, name), data, 0o644); err != nil {
			return out, err
		}
	}
	return out, nil
}

// Summary is a one-line description of what the generator produced.
func (g GenCase) Summary() string {
	m, _ := parsePolicy(g.Request.Policy)
	n := func(k string) int {
		if v, ok := m[k].([]any); ok {
			return len(v)
		}
		return 0
	}
	parts := []string{
		fmt.Sprintf("seed=%d", g.Seed),
		fmt.Sprintf("devs=%d", len(g.Request.Snapshot.Devices)),
		fmt.Sprintf("tags=%d", len(g.Tags)),
		fmt.Sprintf("acls=%d grants=%d ssh=%d", n("acls"), n("grants"), n("ssh")),
	}
	if g.Posture {
		parts = append(parts, "posture")
	}
	return strings.Join(parts, " ")
}
