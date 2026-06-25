package main

import (
	"context"
	"os/exec"
	"testing"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/transport"
)

// TestRoomInitLocalAndBus drives the room command end to end with a local-only
// room (no remote): init writes a usable config + seeded repo, and the room's
// resolved transport round-trips an envelope. git must be on PATH; skipped
// otherwise. UserConfigDir is redirected to a temp dir via XDG_CONFIG_HOME.
func TestRoomInitLocalAndBus(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // os.UserConfigDir honors this on Linux

	if err := roomInit([]string{"--as", "Alice Smith", "--name", "crew"}); err != nil {
		t.Fatalf("roomInit: %v", err)
	}

	rc, err := loadRoom("crew")
	if err != nil {
		t.Fatalf("loadRoom: %v", err)
	}
	if rc.Agent != "Alice_Smith" { // sanitized to a valid leat id
		t.Fatalf("agent id not sanitized: %q", rc.Agent)
	}
	if rc.Remote != "" {
		t.Fatalf("local room should have no remote, got %q", rc.Remote)
	}

	bus, err := roomBus("crew")
	if err != nil {
		t.Fatalf("roomBus: %v", err)
	}
	defer bus.Close()

	ctx := context.Background()
	env := transport.Envelope{
		Participant: "Alice Smith",
		Atoms: []ettlemesh.Atom{
			{From: "alice", Typ: ettlemesh.Intent, Subject: "x", Content: "wiring the room", Confidence: 0.9},
		},
	}
	if err := bus.Publish(ctx, env); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	got, err := bus.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(got) != 1 || len(got[0].Atoms) != 1 || got[0].Atoms[0].Content != "wiring the room" {
		t.Fatalf("round-trip through the room failed: %+v", got)
	}
}

func TestRoomNameFromURL(t *testing.T) {
	cases := map[string]string{
		"git@github.com:crew/standup-room.git":     "standup-room",
		"https://github.com/crew/standup-room.git": "standup-room",
		"https://gitlab.com/team/ettle-room/":      "ettle-room",
	}
	for url, want := range cases {
		if got := roomNameFromURL(url); got != want {
			t.Errorf("roomNameFromURL(%q) = %q, want %q", url, got, want)
		}
	}
}
