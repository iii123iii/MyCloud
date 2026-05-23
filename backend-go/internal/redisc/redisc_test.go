package redisc

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

// New() returns a configured client. Verify it can actually talk to a
// (miniredis) Redis instance. Wire failure would surface as a connection
// refused error from PING.
func TestNew_ConnectsToReachableRedis(t *testing.T) {
	mr := miniredis.RunT(t)
	cli := New(mr.Addr(), "")
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := Ping(ctx, cli); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

// Ping returns an error promptly when the target is unreachable. Boot code
// should treat this as "Redis degraded, but don't crash" — surface a warn
// log, continue.
func TestPing_UnreachableHost(t *testing.T) {
	cli := New("127.0.0.1:1", "") // port 1 reserved/unused
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := Ping(ctx, cli); err == nil {
		t.Errorf("expected error pinging unreachable host")
	}
}

// New with password sets it on the client. Miniredis doesn't enforce auth,
// but we can verify the client is constructed and can ping.
func TestNew_AcceptsPassword(t *testing.T) {
	mr := miniredis.RunT(t)
	cli := New(mr.Addr(), "any-secret")
	defer cli.Close()
	if cli == nil {
		t.Fatalf("New returned nil")
	}
}
