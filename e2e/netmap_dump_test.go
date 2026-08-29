//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tsapi "tailscale.com/client/tailscale/v2"
)

func TestNetmapDump(t *testing.T) {
	cfg := LoadConfig()
	if err := cfg.RequireLive(); err != nil {
		t.Skipf("skipping: %v", err)
	}
	if cfg.Backend != "docker" && cfg.Backend != "" {
		t.Skipf("needs the kernel-TUN docker backend for a full netmap (got %q)", cfg.Backend)
	}

	outDir := cfg.Artifacts
	if outDir == "" {
		outDir = t.TempDir()
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", outDir, err)
	}
	t.Logf("artifacts → %s", outDir)
	write := func(name string, b []byte) {
		p := filepath.Join(outDir, name)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Errorf("write %s: %v", p, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 28*time.Minute)
	defer cancel()

	org := NewOrgClient(cfg)
	runID := NewRunID()
	if rep, err := Reap(ctx, cfg, org, cfg.TailnetMaxAge); err == nil && len(rep.Deleted) > 0 {
		t.Logf("reaper: %s", rep.String())
	}

	run, err := Provision(ctx, cfg, org, runID)
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	t.Logf("child tailnet %s (id=%s dns=%s)", run.Child.DisplayName, run.ID, run.Child.DNSName)
	keep := false
	defer func() {
		if keep || t.Failed() {
			run.Keep()
			t.Logf("KEEPING tailnet %s — creds in %s; `make e2e-down` to reap", run.ID, cfg.RunDir)
		}
		if err := run.Teardown(context.Background()); err != nil {
			t.Errorf("teardown: %v", err)
		}
	}()

	ts := run.TS()

	const tagOwners = `"tagOwners": {
		"tag:client":   ["autogroup:admin"],
		"tag:api":      ["autogroup:admin"],
		"tag:web":      ["autogroup:admin"],
		"tag:gw":       ["autogroup:admin"],
		"tag:outsider": ["autogroup:admin"]
	}`

	// 1. Provisioning policy — declare tags, allow all, so keys mint and nodes converge.
	provPol, err := ProvisioningPolicy("{" + tagOwners + `, "acls":[{"action":"accept","src":["*"],"dst":["*:*"]}]}`)
	if err != nil {
		t.Fatalf("provisioning policy: %v", err)
	}
	if err := ts.SetACLRaw(ctx, provPol); err != nil {
		t.Fatalf("push provisioning policy: %v", err)
	}
	if _, err := ts.PurgeDevices(ctx); err != nil {
		t.Fatalf("purge devices: %v", err)
	}

	// 2. Keys — note the multi-tag key.
	mint := func(tags ...string) string {
		k, err := ts.MintAuthKey(ctx, tags)
		if err != nil {
			t.Fatalf("mint key %v: %v", tags, err)
		}
		return k
	}
	kClient := mint("tag:client")
	kMulti := mint("tag:api", "tag:web")
	kGw := mint("tag:gw")
	kOut := mint("tag:outsider")

	// 3. Launch. `multi` carries two tags; `gw` advertises a subnet.
	backend, err := newBackend(ctx, cfg, runID+"-nm")
	if err != nil {
		t.Fatalf("backend: %v", err)
	}
	defer func() {
		if !keep && !t.Failed() {
			_ = backend.Stop(context.Background())
		}
	}()

	// gw advertises the subnet — but NOT via NodeSpec.AdvertiseRoutes, whose
	// docker `sysctl -w` step fails on a read-only /proc/sys here. We set it by
	// hand after launch; netmap inspection doesn't need kernel forwarding.
	specs := []NodeSpec{
		{Name: "client", Tags: []string{"tag:client"}, AuthKey: kClient},
		{Name: "multi", Tags: []string{"tag:api", "tag:web"}, AuthKey: kMulti},
		{Name: "gw", Tags: []string{"tag:gw"}, AuthKey: kGw},
		{Name: "outsider", Tags: []string{"tag:outsider"}, AuthKey: kOut},
	}
	nodes, err := backend.Launch(ctx, specs, []int{443, 8080}, func(f string, a ...any) { t.Logf("  "+f, a...) })
	if err != nil {
		keep = cfg.Keep
		t.Fatalf("launch: %v", err)
	}
	nodeByName := map[string]*Node{}
	for _, n := range nodes {
		nodeByName[n.Name] = n
		t.Logf("node %-9s ipv4=%s ipv6=%s tags=%v", n.Name, n.IPv4, n.IPv6, n.Tags)
	}

	// 3b. gw advertises 10.99.0.0/24, then the admin approves it.
	if gw := nodeByName["gw"]; gw != nil {
		out, err := runCmd(ctx, "docker", "exec", gw.containerID,
			"tailscale", "set", "--advertise-routes=10.99.0.0/24")
		t.Logf("gw advertise-routes: err=%v out=%s", err, strings.TrimSpace(out))
	}
	if err := ts.ApproveRoutes(ctx, map[string][]string{"gw": {"10.99.0.0/24"}}); err != nil {
		t.Errorf("approve routes: %v", err)
	}

	// 4. Tailscale VIP service `svc:payments`, backed by tag:api (i.e. `multi`).
	vipErr := ts.api.VIPServices().CreateOrUpdate(ctx, tsapi.VIPService{
		Name:    "svc:payments",
		Ports:   []string{"tcp:443"},
		Tags:    []string{"tag:api"},
		Comment: "tg-e2e netmap dump",
	})
	if vipErr != nil {
		t.Logf("VIP service CreateOrUpdate: %v (plan-gated? continuing)", vipErr)
	} else {
		t.Logf("VIP service svc:payments created")
	}
	if list, err := ts.api.VIPServices().List(ctx); err != nil {
		t.Logf("VIP service List: %v", err)
	} else {
		b, _ := json.MarshalIndent(list, "", "  ")
		write("vip-services.json", b)
		t.Logf("VIP services:\n%s", b)
	}

	// Ask `multi` to advertise the service so it can propagate into peer netmaps.
	if vipErr == nil {
		if m := nodeByName["multi"]; m != nil {
			out, err := runCmd(ctx, "docker", "exec", m.containerID,
				"tailscale", "set", "--advertise-services=svc:payments")
			t.Logf("multi advertise-services: err=%v out=%s", err, strings.TrimSpace(out))
			out, err = runCmd(ctx, "docker", "exec", m.containerID,
				"tailscale", "serve", "--service=svc:payments", "--bg", "--tcp=443", "tcp://127.0.0.1:443")
			t.Logf("multi serve --service: err=%v out=%s", err, strings.TrimSpace(out))
		}
	}

	// 5. The policy under test: svc dst on :443 only + a via grant through tag:gw.
	pol := "{" + tagOwners + `,
		"acls": [
			{ "action": "accept", "src": ["tag:client"], "dst": ["svc:payments:443"] }
		],
		"grants": [
			{ "src": ["tag:client"], "dst": ["10.99.0.0/24"], "ip": ["tcp:443"], "via": ["tag:gw"] }
		]
	}`
	if err := ts.ValidateACL(ctx, pol); err != nil {
		t.Errorf("ValidateACL (test policy): %v", err)
	}
	if err := ts.SetACLRaw(ctx, pol); err != nil {
		t.Fatalf("push test policy: %v", err)
	}
	write("policy.hujson", []byte(pol))

	// Let it propagate. No probe here — this test only inspects netmaps.
	t.Log("waiting 45s for the policy + service + route to propagate…")
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-time.After(45 * time.Second):
	}

	// 6. Dump everything.
	devs, err := ts.Devices(ctx)
	if err != nil {
		t.Errorf("devices: %v", err)
	} else {
		b, _ := json.MarshalIndent(devs, "", "  ")
		write("devices.json", b)
		for _, d := range devs {
			t.Logf("API device %-10s tags=%v addrs=%v routes(adv=%v en=%v)",
				d.Hostname, d.Tags, d.Addresses, d.AdvertisedRoutes, d.EnabledRoutes)
		}
	}

	for _, n := range nodes {
		raw, err := backend.Netmap(ctx, n)
		if err != nil {
			t.Errorf("netmap %s: %v", n.Name, err)
			continue
		}
		write(n.Name+".netmap.json", raw)

		for _, extra := range []struct {
			file string
			args []string
		}{
			{n.Name + ".status.json", []string{"status", "--json", "--peers"}},
			{n.Name + ".filter.json", []string{"debug", "packet-filter"}},
			{n.Name + ".whois.txt", []string{"whois", n.IPv4.String()}},
		} {
			cmd := append([]string{"exec", n.containerID, "tailscale"}, extra.args...)
			out, err := runCmd(ctx, "docker", cmd...)
			if err != nil {
				t.Logf("%s: %v", extra.file, err)
			}
			write(extra.file, []byte(out))
		}

		analyzeNetmap(t, n.Name, raw)
	}

	t.Logf("DONE — full dumps in %s", outDir)
}

// analyzeNetmap pulls the three signals out of one node's
// `tailscale debug netmap` JSON and logs them.
func analyzeNetmap(t *testing.T, node string, raw []byte) {
	var nm struct {
		SelfNode struct {
			Name          string         `json:"Name"`
			Addresses     []string       `json:"Addresses"`
			AllowedIPs    []string       `json:"AllowedIPs"`
			Tags          []string       `json:"Tags"`
			PrimaryRoutes []string       `json:"PrimaryRoutes"`
			CapMap        map[string]any `json:"CapMap"`
		} `json:"SelfNode"`
		Peers []struct {
			Name          string          `json:"Name"`
			Addresses     []string        `json:"Addresses"`
			AllowedIPs    []string        `json:"AllowedIPs"`
			PrimaryRoutes []string        `json:"PrimaryRoutes"`
			Tags          []string        `json:"Tags"`
			CapMap        map[string]any  `json:"CapMap"`
			Hostinfo      json.RawMessage `json:"Hostinfo"`
		} `json:"Peers"`
		PacketFilter  json.RawMessage `json:"PacketFilter"`
		PacketFilters json.RawMessage `json:"PacketFilters"`
	}
	if err := json.Unmarshal(raw, &nm); err != nil {
		t.Logf("[%s] netmap parse: %v", node, err)
		return
	}
	t.Logf("── [%s] netmap ──────────────────────────────", node)
	t.Logf("[%s] self Name=%s Tags=%v Addrs=%v AllowedIPs=%v PrimaryRoutes=%v",
		node, nm.SelfNode.Name, nm.SelfNode.Tags, nm.SelfNode.Addresses,
		nm.SelfNode.AllowedIPs, nm.SelfNode.PrimaryRoutes)
	for k := range nm.SelfNode.CapMap {
		t.Logf("[%s] self CapMap key: %s", node, k)
	}
	for _, p := range nm.Peers {
		vip := ""
		for _, a := range p.AllowedIPs {
			if strings.HasPrefix(a, "100.") && !containsStr(p.Addresses, a) && !strings.HasSuffix(a, "/32") {
				vip += " " + a
			}
			if strings.HasPrefix(a, "10.99.") {
				vip += " ROUTE:" + a
			}
		}
		caps := make([]string, 0, len(p.CapMap))
		for k := range p.CapMap {
			caps = append(caps, k)
		}
		t.Logf("[%s] peer %-10s Tags=%v Addrs=%v AllowedIPs=%v PrimaryRoutes=%v extraAllowed=%q caps=%v",
			node, p.Name, p.Tags, p.Addresses, p.AllowedIPs, p.PrimaryRoutes, strings.TrimSpace(vip), caps)
	}
	// Any mention of the VIP prefix (100.96/11 is the Tailscale services range) or "svc:".
	s := string(raw)
	for _, needle := range []string{"svc:payments", "100.96.", "VIPService", "vipService", "services", "Via", "\"via\""} {
		if i := strings.Index(s, needle); i >= 0 {
			lo := i - 60
			if lo < 0 {
				lo = 0
			}
			hi := i + 120
			if hi > len(s) {
				hi = len(s)
			}
			t.Logf("[%s] netmap mentions %q: …%s…", node, needle, strings.ReplaceAll(s[lo:hi], "\n", " "))
		}
	}
}

func containsStr(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
