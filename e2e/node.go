//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// errL4Unsupported is what a backend returns from Probe when it can't run an L4
// differential. RunCase turns it into a skipped row rather than an error. The
// docker-userspace backend has no kernel TUN, so e2eprobe's outbound Dial to a
// 100.x address has no route — only `tailscale ssh`, which tunnels through the
// local API, works without a TUN device.
var errL4Unsupported = errors.New("l4 probing is not supported by this backend")

type NodeBackend interface {
	// Launch brings up one node per spec, joined to the tailnet with the given
	// auth key, each running `e2eprobe serve` on servePorts. It returns once
	// every node is Running and sees every peer. logf (nil ⇒ silent) receives a
	// progress line per node and while waiting for convergence.
	Launch(ctx context.Context, specs []NodeSpec, servePorts []int, logf func(string, ...any)) ([]*Node, error)
	// Probe attempts one L4 echo round-trip from src to dst:port and reports
	// whether it completed.
	Probe(ctx context.Context, src *Node, dst netip.Addr, proto string, port int) (bool, error)
	// Netmap returns src's current netmap JSON (for the sentinel-version poll).
	Netmap(ctx context.Context, src *Node) ([]byte, error)

	SSHProbe(ctx context.Context, src *Node, user string, dst netip.Addr) (SSHVerdict, string, error)
	// Stop tears down every launched node. Ephemeral keys mean the tailnet
	// converges even if this is skipped.
	Stop(ctx context.Context) error
}

// NodeSpec describes one node to launch — it comes straight from a
// snapshot.devices[] entry.
type NodeSpec struct {
	Name    string
	Tags    []string
	AuthKey string
	// SSH enables the Tailscale SSH server on this node (a case with an SSH
	// assertion targeting it).
	SSH bool
	// SSHUsers are non-root local users to create on this node so SSH policies
	// naming them (autogroup:nonroot, localpart) have a real target.
	SSHUsers []string
	// SSHClient installs the system `ssh` binary that `tailscale ssh` shells
	// out to — needed on any node that is the *source* of an SSH assertion.
	SSHClient bool

	AdvertiseRoutes []string
	// AcceptRoutes runs the node with `--accept-routes` so it installs the
	// subnet routes other nodes advertise (Linux defaults to off); without it a
	// probe into a subnet has no route and always reads as a deny.
	AcceptRoutes bool
	// RouteListen are concrete addresses inside the advertised subnet prefixes
	// that the node binds locally so a probe into the subnet has something to
	// answer it. Exit routes get no listener (see the exit-node case's skip).
	RouteListen []netip.Addr
}

// Node is a launched node with its harvested real Tailscale addresses and the
// posture attributes Tailscale populates automatically.
type Node struct {
	Name        string
	Tags        []string
	IPv4        netip.Addr
	IPv6        netip.Addr
	OS          string // Self.OS, e.g. "linux"
	TSVersion   string // dotted client version, e.g. "1.102.3"
	containerID string
}

type dockerBackend struct {
	image      string
	runID      string
	userspace  bool
	probeBin   string // host path to the static e2eprobe binary, bind-mounted in (kernel-TUN mode only)
	containers []string
}

func newBackend(ctx context.Context, cfg Config, runID string) (NodeBackend, error) {
	switch cfg.Backend {
	case "", "docker":
		return NewDockerBackend(ctx, cfg, runID, false)
	case "docker-userspace":
		return NewDockerBackend(ctx, cfg, runID, true)
	case "tsnet":
		return NewTsnetBackend(ctx, cfg, runID)
	default:
		return nil, fmt.Errorf("unknown E2E_BACKEND %q (want docker|docker-userspace|tsnet)", cfg.Backend)
	}
}

// NewDockerBackend builds the docker backend. In kernel-TUN mode it also
// compiles the e2eprobe helper into a temp dir for bind-mounting; userspace mode
// has no L4 probe and skips that.
func NewDockerBackend(ctx context.Context, cfg Config, runID string, userspace bool) (*dockerBackend, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, fmt.Errorf("docker not found on PATH: %w", err)
	}
	d := &dockerBackend{
		image:     cfg.NodeImage,
		runID:     runID,
		userspace: userspace,
	}
	if !userspace {
		bin, err := buildProbeBinary(ctx)
		if err != nil {
			return nil, err
		}
		d.probeBin = bin
	}
	return d, nil
}

func (d *dockerBackend) Launch(ctx context.Context, specs []NodeSpec, servePorts []int, logf func(string, ...any)) ([]*Node, error) {
	logf = orNoopLogf(logf)
	if len(servePorts) == 0 {
		servePorts = []int{ProbePort}
	}
	nodes := make([]*Node, 0, len(specs))
	for _, s := range specs {
		n, err := d.launchOne(ctx, s, servePorts)
		if err != nil {
			return nodes, err
		}
		nodes = append(nodes, n)
		logf("container up: %s", s.Name)
	}

	for _, n := range nodes {
		if err := d.waitRunning(ctx, n); err != nil {
			return nodes, err
		}
		logf("%s running: %s (%s %s)", n.Name, n.IPv4, n.OS, n.TSVersion)
	}

	logf("waiting for %d node(s) to see each other…", len(nodes))
	if err := d.converge(ctx, nodes, logf); err != nil {
		return nodes, err
	}
	logf("converged")
	return nodes, nil
}

// waitRunning blocks until n's tailscaled is Running and harvests its real
// Tailscale addresses into the Node.
func (d *dockerBackend) waitRunning(ctx context.Context, n *Node) error {
	deadline := time.Now().Add(NodeConvergeTimeout)
	for {
		st, err := d.statusByID(ctx, n.containerID)
		if err == nil && st.BackendState == "Running" {
			for _, ipStr := range st.Self.TailscaleIPs {
				ip, perr := netip.ParseAddr(ipStr)
				if perr != nil {
					continue
				}
				if ip.Is4() {
					n.IPv4 = ip
				} else {
					n.IPv6 = ip
				}
			}
			if n.IPv4.IsValid() {
				n.OS = st.Self.OS
				n.TSVersion = shortVersion(st.Version)
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("node %s never reached Running with an address in %s", n.Name, NodeConvergeTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

func (d *dockerBackend) launchOne(ctx context.Context, s NodeSpec, servePorts []int) (*Node, error) {
	name := fmt.Sprintf("%s%s-%s", TailnetPrefix, d.runID, sanitize(s.Name))
	extra := "--hostname=" + sanitize(s.Name) + tagArg(s.Tags)
	if s.SSH {
		extra += " --ssh"
	}
	if len(s.AdvertiseRoutes) > 0 {
		extra += " --advertise-routes=" + strings.Join(s.AdvertiseRoutes, ",")
	}
	if s.AcceptRoutes {
		extra += " --accept-routes"
	}
	args := []string{
		"run", "-d", "--name", name, "--hostname", sanitize(s.Name),
		"-e", "TS_AUTHKEY=" + s.AuthKey,
		"-e", "TS_STATE_DIR=/var/lib/tailscale",
		"-e", "TS_EXTRA_ARGS=" + extra,
	}
	if d.userspace {
		args = append(args, "-e", "TS_USERSPACE=true")
	} else {
		args = append(args,
			"--device=/dev/net/tun", "--cap-add=NET_ADMIN",
			"-e", "TS_USERSPACE=false",
			"-v", d.probeBin+":/e2eprobe:ro")
		if len(s.AdvertiseRoutes) > 0 {
			args = append(args,
				"--sysctl", "net.ipv4.ip_forward=1",
				"--sysctl", "net.ipv6.conf.all.forwarding=1")
		}
	}
	args = append(args, d.image)
	out, err := runCmd(ctx, "docker", args...)
	if err != nil {
		return nil, fmt.Errorf("docker run %s: %w: %s", name, err, out)
	}
	cid := strings.TrimSpace(out)
	d.containers = append(d.containers, cid)
	n := &Node{Name: s.Name, Tags: s.Tags, containerID: cid}

	// Start the echo server on every port any assertion in this case probes.
	// `docker exec -d` returns immediately. Userspace mode has no L4 probe, so
	// there is nothing to serve.
	if !d.userspace {
		ports := make([]string, len(servePorts))
		for i, p := range servePorts {
			ports[i] = fmt.Sprint(p)
		}
		if _, err := runCmd(ctx, "docker", "exec", "-d", cid,
			"/e2eprobe", "serve", "--ports", strings.Join(ports, ","), "--proto", "both"); err != nil {
			return n, fmt.Errorf("start e2eprobe on %s: %w", s.Name, err)
		}

		if len(s.AdvertiseRoutes) > 0 {
			for _, a := range s.RouteListen {
				bits := "32"
				if a.Is6() {
					bits = "128"
				}
				if out, err := runCmd(ctx, "docker", "exec", cid,
					"ip", "addr", "add", a.String()+"/"+bits, "dev", "lo"); err != nil {
					return n, fmt.Errorf("bind route listener %s on %s: %w: %s", a, s.Name, err, out)
				}
			}
		}
	}

	// SSH cases: give the Tailscale SSH server real non-root login targets, and
	// put an ssh client on the node so it can also be an SSH *source*.
	if s.SSH || s.SSHClient || len(s.SSHUsers) > 0 {
		if out, err := runCmd(ctx, "docker", "exec", cid,
			"apk", "add", "--no-cache", "openssh-client"); err != nil {
			return n, fmt.Errorf("install openssh-client on %s: %w: %s", s.Name, err, out)
		}
		for _, u := range s.SSHUsers {
			if u == "" || u == "root" {
				continue
			}
			if out, err := runCmd(ctx, "docker", "exec", cid,
				"adduser", "-D", "-s", "/bin/sh", u); err != nil {
				return n, fmt.Errorf("adduser %s on %s: %w: %s", u, s.Name, err, out)
			}
		}
	}
	return n, nil
}

func (d *dockerBackend) converge(ctx context.Context, nodes []*Node, logf func(string, ...any)) error {
	return awaitConverged(ctx, nodes, logf, "nodes", dockerConvergePoll,
		func(ctx context.Context, n *Node) (map[netip.Addr]bool, string) {
			st, err := d.statusByID(ctx, n.containerID)
			if err != nil {
				return nil, "status err"
			}
			online := map[netip.Addr]bool{}
			for _, p := range st.Peer {
				if !p.Online {
					continue
				}
				for _, ipStr := range p.TailscaleIPs {
					if ip, perr := netip.ParseAddr(ipStr); perr == nil {
						online[ip] = true
					}
				}
			}
			return online, ""
		})
}

// awaitConverged polls until every node sees every *other* node's address as
// an online peer. onlinePeers reports the addresses n currently sees online, or
// a short reason it can't tell. Peers that are not one of our nodes — e.g.
// stale ephemeral devices left on a reused tailnet — are ignored. what names
// the nodes in the timeout error.
func awaitConverged(
	ctx context.Context,
	nodes []*Node,
	logf func(string, ...any),
	what string,
	poll time.Duration,
	onlinePeers func(context.Context, *Node) (map[netip.Addr]bool, string),
) error {
	logf = orNoopLogf(logf)
	want := map[netip.Addr]string{}
	for _, n := range nodes {
		want[n.IPv4] = n.Name
	}
	deadline := time.Now().Add(NodeConvergeTimeout)
	lastReport := time.Now()
	for {
		var missing []string
		for _, n := range nodes {
			online, why := onlinePeers(ctx, n)
			if why != "" {
				missing = append(missing, n.Name+"("+why+")")
				continue
			}
			for peerIP, peerName := range want {
				if peerName == n.Name {
					continue
				}
				if !online[peerIP] {
					missing = append(missing, n.Name+"→"+peerName)
				}
			}
		}
		if len(missing) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s did not converge within %s (missing: %s)",
				what, NodeConvergeTimeout, strings.Join(missing, ", "))
		}
		if time.Since(lastReport) >= convergeReportEvery {
			logf("still converging (%d link(s) pending: %s)", len(missing), strings.Join(missing, ", "))
			lastReport = time.Now()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(poll):
		}
	}
}

func (d *dockerBackend) Probe(ctx context.Context, src *Node, dst netip.Addr, proto string, port int) (bool, error) {
	if d.userspace {
		return false, errL4Unsupported
	}
	target := netip.AddrPortFrom(dst, uint16(port)).String()
	var lastErr error
	backoff := 500 * time.Millisecond
	for i := 0; i < ProbeRetries; i++ {
		out, err := runCmd(ctx, "docker", "exec", src.containerID,
			"/e2eprobe", "connect", target, "--proto", proto, "--timeout", ProbeTimeout.String())
		if err == nil && strings.Contains(out, "ok") {
			return true, nil
		}
		lastErr = fmt.Errorf("%w: %s", err, strings.TrimSpace(out))
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < 4*time.Second {
			backoff *= 2
		}
	}
	// Every attempt failed. For a probe this is a legitimate "deny" verdict —
	// the caller decides whether that agrees with the engine.
	_ = lastErr
	return false, nil
}

func (d *dockerBackend) Netmap(ctx context.Context, src *Node) ([]byte, error) {
	out, err := runCmd(ctx, "docker", "exec", src.containerID, "tailscale", "debug", "netmap")
	if err != nil {
		return nil, fmt.Errorf("netmap %s: %w", src.Name, err)
	}
	return []byte(out), nil
}

func (d *dockerBackend) SSHProbe(ctx context.Context, src *Node, user string, dst netip.Addr) (SSHVerdict, string, error) {
	target := user + "@" + dst.String()
	var last string
	backoff := time.Second
	for i := 0; i < ProbeRetries; i++ {
		// tailscale ssh checks the host key against the coordination server
		// itself and auth is Tailscale-native, so no ssh -o flags are needed.
		// No --tty flag: `tailscale ssh` doesn't define one, and passing a
		// command (`true`) already means OpenSSH requests no PTY. `docker exec`
		// here has no TTY on stdin either.
		out, err := runCmd(ctx, "docker", "exec", src.containerID,
			"timeout", "25",
			"tailscale", "ssh", target, "true")
		v, settled := classifySSH(out, err == nil)
		last = strings.TrimSpace(out)
		if settled {
			return v, last, nil
		}
		select {
		case <-ctx.Done():
			return SSHVerdict(""), last, ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < 6*time.Second {
			backoff *= 2
		}
	}
	// Never observed an SSH banner or a completed session — treat as blocked.
	return SSHDeny, last, nil
}

// shortVersion trims a tailscaled version string ("1.102.3-t53a0d659a") to its
// dotted core so engine/posture's numeric comparison accepts it.
func shortVersion(v string) string {
	if i := strings.IndexAny(v, "-+ "); i > 0 {
		return v[:i]
	}
	return v
}

// Stop force-removes every container this backend launched. The caller decides
// whether to call it (the test keeps a failed case's nodes under E2E_KEEP).
func (d *dockerBackend) Stop(ctx context.Context) error {
	var firstErr error
	for _, cid := range d.containers {
		if _, err := runCmd(ctx, "docker", "rm", "-f", cid); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	d.containers = nil
	return firstErr
}

// --- tailscale status plumbing ---

type tsStatus struct {
	BackendState string
	Version      string
	Self         tsPeerStatus
	Peer         map[string]tsPeerStatus
}

type tsPeerStatus struct {
	HostName     string
	OS           string
	TailscaleIPs []string
	Online       bool
}

func (d *dockerBackend) statusByID(ctx context.Context, cid string) (*tsStatus, error) {
	out, err := runCmd(ctx, "docker", "exec", cid, "tailscale", "status", "--json")
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(out))
	}
	var st tsStatus
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		return nil, fmt.Errorf("parse status json: %w", err)
	}
	return &st, nil
}

// --- helpers ---

// buildProbeBinary compiles ./e2e/e2eprobe as a static linux/amd64 binary into
// a temp file and returns its host path.
func buildProbeBinary(ctx context.Context) (string, error) {
	dir, err := os.MkdirTemp("", "tg-e2eprobe-")
	if err != nil {
		return "", err
	}
	bin := filepath.Join(dir, "e2eprobe")
	// `go test` runs with the working directory set to this package's dir.
	// The container shares the Docker host's architecture.
	cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, "./e2eprobe")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+runtime.GOARCH)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build e2eprobe: %w: %s", err, out)
	}
	return bin, nil
}

// orNoopLogf returns logf, or a silent stand-in if it is nil, so backends can
// call it unconditionally.
func orNoopLogf(logf func(string, ...any)) func(string, ...any) {
	if logf == nil {
		return func(string, ...any) {}
	}
	return logf
}

func runCmd(ctx context.Context, name string, args ...string) (string, error) {
	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// validTag is the shape of a Tailscale tag. TS_EXTRA_ARGS is split on spaces
// by the container entrypoint, so anything looser could smuggle in extra flags.
var validTag = regexp.MustCompile(`^tag:[A-Za-z][A-Za-z0-9-]*$`)

// tagArg renders --advertise-tags, dropping tags that aren't well-formed (a
// malformed one would be rejected by the control plane anyway).
func tagArg(tags []string) string {
	var ok []string
	for _, t := range tags {
		if validTag.MatchString(t) {
			ok = append(ok, t)
		}
	}
	if len(ok) == 0 {
		return ""
	}
	return " --advertise-tags=" + strings.Join(ok, ",")
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return b.String()
}
