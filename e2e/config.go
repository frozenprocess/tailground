//go:build e2e

// Idea with this package is to run everything in an actual environment and check
// if it goes boom.
package e2e

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	OAuthClientID     string
	OAuthClientSecret string

	// APIBaseURL is the Tailscale API root. Overridable for testing.
	APIBaseURL string

	Backend string

	// NodeImage is the pinned tailscale/tailscale image the docker backend runs
	// (E2E_NODE_IMAGE). Default: DefaultNodeImage.
	NodeImage string

	// RunDir is the git-ignored directory for per-run child-tailnet creds + id.
	RunDir string

	// Keep freezes a failed case's nodes for live inspection (E2E_KEEP).
	Keep bool

	// EngineOnly skips the tailnet entirely and scores the engine against the
	// authored expectations.
	EngineOnly bool
	// Some stuff are not free that is how we flagem.
	Premium       bool
	Reuse         string
	TailnetMaxAge time.Duration
	Artifacts     string
	CaseDir       string
	Case          string
	Report        string
	Shards        int
}

// Harness-wide invariants. Not knobs, changing these changes what the harness
// is, so they live in code.
const (
	TailnetPrefix    = "tg-e2e-"
	ProbePort        = 22
	DefaultNodeImage = "tailscale/tailscale@sha256:8c42c4574ab066384fcb72f69e086a2ff1dd3652eb6f56856cee34bcf0d2f680"
	SentinelTag      = "tag:tg-e2e-sentinel"
)

// Timing constants.
const (
	// APITimeout bounds a single Tailscale API call.
	APITimeout = 30 * time.Second

	// TailnetCreateTimeout bounds child-tailnet provisioning
	TailnetCreateTimeout = 2 * time.Minute

	// NodeConvergeTimeout bounds waiting for every launched node to reach
	// Running and see all peers.
	NodeConvergeTimeout = 3 * time.Minute

	// convergeReportEvery is how often a still-converging wait logs the links
	// it is waiting on.
	convergeReportEvery = 20 * time.Second

	// dockerConvergePoll / tsnetConvergePoll are how often each backend
	// re-reads its nodes' peer status while converging (a docker status is a
	// `docker exec`, so it polls less often than tsnet's in-process client).
	dockerConvergePoll = 3 * time.Second
	tsnetConvergePoll  = 2 * time.Second

	// FilterSettleTimeout bounds waiting for a pushed ACL to appear in every
	// node's netmap.
	FilterSettleTimeout = 90 * time.Second

	// ProbeTimeout is one e2eprobe connect attempt.
	ProbeTimeout = 2 * time.Second

	// ProbeRetries is how many times a probe is retried (with backoff).
	ProbeRetries = 6
)

// LoadConfig resolves the harness configuration from the environment.
func LoadConfig() Config {
	c := Config{
		OAuthClientID:     os.Getenv("TS_OAUTH_CLIENT_ID"),
		OAuthClientSecret: os.Getenv("TS_OAUTH_CLIENT_SECRET"),
		APIBaseURL:        envOr("E2E_API_BASE_URL", "https://api.tailscale.com"),
		Backend:           envOr("E2E_BACKEND", "docker"),
		NodeImage:         envOr("E2E_NODE_IMAGE", DefaultNodeImage),
		RunDir:            envOr("E2E_RUN_DIR", ".run"),
		Keep:              envBool("E2E_KEEP"),
		EngineOnly:        envBool("E2E_ENGINE_ONLY"),
		Premium:           envBool("E2E_PREMIUM"),
		Reuse:             os.Getenv("E2E_REUSE"),
		Artifacts:         os.Getenv("E2E_ARTIFACTS"),
		CaseDir:           envOr("E2E_CASE_DIR", "cases"),
		Case:              os.Getenv("E2E_CASE"),
		Report:            os.Getenv("E2E_REPORT"),
		TailnetMaxAge:     3 * time.Hour,
		Shards:            envInt("E2E_SHARDS", 1),
	}
	if c.Shards < 1 {
		c.Shards = 1
	}
	if c.Shards > 8 {
		c.Shards = 8 // free-org ceiling
	}
	if v := os.Getenv("E2E_TAILNET_MAX_AGE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.TailnetMaxAge = d
		}
	}
	return c
}

// RequireLive returns an error unless the org OAuth client is configured. Tests
// that need the real tailnet call this and skip when it fails.
func (c Config) RequireLive() error {
	if c.EngineOnly {
		return fmt.Errorf("E2E_ENGINE_ONLY is set")
	}
	if c.OAuthClientID == "" || c.OAuthClientSecret == "" {
		return fmt.Errorf("TS_OAUTH_CLIENT_ID / TS_OAUTH_CLIENT_SECRET not set")
	}
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string) bool {
	v := os.Getenv(key)
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	return err == nil && b
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}
