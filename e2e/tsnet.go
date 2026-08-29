//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"tailscale.com/tsnet"
)

// errSSHUnsupported is what a backend returns from SSHProbe when it can't run a
// `tailscale ssh` differential. RunCase turns it into a skipped row rather than
// an error. The tsnet backend is userspace-only and has no SSH client.
var errSSHUnsupported = errors.New("ssh probing is not supported by this backend")

// tsnetBackend runs each node as an in-process userspace tsnet.
type tsnetBackend struct {
	cfg   Config
	runID string
	dir   string // temp base dir; one subdir of state per node

	mu    sync.Mutex
	nodes []*tsnetNode
}

type tsnetNode struct {
	name string
	srv  *tsnet.Server
	lns  []net.Listener
	pcs  []net.PacketConn
}

// NewTsnetBackend prepares a tsnet backend. Nodes are created in Launch.
func NewTsnetBackend(_ context.Context, cfg Config, runID string) (*tsnetBackend, error) {
	dir, err := os.MkdirTemp("", "tg-tsnet-"+sanitize(runID)+"-")
	if err != nil {
		return nil, err
	}
	return &tsnetBackend{cfg: cfg, runID: runID, dir: dir}, nil
}

func (t *tsnetBackend) Launch(ctx context.Context, specs []NodeSpec, servePorts []int, logf func(string, ...any)) ([]*Node, error) {
	logf = orNoopLogf(logf)
	if len(servePorts) == 0 {
		servePorts = []int{ProbePort}
	}
	// Bring the nodes up concurrently — each srv.Up() is a ~30s round-trip to
	// the real coordination server, and they're independent.
	nodes := make([]*Node, len(specs))
	errs := make([]error, len(specs))
	var wg sync.WaitGroup
	for i, s := range specs {
		wg.Add(1)
		go func(i int, s NodeSpec) {
			defer wg.Done()
			nodes[i], errs[i] = t.launchOne(ctx, s, servePorts)
			if errs[i] == nil {
				logf("%s up: %s", nodes[i].Name, nodes[i].IPv4)
			}
		}(i, s)
	}
	wg.Wait()

	for i := range specs {
		if errs[i] != nil {
			return nil, fmt.Errorf("node %s: %w", specs[i].Name, errs[i])
		}
	}
	logf("waiting for %d node(s) to see each other…", len(nodes))
	if err := t.converge(ctx, nodes, logf); err != nil {
		return nodes, err
	}
	logf("converged")
	return nodes, nil
}

func (t *tsnetBackend) launchOne(ctx context.Context, s NodeSpec, servePorts []int) (*Node, error) {
	srv := &tsnet.Server{
		Dir:       filepath.Join(t.dir, sanitize(s.Name)),
		Hostname:  sanitize(s.Name),
		AuthKey:   s.AuthKey,
		Ephemeral: true,
		Logf:      func(string, ...any) {},
		UserLogf:  func(string, ...any) {},
	}
	if len(s.Tags) > 0 {
		srv.AdvertiseTags = append([]string(nil), s.Tags...)
	}

	upCtx, cancel := context.WithTimeout(ctx, NodeConvergeTimeout)
	defer cancel()
	st, err := srv.Up(upCtx)
	if err != nil {
		_ = srv.Close()
		return nil, fmt.Errorf("tsnet node %s did not come up: %w", s.Name, err)
	}

	tn := &tsnetNode{name: s.Name, srv: srv}
	t.mu.Lock()
	t.nodes = append(t.nodes, tn)
	t.mu.Unlock()

	n := &Node{Name: s.Name, Tags: s.Tags}
	for _, ip := range st.TailscaleIPs {
		if ip.Is4() {
			n.IPv4 = ip
		} else {
			n.IPv6 = ip
		}
	}
	if !n.IPv4.IsValid() {
		return n, fmt.Errorf("tsnet node %s came up without an IPv4 address", s.Name)
	}
	n.OS = runtime.GOOS
	if lc, err := srv.LocalClient(); err == nil {
		if full, err := lc.Status(ctx); err == nil {
			if full.Self != nil && full.Self.OS != "" {
				n.OS = full.Self.OS
			}
			n.TSVersion = shortVersion(full.Version)
		}
	}

	// One TCP + one UDP echo listener per port any assertion in this case
	// probes. tsnet's ListenPacket needs an explicit bind IP (Listen doesn't);
	// bind UDP to the node's v4 — the corpus probes v4 destinations.
	for _, p := range servePorts {
		ln, err := srv.Listen("tcp", fmt.Sprintf(":%d", p))
		if err != nil {
			return n, fmt.Errorf("tsnet listen tcp :%d on %s: %w", p, s.Name, err)
		}
		tn.lns = append(tn.lns, ln)
		go echoTCP(ln)

		pc, err := srv.ListenPacket("udp", net.JoinHostPort(n.IPv4.String(), fmt.Sprint(p)))
		if err != nil {
			return n, fmt.Errorf("tsnet listen udp %s:%d on %s: %w", n.IPv4, p, s.Name, err)
		}
		tn.pcs = append(tn.pcs, pc)
		go echoUDP(pc)
	}
	return n, nil
}

// converge waits until every node sees every other node as an online peer.
func (t *tsnetBackend) converge(ctx context.Context, nodes []*Node, logf func(string, ...any)) error {
	return awaitConverged(ctx, nodes, logf, "tsnet nodes", tsnetConvergePoll,
		func(ctx context.Context, n *Node) (map[netip.Addr]bool, string) {
			tn := t.byName(n.Name)
			if tn == nil {
				return nil, "gone"
			}
			lc, err := tn.srv.LocalClient()
			if err != nil {
				return nil, "lc err"
			}
			st, err := lc.Status(ctx)
			if err != nil {
				return nil, "status err"
			}
			online := map[netip.Addr]bool{}
			for _, p := range st.Peer {
				if !p.Online {
					continue
				}
				for _, ip := range p.TailscaleIPs {
					online[ip] = true
				}
			}
			return online, ""
		})
}

func (t *tsnetBackend) Probe(ctx context.Context, src *Node, dst netip.Addr, proto string, port int) (bool, error) {
	tn := t.byName(src.Name)
	if tn == nil {
		return false, fmt.Errorf("no tsnet node %q", src.Name)
	}
	target := netip.AddrPortFrom(dst, uint16(port)).String()
	backoff := 500 * time.Millisecond
	for i := 0; i < ProbeRetries; i++ {
		if t.probeOnce(ctx, tn.srv, proto, target) {
			return true, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < 4*time.Second {
			backoff *= 2
		}
	}
	// Every attempt failed — a legitimate "deny" for the caller to compare.
	return false, nil
}

// probeOnce dials src→target and checks one echo round-trip. A filter denial
// shows up as a dial timeout (TCP) or a read timeout (UDP).
func (t *tsnetBackend) probeOnce(ctx context.Context, srv *tsnet.Server, proto, target string) bool {
	dialCtx, cancel := context.WithTimeout(ctx, ProbeTimeout)
	defer cancel()
	conn, err := srv.Dial(dialCtx, proto, target)
	if err != nil {
		return false
	}
	defer conn.Close()

	msg := []byte("tg-e2e-probe")
	_ = conn.SetDeadline(time.Now().Add(ProbeTimeout))
	if _, err := conn.Write(msg); err != nil {
		return false
	}
	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, buf); err != nil {
		return false
	}
	return bytes.Equal(buf, msg)
}

// Netmap returns the node's compiled packet-filter rules as JSON — the input to
// the filter-landed digest. (Not a full netmap; the docker backend shells
// `tailscale debug netmap`, tsnet has a narrower local API.)
func (t *tsnetBackend) Netmap(ctx context.Context, src *Node) ([]byte, error) {
	tn := t.byName(src.Name)
	if tn == nil {
		return nil, fmt.Errorf("no tsnet node %q", src.Name)
	}
	lc, err := tn.srv.LocalClient()
	if err != nil {
		return nil, err
	}
	rules, err := lc.DebugPacketFilterRules(ctx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(rules)
}

// SSHProbe is unsupported: tsnet is userspace-only with no ssh client.
func (t *tsnetBackend) SSHProbe(context.Context, *Node, string, netip.Addr) (SSHVerdict, string, error) {
	return SSHVerdict(""), "", errSSHUnsupported
}

func (t *tsnetBackend) Stop(context.Context) error {
	t.mu.Lock()
	nodes := t.nodes
	t.nodes = nil
	t.mu.Unlock()

	var firstErr error
	for _, tn := range nodes {
		for _, ln := range tn.lns {
			_ = ln.Close()
		}
		for _, pc := range tn.pcs {
			_ = pc.Close()
		}
		if err := tn.srv.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	_ = os.RemoveAll(t.dir)
	return firstErr
}

func (t *tsnetBackend) byName(name string) *tsnetNode {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, tn := range t.nodes {
		if tn.name == name {
			return tn
		}
	}
	return nil
}

// echoTCP accepts connections until the listener closes and echoes each one.
func echoTCP(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			_, _ = io.Copy(c, c)
		}(c)
	}
}

// echoUDP echoes every datagram back to its sender until the socket closes.
func echoUDP(pc net.PacketConn) {
	buf := make([]byte, 2048)
	for {
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		if _, err := pc.WriteTo(buf[:n], addr); err != nil {
			return
		}
	}
}
