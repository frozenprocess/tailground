package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/frozenprocess/tailground/engine/adapters/hs"
	"github.com/frozenprocess/tailground/engine/eval"
)

func main() {
	demo := flag.Bool("demo", false, "ignore stdin, evaluate the built-in demo scenario")
	flag.Bool("eval", false, "read an eval.Request from stdin, write an eval.Result to stdout (the default mode; the flag is explicit for callers)")
	flag.Parse()

	var req eval.Request
	if *demo {
		req = demoRequest()
	} else {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fatal(err)
		}
		if err := json.Unmarshal(b, &req); err != nil {
			fatal(fmt.Errorf("parsing request: %w", err))
		}
	}

	res := hs.New().Evaluate(req)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(res); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "tg-eval:", err)
	os.Exit(1)
}

// demoRequest is a 3-device tailnet (one user
// device, two tagged) with a policy that lets the user's device reach tag:web
// on 443 and nothing else.
func demoRequest() eval.Request {
	const policy = `
{
  "postures": {
    "posture:corp": ["node:os == 'macos'", "node:tsVersion >= '1.80'"]
  },
  "tagOwners": {
    "tag:web": ["alice@example.com"],
    "tag:db":  ["alice@example.com"]
  },
  "acls": [
    { "action": "accept", "src": ["alice@example.com"], "srcPosture": ["posture:corp"], "dst": ["tag:web:443"] },
    { "action": "accept", "src": ["tag:web"],           "dst": ["tag:db:5432"] }
  ],
  "grants": [
    { "src": ["alice@example.com"], "dst": ["tag:web"],
      "app": { "example.com/cap/admin": [{ "role": "admin" }] } }
  ],
  "ssh": [
    { "action": "check", "src": ["alice@example.com"], "dst": ["tag:web"], "users": ["autogroup:nonroot"] }
  ],
  "nodeAttrs": [
    { "target": ["tag:web"], "attr": ["mullvad"] }
  ],
  "tests": [
    { "src": "alice@example.com", "accept": ["tag:web:443"], "deny": ["tag:db:5432"] }
  ]
}`

	return eval.Request{
		Policy: policy,
		Snapshot: eval.Snapshot{
			Users: []eval.User{{ID: 1, Email: "alice@example.com"}},
			Devices: []eval.Device{
				{ID: 1, Name: "alice-laptop", User: "alice@example.com", IPv4: "100.64.0.1", IPv6: "fd7a:115c:a1e0::1", OS: "macos", Posture: map[string]string{"tsVersion": "1.82.0"}},
				{ID: 2, Name: "web-1", Tags: []string{"tag:web"}, IPv4: "100.64.0.2", IPv6: "fd7a:115c:a1e0::2", OS: "linux"},
				{ID: 3, Name: "db-1", Tags: []string{"tag:db"}, IPv4: "100.64.0.3", IPv6: "fd7a:115c:a1e0::3", OS: "linux"},
			},
		},
		Probes: []eval.Probe{
			{Name: "https", Proto: "tcp", Port: 443},
			{Name: "pg", Proto: "tcp", Port: 5432},
			{Name: "ssh", Proto: "tcp", Port: 22},
		},
		SSHUsers: []string{"root", "ubuntu"},
	}
}
