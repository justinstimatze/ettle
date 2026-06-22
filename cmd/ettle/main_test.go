package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justinstimatze/ettle/internal/crux"
	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/transport"
)

// captureStdout runs f with os.Stdout redirected to a pipe and returns what it wrote.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	f()
	_ = w.Close()
	os.Stdout = old
	b, _ := io.ReadAll(r)
	return string(b)
}

// Legible abstention (docs/LEGIBILITY.md stage 0a): a coupling-check suppression must
// be SHOWN — off the agenda, filtered to me — never silently dropped.
func TestSurfaceHeldBack(t *testing.T) {
	ctx := context.Background()
	mine := []ettlemesh.Knot{{Kind: ettlemesh.KindCollision, Parties: []string{"mabel", "opal"}, About: "metrics API shape", Explanation: "producer/consumer, not a clash", Confidence: 0.5}}

	out := captureStdout(t, func() { surface(ctx, "mabel", nil, mine, 0, nil, nil, crux.Inline{}) })
	if !strings.Contains(out, "held back") {
		t.Fatalf("a suppressed knot must surface a held-back section:\n%s", out)
	}
	if !strings.Contains(out, "metrics API shape") {
		t.Fatalf("held-back section must list the suppressed knot:\n%s", out)
	}

	// Absent when nothing was held back.
	if out := captureStdout(t, func() { surface(ctx, "mabel", nil, nil, 0, nil, nil, crux.Inline{}) }); strings.Contains(out, "held back") {
		t.Fatalf("no held-back section when nothing was suppressed:\n%s", out)
	}

	// Filtered to me: a suppression about other people must not appear in my horizon.
	notMine := []ettlemesh.Knot{{Kind: ettlemesh.KindCollision, Parties: []string{"nash", "reed"}, About: "not mine"}}
	if out := captureStdout(t, func() { surface(ctx, "mabel", nil, notMine, 0, nil, nil, crux.Inline{}) }); strings.Contains(out, "held back") {
		t.Fatalf("held-back must be filtered to me:\n%s", out)
	}

	// Floor drops are NOT listed — they surface as a single quiet aggregate count.
	out = captureStdout(t, func() { surface(ctx, "mabel", nil, nil, 3, nil, nil, crux.Inline{}) })
	if !strings.Contains(out, "below the confidence floor") {
		t.Fatalf("a positive floor count must surface the aggregate line:\n%s", out)
	}
	if !strings.Contains(out, "3 low-recurrence") {
		t.Fatalf("floor line must report the count (3):\n%s", out)
	}
	if out := captureStdout(t, func() { surface(ctx, "mabel", nil, nil, 0, nil, nil, crux.Inline{}) }); strings.Contains(out, "below the confidence floor") {
		t.Fatalf("no floor line when floorHeld==0:\n%s", out)
	}
}

func TestLoadParticipants(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	withHeader := write("zeb.md", "name: Alice\nrole: user-service\n\nrenaming GetUser today\n")
	noHeader := write("bob.md", "just some working notes\n")

	people, err := loadParticipants([]string{withHeader, noHeader})
	if err != nil {
		t.Fatal(err)
	}
	if len(people) != 2 {
		t.Fatalf("got %d participants, want 2", len(people))
	}
	// Sorted by name: "Alice" < "bob" (uppercase sorts first).
	if people[0].Name != "Alice" || people[0].Role != "user-service" || people[0].Notes != "renaming GetUser today" {
		t.Errorf("header parse: %+v", people[0])
	}
	// No header → filename (sans ext) is the name, role empty, whole file is the note.
	if people[1].Name != "bob" || people[1].Role != "" || people[1].Notes != "just some working notes" {
		t.Errorf("headerless parse: %+v", people[1])
	}
}

func TestHasParticipant(t *testing.T) {
	people := []participant{{Name: "alice"}, {Name: "Bob"}}
	if !hasParticipant(people, "ALICE") {
		t.Error("hasParticipant should be case-insensitive")
	}
	if hasParticipant(people, "alie") {
		t.Error("typo'd name must not match — that's the false-all-clear guard")
	}
}

func TestPartyOf(t *testing.T) {
	k := ettlemesh.Knot{Parties: []string{"alice", " bob "}}
	if !partyOf(k, "Bob") {
		t.Error("partyOf should trim + case-fold")
	}
	if partyOf(k, "carol") {
		t.Error("carol is not a party")
	}
}

func TestDistinctParticipants(t *testing.T) {
	envs := []transport.Envelope{
		{Participant: "alice"},
		{Participant: "Alice"}, // dup (case)
		{Participant: " bob "}, // dup-normalized distinct person
	}
	if got := distinctParticipants(envs); got != 2 {
		t.Errorf("got %d, want 2 distinct (alice, bob)", got)
	}
}
