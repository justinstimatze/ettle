package main

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

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

// TestRenderRoomStatus pins the presence view: participants sorted, atoms framed
// by type ("working on" for intent), and per-person freshness from EmittedAt.
func TestRenderRoomStatus(t *testing.T) {
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	envs := []transport.Envelope{
		{
			Participant: "bob", Role: "checkout",
			EmittedAt: now.Add(-3 * 24 * time.Hour).Format(time.RFC3339),
			Atoms: []ettlemesh.Atom{
				{Typ: ettlemesh.Dependency, Subject: "pricing", Content: "calls pricing in-process"},
			},
		},
		{
			Participant: "alice", Role: "user-service",
			EmittedAt: now.Add(-30 * time.Minute).Format(time.RFC3339),
			Atoms: []ettlemesh.Atom{
				{Typ: ettlemesh.Intent, Subject: "rename", Content: "renaming GetUser"},
			},
		},
	}
	out := renderRoomStatus("crew", envs, nil, now)

	if !strings.Contains(out, `room "crew" — 2 present`) {
		t.Fatalf("header/count missing:\n%s", out)
	}
	// alice (active) must sort before bob (3d ago).
	if strings.Index(out, "alice") > strings.Index(out, "bob") {
		t.Fatalf("participants not sorted:\n%s", out)
	}
	if !strings.Contains(out, "alice (user-service) · active") {
		t.Fatalf("freshness 'active' missing for alice:\n%s", out)
	}
	if !strings.Contains(out, "3d ago") {
		t.Fatalf("freshness '3d ago' missing for bob:\n%s", out)
	}
	if !strings.Contains(out, "working on:") || !strings.Contains(out, "renaming GetUser") {
		t.Fatalf("intent must render as 'working on':\n%s", out)
	}
	if !strings.Contains(out, "depends on:") {
		t.Fatalf("dependency must render as 'depends on':\n%s", out)
	}

	// Empty room gives the join hint, not a bare header.
	if empty := renderRoomStatus("crew", nil, nil, now); !strings.Contains(empty, "nobody has published yet") {
		t.Fatalf("empty room should hint how to publish:\n%s", empty)
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
