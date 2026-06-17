//go:build nats

package transport

import (
	"context"
	"testing"
	"time"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
	natstest "github.com/nats-io/nats-server/v2/test"
)

// TestNATSRoundTrip exercises the real wire path against an embedded JetStream
// server: publish-then-collect (the driver's actual ordering), retention so a
// late Collect still sees earlier publishers, and latest-wins dedup per
// participant. This is the case core pub/sub would silently drop.
func TestNATSRoundTrip(t *testing.T) {
	opts := natstest.DefaultTestOptions
	opts.Port = -1 // random free port
	opts.JetStream = true
	opts.StoreDir = t.TempDir()
	srv := natstest.RunServer(&opts)
	defer srv.Shutdown()

	cfg := NATSConfig{URL: srv.ClientURL(), InsecureTCP: true, Team: "test team", GatherFor: 1500 * time.Millisecond}
	bus, err := DialNATS(cfg)
	if err != nil {
		t.Fatalf("DialNATS: %v", err)
	}
	defer bus.Close()

	ctx := context.Background()
	// Publish BEFORE Collect subscribes — the ordering core NATS would drop.
	mustPublish(t, bus, "alice", []ettlemesh.Atom{{From: "alice", Subject: "stale", Content: "v1"}})
	mustPublish(t, bus, "bob", []ettlemesh.Atom{{From: "bob", Subject: "x", Content: "y"}})
	// Re-publish alice — latest should win.
	mustPublish(t, bus, "alice", []ettlemesh.Atom{{From: "alice", Subject: "fresh", Content: "v2"}})

	envs, err := bus.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got := distinct(envs); got != 2 {
		t.Fatalf("got %d distinct participants, want 2 (alice, bob)", got)
	}
	for _, e := range envs {
		if e.Participant == "alice" {
			if len(e.Atoms) != 1 || e.Atoms[0].Subject != "fresh" {
				t.Errorf("alice envelope not the latest: %+v", e.Atoms)
			}
		}
	}
}

func TestNATSInsecureRefusesRemote(t *testing.T) {
	_, err := DialNATS(NATSConfig{URL: "nats://nats.example.com:4222", InsecureTCP: true})
	if err == nil {
		t.Fatal("expected refusal: InsecureTCP against a non-localhost URL must error")
	}
}

func mustPublish(t *testing.T, bus *NATSBus, who string, atoms []ettlemesh.Atom) {
	t.Helper()
	if err := bus.Publish(context.Background(), Envelope{Participant: who, Atoms: atoms}); err != nil {
		t.Fatalf("Publish(%s): %v", who, err)
	}
}

func distinct(envs []Envelope) int {
	seen := map[string]bool{}
	for _, e := range envs {
		seen[e.Participant] = true
	}
	return len(seen)
}
