package ettlemesh

import (
	"strings"
	"testing"
)

func ck(kind string, parties ...string) Knot {
	return Knot{Kind: kind, Parties: parties, Confidence: 1.0}
}

// buildGroundPrompt must number knots by their FULL-SLICE index, not their position
// within idxs — otherwise the model's verdict (keyed by the shown index) misses its
// knot in applyGroundingVerdicts (keyed by full-slice index) whenever a non-groundable
// knot precedes a groundable one. This pins the verdict-mapping fix.
func TestBuildGroundPromptNumbersBySliceIndex(t *testing.T) {
	knots := []Knot{
		ck(KindStaleAssumption, "cleo"),   // 0: self knot — not groundable
		ck(KindCollision, "alice", "bob"), // 1: groundable
		ck(KindCollision, "dao", "evan"),  // 2: groundable
	}
	got := buildGroundPrompt(KindCollision, []int{1, 2}, knots, nil)
	if !strings.Contains(got, "Knot 1 — [collision]") || !strings.Contains(got, "Knot 2 — [collision]") {
		t.Fatalf("want knots numbered by full-slice index (1, 2), got:\n%s", got)
	}
	if strings.Contains(got, "Knot 0 —") {
		t.Fatalf("numbered by subset position (0-based) — the verdict-mapping bug:\n%s", got)
	}
	// Focused prompt: only the collision test, never the duplication/teamwide text.
	if strings.Contains(got, "HTTP retry helpers") || strings.Contains(got, "freeze on the 27th") {
		t.Fatalf("collision prompt leaked another kind's guidance:\n%s", got)
	}
}

func TestMultiPerson(t *testing.T) {
	cases := []struct {
		parties []string
		want    bool
	}{
		{nil, false},
		{[]string{"alice"}, false},
		{[]string{"alice", "Alice"}, false}, // same person, different case
		{[]string{"alice", "bob"}, true},
		{[]string{"alice", "alice", "dao"}, true},
	}
	for _, c := range cases {
		if got := multiPerson(c.parties); got != c.want {
			t.Errorf("multiPerson(%v) = %v, want %v", c.parties, got, c.want)
		}
	}
}

func TestGroundableKnots(t *testing.T) {
	knots := []Knot{
		ck(KindCollision, "alice", "bob"),                 // 0: checkable
		ck(KindDuplication, "evan", "fay"),                // 1: checkable (broadened)
		ck(KindTeamwideDivergence, "jun", "kara", "liam"), // 2: checkable (broadened)
		ck(KindDecisionRights, "alice", "bob"),            // 3: excluded — who-decides
		ck(KindCollision, "alice", "Alice"),               // 4: excluded — single person
		ck(KindStaleAssumption, "cleo"),                   // 5: excluded — self knot
		ck(KindCollision, "alice"),                        // 6: excluded — one party
	}
	got := groundableKnots(knots)
	want := []int{0, 1, 2}
	if len(got) != len(want) {
		t.Fatalf("groundableKnots = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("groundableKnots = %v, want %v", got, want)
		}
	}
}

func TestApplyGroundingVerdictsDropsUngroundedCrossPerson(t *testing.T) {
	knots := []Knot{
		ck(KindCollision, "alice", "bob"), // 0: grounded → keep
		ck(KindCollision, "alice", "dao"), // 1: NOT grounded → drop
		ck(KindStaleAssumption, "cleo"),   // 2: self knot → always keep
	}
	verdicts := map[int]bool{0: true, 1: false}
	out := applyGroundingVerdicts(knots, verdicts)
	if len(out) != 2 {
		t.Fatalf("expected 2 knots (alice×bob + cleo self), got %d: %+v", len(out), out)
	}
	for _, k := range out {
		if multiPerson(k.Parties) && SamePerson(k.Parties[1], "dao") {
			t.Fatalf("the ungrounded alice×dao knot should have been dropped")
		}
	}
}

func TestApplyGroundingVerdictsFailsOpen(t *testing.T) {
	// A multi-person knot with NO returned verdict must be KEPT (protects recall
	// if the verifier garbles or omits it).
	knots := []Knot{ck(KindCollision, "alice", "bob")}
	out := applyGroundingVerdicts(knots, map[int]bool{}) // no verdict for index 0
	if len(out) != 1 {
		t.Fatalf("unjudged knot must survive (fail open), got %d", len(out))
	}
}

func TestApplyGroundingVerdictsSelfKnotsNeverChecked(t *testing.T) {
	// A self knot keeps even with a (mistaken) false verdict against its index.
	knots := []Knot{ck(KindStaleAssumption, "alice")}
	out := applyGroundingVerdicts(knots, map[int]bool{0: false})
	if len(out) != 1 {
		t.Fatalf("a single-author knot must never be dropped by grounding, got %d", len(out))
	}
}

func TestApplyGroundingVerdictsDoesNotAliasInput(t *testing.T) {
	knots := []Knot{
		ck(KindCollision, "alice", "dao"), // will be dropped
		ck(KindCollision, "alice", "bob"), // kept
	}
	_ = applyGroundingVerdicts(knots, map[int]bool{0: false, 1: true})
	// The original slice must be untouched (out used a fresh backing array).
	if len(knots) != 2 || !SamePerson(knots[0].Parties[1], "dao") {
		t.Fatalf("input slice was mutated by applyGroundingVerdicts: %+v", knots)
	}
}
