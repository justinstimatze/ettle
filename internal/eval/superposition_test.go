package eval

import "testing"

import "github.com/justinstimatze/ettle/internal/ettlemesh"

func keySet(knots ...ettlemesh.Knot) map[string]bool {
	return RunKeys(knots, nil)
}

func TestSuperpositionPerfectLocality(t *testing.T) {
	groupA := map[string]bool{"alice": true, "bob": true}
	groupB := map[string]bool{"cleo": true, "dao": true}
	ab := keySet(knot(ettlemesh.KindCollision, "alice", "bob")) // A's real knot
	a := keySet(knot(ettlemesh.KindCollision, "alice", "bob"))
	b := keySet() // B has nothing
	abJoint := keySet(knot(ettlemesh.KindCollision, "alice", "bob"))
	_ = ab
	r := ComputeSuperposition(a, b, abJoint, groupA, groupB)
	if r.LocalityScore() != 1.0 {
		t.Fatalf("law holds exactly; want 1.0, got %.2f (%+v)", r.LocalityScore(), r)
	}
	if len(r.Preserved) != 1 || len(r.CrossBoundary) != 0 {
		t.Fatalf("expected 1 preserved, 0 cross-boundary; got %+v", r)
	}
}

func TestSuperpositionCatchesFabricatedCrossBoundary(t *testing.T) {
	groupA := map[string]bool{"alice": true, "bob": true}
	groupB := map[string]bool{"cleo": true, "dao": true}
	a := keySet(knot(ettlemesh.KindCollision, "alice", "bob"))
	b := keySet()
	// Joint run invents a knot linking alice (A) to cleo (B) — impossible to have
	// appeared in either solo run; provably fabricated.
	abJoint := keySet(
		knot(ettlemesh.KindCollision, "alice", "bob"),
		knot(ettlemesh.KindDuplication, "alice", "cleo"),
	)
	r := ComputeSuperposition(a, b, abJoint, groupA, groupB)
	if len(r.CrossBoundary) != 1 {
		t.Fatalf("the alice×cleo knot must be flagged cross-boundary; got %+v", r)
	}
	if r.LocalityScore() != 0.5 { // 1 preserved, 1 violation
		t.Fatalf("want locality 0.50, got %.2f", r.LocalityScore())
	}
}

func TestSuperpositionDroppedAndSpurious(t *testing.T) {
	groupA := map[string]bool{"alice": true, "bob": true}
	groupB := map[string]bool{"cleo": true, "dao": true}
	a := keySet(knot(ettlemesh.KindCollision, "alice", "bob")) // found alone
	b := keySet()
	// Joint: the alice×bob knot VANISHED, and a new intra-A stale-assumption appeared.
	abJoint := keySet(knot(ettlemesh.KindStaleAssumption, "alice"))
	r := ComputeSuperposition(a, b, abJoint, groupA, groupB)
	if len(r.Dropped) != 1 {
		t.Fatalf("alice×bob should be Dropped; got %+v", r)
	}
	if len(r.SpuriousIntra) != 1 {
		t.Fatalf("the new alice stale-assumption should be SpuriousIntra; got %+v", r)
	}
}

func TestSuperpositionOrphanSurfacesRosterBug(t *testing.T) {
	groupA := map[string]bool{"alice": true}
	groupB := map[string]bool{"bob": true}
	abJoint := keySet(knot(ettlemesh.KindStaleAssumption, "ghost")) // nobody's name
	r := ComputeSuperposition(keySet(), keySet(), abJoint, groupA, groupB)
	if len(r.Orphan) != 1 {
		t.Fatalf("a knot about an unknown party should be an Orphan; got %+v", r)
	}
}
