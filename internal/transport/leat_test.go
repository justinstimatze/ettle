package transport

import (
	"context"
	"os/exec"
	"testing"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
)

// TestLeatRoundTrip exercises the leat adapter over a real LOCAL git repo (no
// remote): publish a participant's atoms, collect them back, and confirm the
// ettle Envelope round-trips through the leat record Body — plus that a second
// publish for the same participant replaces the first (LWW per participant).
// git must be on PATH; skipped otherwise.
func TestLeatRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	// leat.New needs a born HEAD; an empty initial commit gives it one.
	runGit(t, dir, "-c", "user.name=t", "-c", "user.email=t@x", "commit", "--allow-empty", "-m", "init")

	bus, err := NewLeatBus(dir, "alice", "", "testroom")
	if err != nil {
		t.Fatalf("NewLeatBus: %v", err)
	}
	defer bus.Close()

	ctx := context.Background()
	env := Envelope{
		Participant: "alice",
		Role:        "user-service",
		Atoms: []ettlemesh.Atom{
			{From: "alice", Typ: ettlemesh.Intent, Subject: "rename", Content: "renaming GetUser to FetchUser", Confidence: 0.9},
		},
	}
	if err := bus.Publish(ctx, env); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	got, err := bus.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d envelopes, want 1: %+v", len(got), got)
	}
	if got[0].Participant != "alice" || got[0].Role != "user-service" {
		t.Fatalf("envelope header lost in round-trip: %+v", got[0])
	}
	if len(got[0].Atoms) != 1 || got[0].Atoms[0].Content != "renaming GetUser to FetchUser" {
		t.Fatalf("atoms lost in round-trip: %+v", got[0].Atoms)
	}

	// LWW: a second publish for the same participant replaces the first set.
	env.Atoms[0].Content = "actually keeping GetUser, no rename"
	if err := bus.Publish(ctx, env); err != nil {
		t.Fatalf("Publish 2: %v", err)
	}
	got, err = bus.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect 2: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("LWW failed: got %d envelopes, want 1 (latest per participant)", len(got))
	}
	if got[0].Atoms[0].Content != "actually keeping GetUser, no rename" {
		t.Fatalf("LWW kept the stale atom set: %+v", got[0].Atoms)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
