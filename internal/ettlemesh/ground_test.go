package ettlemesh

import (
	"strings"
	"testing"
)

func ck(kind string, parties ...string) Tangle {
	return Tangle{Kind: kind, Parties: parties, Confidence: 1.0}
}

// buildGroundPrompt must number tangles by their FULL-SLICE index, not their position
// within idxs — otherwise the model's verdict (keyed by the shown index) misses its
// tangle in applyGroundingVerdicts (keyed by full-slice index) whenever a non-groundable
// tangle precedes a groundable one. This pins the verdict-mapping fix.
func TestBuildGroundPromptNumbersBySliceIndex(t *testing.T) {
	tangles := []Tangle{
		ck(KindStaleAssumption, "cleo"),   // 0: self tangle — not groundable
		ck(KindCollision, "alice", "bob"), // 1: groundable
		ck(KindCollision, "dao", "evan"),  // 2: groundable
	}
	got := buildGroundPrompt(KindCollision, []int{1, 2}, tangles, nil)
	if !strings.Contains(got, "Tangle 1 — [collision]") || !strings.Contains(got, "Tangle 2 — [collision]") {
		t.Fatalf("want tangles numbered by full-slice index (1, 2), got:\n%s", got)
	}
	if strings.Contains(got, "Tangle 0 —") {
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
		if got := MultiPerson(c.parties); got != c.want {
			t.Errorf("MultiPerson(%v) = %v, want %v", c.parties, got, c.want)
		}
	}
}

func TestGroundableTangles(t *testing.T) {
	tangles := []Tangle{
		ck(KindCollision, "alice", "bob"),                 // 0: checkable
		ck(KindDuplication, "evan", "fay"),                // 1: checkable (broadened)
		ck(KindTeamwideDivergence, "jun", "kara", "liam"), // 2: checkable (broadened)
		ck(KindDecisionRights, "alice", "bob"),            // 3: excluded — who-decides
		ck(KindCollision, "alice", "Alice"),               // 4: excluded — single person
		ck(KindStaleAssumption, "cleo"),                   // 5: excluded — self tangle
		ck(KindCollision, "alice"),                        // 6: excluded — one party
	}
	got := groundableTangles(tangles)
	want := []int{0, 1, 2}
	if len(got) != len(want) {
		t.Fatalf("groundableTangles = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("groundableTangles = %v, want %v", got, want)
		}
	}
}

func TestApplyGroundingVerdictsDropsUngroundedCrossPerson(t *testing.T) {
	tangles := []Tangle{
		ck(KindCollision, "alice", "bob"), // 0: grounded → keep
		ck(KindCollision, "alice", "dao"), // 1: NOT grounded → drop
		ck(KindStaleAssumption, "cleo"),   // 2: self tangle → always keep
	}
	verdicts := map[int]bool{0: true, 1: false}
	out, suppressed := applyGroundingVerdicts(tangles, verdicts)
	if len(out) != 2 {
		t.Fatalf("expected 2 tangles (alice×bob + cleo self), got %d: %+v", len(out), out)
	}
	for _, k := range out {
		if MultiPerson(k.Parties) && SamePerson(k.Parties[1], "dao") {
			t.Fatalf("the ungrounded alice×dao tangle should have been dropped")
		}
	}
	// The dropped tangle is RETURNED as suppressed (for legible abstention), not lost.
	if len(suppressed) != 1 || !SamePerson(suppressed[0].Parties[1], "dao") {
		t.Fatalf("expected the alice×dao tangle in suppressed, got %+v", suppressed)
	}
}

func TestApplyGroundingVerdictsFailsOpen(t *testing.T) {
	// A multi-person tangle with NO returned verdict must be KEPT (protects recall
	// if the verifier garbles or omits it).
	tangles := []Tangle{ck(KindCollision, "alice", "bob")}
	out, suppressed := applyGroundingVerdicts(tangles, map[int]bool{}) // no verdict for index 0
	if len(out) != 1 {
		t.Fatalf("unjudged tangle must survive (fail open), got %d", len(out))
	}
	if len(suppressed) != 0 {
		t.Fatalf("fail-open must not suppress an unjudged tangle, got %+v", suppressed)
	}
}

func TestApplyGroundingVerdictsSelfTanglesNeverChecked(t *testing.T) {
	// A self tangle keeps even with a (mistaken) false verdict against its index.
	tangles := []Tangle{ck(KindStaleAssumption, "alice")}
	out, _ := applyGroundingVerdicts(tangles, map[int]bool{0: false})
	if len(out) != 1 {
		t.Fatalf("a single-author tangle must never be dropped by grounding, got %d", len(out))
	}
}

func TestApplyGroundingVerdictsDoesNotAliasInput(t *testing.T) {
	tangles := []Tangle{
		ck(KindCollision, "alice", "dao"), // will be dropped
		ck(KindCollision, "alice", "bob"), // kept
	}
	_, _ = applyGroundingVerdicts(tangles, map[int]bool{0: false, 1: true})
	// The original slice must be untouched (out used a fresh backing array).
	if len(tangles) != 2 || !SamePerson(tangles[0].Parties[1], "dao") {
		t.Fatalf("input slice was mutated by applyGroundingVerdicts: %+v", tangles)
	}
}
