//go:build e2e

package e2e

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"
)

func TestAwaitConverged(t *testing.T) {
	a := &Node{Name: "a", IPv4: netip.MustParseAddr("100.64.0.1")}
	b := &Node{Name: "b", IPv4: netip.MustParseAddr("100.64.0.2")}
	stale := netip.MustParseAddr("100.64.0.9") // not one of ours — ignored

	// b only sees a from the third poll on; a sees b (and a stale device) at once.
	polls := 0
	onlinePeers := func(_ context.Context, n *Node) (map[netip.Addr]bool, string) {
		if n == a {
			return map[netip.Addr]bool{b.IPv4: true, stale: true}, ""
		}
		polls++
		if polls == 1 {
			return nil, "status err"
		}
		return map[netip.Addr]bool{a.IPv4: polls >= 3}, ""
	}
	if err := awaitConverged(context.Background(), []*Node{a, b}, nil, "nodes", time.Millisecond, onlinePeers); err != nil {
		t.Fatalf("awaitConverged: %v", err)
	}
	if polls != 3 {
		t.Errorf("polled b %d times, want 3", polls)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	never := func(context.Context, *Node) (map[netip.Addr]bool, string) { return nil, "gone" }
	if err := awaitConverged(ctx, []*Node{a, b}, nil, "nodes", time.Millisecond, never); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled wait: err = %v, want context.Canceled", err)
	}
}
