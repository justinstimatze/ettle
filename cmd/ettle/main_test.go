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

// The read-side mirror (docs/LEGIBILITY.md stage 1b): bob sees what the team's L2
// models believe ABOUT him, with stale beliefs flagged; attribution coarsened by
// default, opt-in via --by-observer. Deterministic — build the mesh from self-models,
// then drift bob so alice's model of him goes stale.
func TestMirror(t *testing.T) {
	atom := func(subj, content string) ettlemesh.Atom {
		return ettlemesh.Atom{Typ: ettlemesh.Intent, Subject: subj, Content: content, From: "bob", Confidence: 0.9}
	}
	// Round 1: seed every model from each person's self-model. alice's model of bob
	// becomes bob's round-1 beliefs.
	bobR1 := []ettlemesh.Atom{atom("rename", "renaming GetUser to FetchUser"), atom("cache", "built a user-lookup cache")}
	state := ettlemesh.NewMeshState()
	state.Advance(map[string][]ettlemesh.Atom{
		"alice": {{Typ: ettlemesh.Intent, Subject: "billing", Content: "wiring reconciliation", From: "alice", Confidence: 0.9}},
		"bob":   bobR1,
	})
	// Bob drifts: the rename belief changed; the cache belief still holds.
	bobNow := []ettlemesh.Atom{atom("rename", "actually keeping GetUser, no rename"), atom("cache", "built a user-lookup cache")}
	currSelf := map[string][]ettlemesh.Atom{"alice": {}, "bob": bobNow}

	out := captureStdout(t, func() { printMirror("bob", false, state, currSelf) })
	if !strings.Contains(out, "what the team's models believe about bob") {
		t.Fatalf("mirror header missing:\n%s", out)
	}
	if !strings.Contains(out, "built a user-lookup cache") || !strings.Contains(out, "renaming GetUser") {
		t.Fatalf("mirror must show the beliefs held about bob:\n%s", out)
	}
	if !strings.Contains(out, "stale") || !strings.Contains(out, "actually keeping GetUser") {
		t.Fatalf("the drifted belief must be flagged stale with bob's current value:\n%s", out)
	}
	// Coarsened by default: the observer's name is NOT attributed.
	if strings.Contains(out, "alice's model of you") {
		t.Fatalf("default mirror must coarsen attribution (no per-observer naming):\n%s", out)
	}

	// --by-observer opts into attribution.
	out = captureStdout(t, func() { printMirror("bob", true, state, currSelf) })
	if !strings.Contains(out, "alice's model of you") {
		t.Fatalf("--by-observer must attribute beliefs to the teammate holding them:\n%s", out)
	}
}

// Subject-gated inference (docs/LEGIBILITY.md stage 0b): an inferred atom about you is
// surfaced to you, held back from the team — your agent's guess, not a stated fact.
func TestSurfaceInferredAboutMe(t *testing.T) {
	ctx := context.Background()
	inferred := []ettlemesh.Atom{{Typ: ettlemesh.Assumption, Subject: "departure", Content: "you are leaving the team", Confidence: 0.6, Inferred: true}}

	out := captureStdout(t, func() { surface(ctx, "mabel", nil, nil, 0, nil, nil, inferred, 0, crux.Inline{}) })
	if !strings.Contains(out, "inferred about you") || !strings.Contains(out, "held back from the team") {
		t.Fatalf("an inferred atom about me must be surfaced as held-back:\n%s", out)
	}
	if !strings.Contains(out, "you are leaving the team") {
		t.Fatalf("the inferred claim must be shown to its subject:\n%s", out)
	}
	if out := captureStdout(t, func() { surface(ctx, "mabel", nil, nil, 0, nil, nil, nil, 0, crux.Inline{}) }); strings.Contains(out, "inferred about you") {
		t.Fatalf("no inferred section when nothing was inferred:\n%s", out)
	}

	// Team view (no --me): no subject to show, but the held-back count is surfaced —
	// the no-silent-drop discipline — as a count only, never the claims themselves.
	out = captureStdout(t, func() { surface(ctx, "", nil, nil, 0, nil, nil, nil, 2, crux.Inline{}) })
	if !strings.Contains(out, "inference held back") || !strings.Contains(out, "2 inferred atoms") {
		t.Fatalf("team view must surface the held-back inferred count:\n%s", out)
	}

	// In --me view the team aggregate must NOT appear — the subject sees their own
	// detail instead, and others' held-back inferences stay private (not even counted).
	out = captureStdout(t, func() { surface(ctx, "mabel", nil, nil, 0, nil, nil, inferred, 5, crux.Inline{}) })
	if strings.Contains(out, "inference held back") {
		t.Fatalf("--me view shows the subject's own detail, not the team aggregate:\n%s", out)
	}
}

// Act/ask routing (docs/LEGIBILITY.md stage 0c): a cross-person knot is posed as a
// QUESTION (the detector can't certify it); a self knot — own drift — is asserted.
func TestSurfaceActAskRouting(t *testing.T) {
	ctx := context.Background()
	cross := ettlemesh.Knot{Kind: ettlemesh.KindCollision, Parties: []string{"mabel", "opal"}, About: "metrics API shape", Explanation: "may clash", Confidence: 0.6, Votes: 5, Samples: 5}
	self := ettlemesh.Knot{Kind: ettlemesh.KindStaleAssumption, Parties: []string{"mabel"}, About: "deadline assumption", Explanation: "you assumed Friday", Confidence: 0.6, Votes: 5, Samples: 5}

	// Cross-person knot → interrogative register, never asserted as "[collision]".
	out := captureStdout(t, func() { surface(ctx, "mabel", []ettlemesh.Knot{cross}, nil, 0, nil, nil, nil, 0, crux.Inline{}) })
	if !strings.Contains(out, "worth checking together") || !strings.Contains(out, "? [possible collision]") {
		t.Fatalf("a cross-person knot must be posed as a question:\n%s", out)
	}
	if strings.Contains(out, "• [collision]") {
		t.Fatalf("a cross-person knot must NOT be asserted as a bare claim:\n%s", out)
	}
	if !strings.Contains(out, "Real, or already handled?") {
		t.Fatalf("the question framing must invite confirm/dismiss:\n%s", out)
	}

	// Self knot (own drift) → asserted in the act lane.
	out = captureStdout(t, func() { surface(ctx, "mabel", []ettlemesh.Knot{self}, nil, 0, nil, nil, nil, 0, crux.Inline{}) })
	if !strings.Contains(out, "your own assumptions") || !strings.Contains(out, "• [stale-assumption]") {
		t.Fatalf("a self knot (own drift) must be asserted, not questioned:\n%s", out)
	}
}

// Legible abstention (docs/LEGIBILITY.md stage 0a): a coupling-check suppression must
// be SHOWN — off the agenda, filtered to me — never silently dropped.
func TestSurfaceHeldBack(t *testing.T) {
	ctx := context.Background()
	mine := []ettlemesh.Knot{{Kind: ettlemesh.KindCollision, Parties: []string{"mabel", "opal"}, About: "metrics API shape", Explanation: "producer/consumer, not a clash", Confidence: 0.5}}

	out := captureStdout(t, func() { surface(ctx, "mabel", nil, mine, 0, nil, nil, nil, 0, crux.Inline{}) })
	if !strings.Contains(out, "held back") {
		t.Fatalf("a suppressed knot must surface a held-back section:\n%s", out)
	}
	if !strings.Contains(out, "metrics API shape") {
		t.Fatalf("held-back section must list the suppressed knot:\n%s", out)
	}

	// Absent when nothing was held back.
	if out := captureStdout(t, func() { surface(ctx, "mabel", nil, nil, 0, nil, nil, nil, 0, crux.Inline{}) }); strings.Contains(out, "held back") {
		t.Fatalf("no held-back section when nothing was suppressed:\n%s", out)
	}

	// Filtered to me: a suppression about other people must not appear in my horizon.
	notMine := []ettlemesh.Knot{{Kind: ettlemesh.KindCollision, Parties: []string{"nash", "reed"}, About: "not mine"}}
	if out := captureStdout(t, func() { surface(ctx, "mabel", nil, notMine, 0, nil, nil, nil, 0, crux.Inline{}) }); strings.Contains(out, "held back") {
		t.Fatalf("held-back must be filtered to me:\n%s", out)
	}

	// Floor drops are NOT listed — they surface as a single quiet aggregate count.
	out = captureStdout(t, func() { surface(ctx, "mabel", nil, nil, 3, nil, nil, nil, 0, crux.Inline{}) })
	if !strings.Contains(out, "below the confidence floor") {
		t.Fatalf("a positive floor count must surface the aggregate line:\n%s", out)
	}
	if !strings.Contains(out, "3 low-recurrence") {
		t.Fatalf("floor line must report the count (3):\n%s", out)
	}
	if out := captureStdout(t, func() { surface(ctx, "mabel", nil, nil, 0, nil, nil, nil, 0, crux.Inline{}) }); strings.Contains(out, "below the confidence floor") {
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
