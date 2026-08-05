package main

import (
	"strings"
	"testing"
	"time"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/transport"
)

func firmT(kind string, parties ...string) ettlemesh.Tangle {
	return ettlemesh.Tangle{Kind: kind, Parties: parties, About: "session storage", Explanation: "conflict", Confidence: 0.9}
}
func softT(kind string, parties ...string) ettlemesh.Tangle {
	return ettlemesh.Tangle{Kind: kind, Parties: parties, About: "logging", Explanation: "maybe", Confidence: 0.3}
}

func TestClassifyHorizonSplitsAndFiltersByMe(t *testing.T) {
	kept := []ettlemesh.Tangle{
		firmT("collision", "Alice", "Bob"),     // alice: firm
		softT("duplication", "Alice", "Carol"), // alice: soft
		firmT("collision", "Carol", "Dave"),    // not alice: dropped
	}
	suppressed := []ettlemesh.Tangle{
		firmT("teamwide-divergence", "Alice", "Eve"), // alice: held
		firmT("collision", "Frank", "Gina"),          // not alice: dropped
	}
	res := classifyHorizon(kept, suppressed, []string{"Alice", "Bob"}, 2, "Alice")
	if len(res.firm) != 1 || len(res.soft) != 1 {
		t.Fatalf("firm=%d soft=%d, want 1/1", len(res.firm), len(res.soft))
	}
	if len(res.held) != 1 {
		t.Fatalf("held=%d, want 1 (only alice's suppressed)", len(res.held))
	}
	if res.floorHeld != 2 {
		t.Errorf("floorHeld carried wrong: %d", res.floorHeld)
	}
}

func TestClassifyHorizonWholeTeam(t *testing.T) {
	kept := []ettlemesh.Tangle{firmT("collision", "Carol", "Dave"), softT("duplication", "Eve", "Frank")}
	res := classifyHorizon(kept, nil, nil, 0, "") // empty me = whole team, no filter
	if len(res.firm) != 1 || len(res.soft) != 1 {
		t.Fatalf("whole-team view should keep all: firm=%d soft=%d", len(res.firm), len(res.soft))
	}
}

func TestRenderHorizonBlockClear(t *testing.T) {
	res := horizonResult{participants: []string{"alice", "bob"}}
	got := renderHorizonBlock(res, "alice", time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC))
	if !strings.Contains(got, "Horizon clear") {
		t.Errorf("clear horizon should say so:\n%s", got)
	}
	if !strings.Contains(got, "2 participants") {
		t.Errorf("clear horizon should note who's on the bus:\n%s", got)
	}
}

func TestRenderHorizonBlockFirmSoft(t *testing.T) {
	res := horizonResult{
		firm: []ettlemesh.Tangle{firmT("collision", "alice", "bob")},
		soft: []ettlemesh.Tangle{softT("duplication", "alice", "carol")},
		held: []ettlemesh.Tangle{firmT("teamwide-divergence", "alice", "dave")},
	}
	got := renderHorizonBlock(res, "alice", time.Now().UTC())
	for _, want := range []string{"Firm (worth a look)", "Soft (worth a question)", "collision", "alice, bob", "held back"} {
		if !strings.Contains(got, want) {
			t.Errorf("render missing %q:\n%s", want, got)
		}
	}
	// Whisper-first framing must be present so it never reads as something to post.
	if !strings.Contains(got, "nothing is posted") {
		t.Errorf("render should carry the whisper-first framing:\n%s", got)
	}
}

func TestTagHorizonSuppressesMutedAndFlagsEscalated(t *testing.T) {
	res := horizonResult{
		firm: []ettlemesh.Tangle{firmT("collision", "alice", "bob"), firmT("duplication", "alice", "carol")},
		soft: []ettlemesh.Tangle{softT("stale-assumption", "alice", "dave")},
	}
	muted := map[string]bool{escalateKey(firmT("duplication", "alice", "carol")): true}
	escalated := map[string]bool{escalateKey(firmT("collision", "alice", "bob")): true}
	got := tagHorizon(res, muted, escalated)
	if len(got.firm) != 1 || got.firm[0].Kind != "collision" {
		t.Fatalf("muted duplication should be dropped from firm, got %+v", got.firm)
	}
	if got.muted != 1 {
		t.Errorf("muted count should be 1, got %d", got.muted)
	}
	if got.escalated == nil {
		t.Error("escalated set should be non-nil for a tagged (Linear) horizon")
	}
}

func TestRenderHorizonBlockAgentFramedWithShareTags(t *testing.T) {
	now := time.Now().UTC()
	collision := firmT("collision", "alice", "bob")
	// Linear room (escalated non-nil), knot not yet escalated.
	res := horizonResult{firm: []ettlemesh.Tangle{collision}, escalated: map[string]bool{}}
	got := renderHorizonBlock(res, "alice", now)
	// "· not yet shared" is the per-knot tag (the instruction line mentions the phrase
	// in backticks, so the middot separator is what pins it to the bullet).
	for _, want := range []string{"You are alice's ettle agent", "· not yet shared", "ettle_escalate"} {
		if !strings.Contains(got, want) {
			t.Errorf("agent-framed block missing %q:\n%s", want, got)
		}
	}
	// Once escalated, the same knot reads as shared and the per-knot offer tag is gone.
	res.escalated = map[string]bool{escalateKey(collision): true}
	got = renderHorizonBlock(res, "alice", now)
	if !strings.Contains(got, "· shared with the team") || strings.Contains(got, "· not yet shared") {
		t.Errorf("escalated knot should read as shared:\n%s", got)
	}
	// A non-Linear horizon (escalated nil) shows no share tags at all.
	plain := renderHorizonBlock(horizonResult{firm: []ettlemesh.Tangle{collision}}, "alice", now)
	if strings.Contains(plain, "· not yet shared") || strings.Contains(plain, "ettle_escalate") {
		t.Errorf("non-Linear horizon must not show escalation tags:\n%s", plain)
	}
}

func TestRenderHorizonBlockMutedCount(t *testing.T) {
	got := renderHorizonBlock(horizonResult{participants: []string{"a", "b"}, muted: 2}, "alice", time.Now().UTC())
	if !strings.Contains(got, "2 knots muted") {
		t.Errorf("a clear-by-muting horizon should say so:\n%s", got)
	}
}

func TestFoldLatestKeepsLastPerParticipant(t *testing.T) {
	envs := []transport.Envelope{
		{Participant: "alice", Atoms: []ettlemesh.Atom{{Content: "old"}}},
		{Participant: "bob", Atoms: []ettlemesh.Atom{{Content: "b"}}},
		{Participant: "Alice", Atoms: []ettlemesh.Atom{{Content: "new"}}}, // same person, later
	}
	out := foldLatest(envs)
	if len(out) != 2 {
		t.Fatalf("want 2 participants after fold, got %d", len(out))
	}
	for _, e := range out {
		if strings.EqualFold(e.Participant, "alice") && e.Atoms[0].Content != "new" {
			t.Errorf("fold should keep the later alice envelope, got %q", e.Atoms[0].Content)
		}
	}
}

func TestHorizonCacheRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if got, err := readHorizonCache("room", "alice"); err != nil || got != "" {
		t.Fatalf("cold cache should be empty: got %q err %v", got, err)
	}
	if err := writeHorizonCache("room", "alice", "# horizon\n- thing"); err != nil {
		t.Fatal(err)
	}
	got, err := readHorizonCache("room", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if got != "# horizon\n- thing" {
		t.Errorf("round-trip mismatch: %q", got)
	}
	// A different identity has its own cache (no cross-person bleed).
	if other, _ := readHorizonCache("room", "bob"); other != "" {
		t.Errorf("bob should not see alice's cache: %q", other)
	}
}

func TestDueForHorizonDebounces(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if due, _ := dueForHorizon("room", "alice", time.Hour); !due {
		t.Fatal("first call should be due")
	}
	if due, _ := dueForHorizon("room", "alice", time.Hour); due {
		t.Fatal("second call inside the window should be debounced")
	}
	// Keyed per identity: bob is still due despite alice's recent refresh.
	if due, _ := dueForHorizon("room", "bob", time.Hour); !due {
		t.Fatal("a different identity should refresh independently")
	}
}
