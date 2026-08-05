package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
)

func TestEscalateKeyIsWordingIndependent(t *testing.T) {
	a := escalateKey(ettlemesh.Tangle{Kind: "collision", Parties: []string{"Bob", "Alice"}, Explanation: "one wording"})
	b := escalateKey(ettlemesh.Tangle{Kind: "collision", Parties: []string{"alice", "bob", "Alice"}, Explanation: "different wording"})
	if a != b {
		t.Errorf("key should be kind + sorted distinct parties, got %q vs %q", a, b)
	}
	if a != "collision|alice+bob" {
		t.Errorf("unexpected key %q", a)
	}
}

func TestEscalatableKnotsFirmCrossPersonNew(t *testing.T) {
	already := map[string]bool{escalateKey(firmT("collision", "alice", "bob")): true}
	res := horizonResult{
		firm: []ettlemesh.Tangle{
			firmT("collision", "alice", "bob"),     // already emitted → skip
			firmT("duplication", "alice", "carol"), // new cross-person → keep
			firmT("stale-assumption", "alice"),     // self (single party) → skip
		},
		soft: []ettlemesh.Tangle{firmT("collision", "dave", "eve")}, // soft is never escalated
	}
	muted := map[string]bool{escalateKey(firmT("duplication", "alice", "carol")): true} // muted → also skipped
	got := escalatableKnots(res, already, muted)
	if len(got) != 0 {
		t.Fatalf("the only remaining firm cross-person knot is muted, so nothing should escalate, got %d", len(got))
	}
	// Without the mute, that knot escalates.
	got = escalatableKnots(res, already, map[string]bool{})
	if len(got) != 1 || escalateKey(got[0]) != "duplication|alice+carol" {
		t.Fatalf("want the one new cross-person firm knot, got %d", len(got))
	}
}

func TestRenderKnotBody(t *testing.T) {
	body := renderKnotBody(firmT("collision", "alice", "bob"))
	for _, want := range []string{"collision", "alice, bob", "Reply here"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}

// fakePoster records the escalate write orchestration without a network or token.
type fakePoster struct {
	ensureRoom, ensureTeam string
	opened                 []string
	postedBodies           []string
	failAt                 int // 1-based PostKnot call to fail; 0 = never
	n                      int
}

func (f *fakePoster) EnsureCoordinationIssue(_ context.Context, room, team string) (string, bool, error) {
	f.ensureRoom, f.ensureTeam = room, team
	return "issue-1", true, nil
}
func (f *fakePoster) OpenSession(_ context.Context, issueID string) (string, error) {
	f.opened = append(f.opened, issueID)
	return "sess-1", nil
}
func (f *fakePoster) PostKnot(_ context.Context, _ /*sid*/, body string) (string, error) {
	f.n++
	if f.failAt > 0 && f.n == f.failAt {
		return "", fmt.Errorf("boom")
	}
	f.postedBodies = append(f.postedBodies, body)
	return fmt.Sprintf("act-%d", f.n), nil
}

func TestPostKnotsEnsuresIssueOpensSessionPostsEach(t *testing.T) {
	f := &fakePoster{}
	knots := []ettlemesh.Tangle{firmT("collision", "alice", "bob"), firmT("duplication", "alice", "carol")}
	issueID, created, keys, err := postKnots(context.Background(), f, "myroom", "team-1", knots)
	if err != nil {
		t.Fatal(err)
	}
	if issueID != "issue-1" || !created {
		t.Errorf("issue=%q created=%v", issueID, created)
	}
	if f.ensureRoom != "myroom" || f.ensureTeam != "team-1" {
		t.Errorf("ensure got room=%q team=%q", f.ensureRoom, f.ensureTeam)
	}
	if len(f.opened) != 1 || f.opened[0] != "issue-1" {
		t.Errorf("session should open once on the issue, got %v", f.opened)
	}
	if len(keys) != 2 || len(f.postedBodies) != 2 {
		t.Fatalf("want 2 posted, got keys=%d bodies=%d", len(keys), len(f.postedBodies))
	}
}

func TestPostKnotsRecordsProgressOnPartialFailure(t *testing.T) {
	f := &fakePoster{failAt: 2}
	knots := []ettlemesh.Tangle{firmT("collision", "alice", "bob"), firmT("duplication", "alice", "carol"), firmT("collision", "eve", "frank")}
	_, _, keys, err := postKnots(context.Background(), f, "r", "team", knots)
	if err == nil {
		t.Fatal("expected the second post to fail")
	}
	if len(keys) != 1 {
		t.Fatalf("only the first knot posted before the failure, want 1 key, got %d", len(keys))
	}
	if keys[0] != "collision|alice+bob" {
		t.Errorf("wrong first key %q", keys[0])
	}
}

func TestEmittedStoreRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if set, err := loadEmitted("room"); err != nil || len(set) != 0 {
		t.Fatalf("cold store should be empty: %v %v", set, err)
	}
	if err := saveEmitted("room", map[string]bool{"collision|alice+bob": true, "duplication|carol+dave": true}); err != nil {
		t.Fatal(err)
	}
	set, err := loadEmitted("room")
	if err != nil {
		t.Fatal(err)
	}
	if !set["collision|alice+bob"] || !set["duplication|carol+dave"] || len(set) != 2 {
		t.Errorf("round-trip mismatch: %v", set)
	}
}
