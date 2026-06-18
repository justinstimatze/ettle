package eval

import "testing"

import "github.com/justinstimatze/ettle/internal/ettlemesh"

func knot(kind string, parties ...string) ettlemesh.Knot {
	return ettlemesh.Knot{Kind: kind, Parties: parties, Confidence: 1.0}
}

func TestKnotKeyOrderIndependentAndFolded(t *testing.T) {
	a := KnotKey(knot(ettlemesh.KindCollision, "Alice", "Bob"))
	b := KnotKey(knot(ettlemesh.KindCollision, "bob", " alice "))
	if a != b {
		t.Fatalf("keys should match regardless of order/case/space: %q vs %q", a, b)
	}
	if KnotKey(knot(ettlemesh.KindDuplication, "alice", "bob")) == a {
		t.Fatal("different kind must produce a different key")
	}
}

func TestComputeStabilityPerfectAgreement(t *testing.T) {
	run := RunKeys([]ettlemesh.Knot{knot(ettlemesh.KindCollision, "a", "b")}, nil)
	res := ComputeStability([]map[string]bool{run, run, run})
	if res.MeanJaccard != 1.0 || res.MinJaccard != 1.0 {
		t.Fatalf("identical runs should be perfectly stable, got mean=%.2f min=%.2f", res.MeanJaccard, res.MinJaccard)
	}
	if len(res.Flickering()) != 0 {
		t.Fatalf("no flicker expected, got %v", res.Flickering())
	}
	if len(res.Stable()) != 1 {
		t.Fatalf("the one knot should be stable across all runs, got %v", res.Stable())
	}
}

func TestComputeStabilityEmptyRunsAgree(t *testing.T) {
	// Surfacing nothing on every run IS consistent — the independent-work case.
	res := ComputeStability([]map[string]bool{{}, {}, {}})
	if res.MeanJaccard != 1.0 {
		t.Fatalf("two empty runs must agree (Jaccard 1.0), got %.2f", res.MeanJaccard)
	}
}

func TestComputeStabilityFlickerDetected(t *testing.T) {
	stable := KnotKey(knot(ettlemesh.KindCollision, "a", "b"))
	flick := KnotKey(knot(ettlemesh.KindDuplication, "a", "c"))
	runs := []map[string]bool{
		{stable: true, flick: true}, // run 1: both
		{stable: true},              // run 2: only the stable one
		{stable: true},              // run 3: only the stable one
	}
	res := ComputeStability(runs)
	if got := res.Flickering(); len(got) != 1 || got[0] != flick {
		t.Fatalf("flicker should be exactly the duplication knot, got %v", got)
	}
	if got := res.Stable(); len(got) != 1 || got[0] != stable {
		t.Fatalf("stable should be exactly the collision knot, got %v", got)
	}
	// 3 pairs: {1,2}=1/2, {1,3}=1/2, {2,3}=1/1 → mean = (0.5+0.5+1)/3.
	if res.MinJaccard != 0.5 {
		t.Fatalf("worst pair should be 0.5, got %.2f", res.MinJaccard)
	}
	want := (0.5 + 0.5 + 1.0) / 3.0
	if diff := res.MeanJaccard - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("mean jaccard = %.4f, want %.4f", res.MeanJaccard, want)
	}
}

func TestComputeStabilitySingleRunNoPairs(t *testing.T) {
	res := ComputeStability([]map[string]bool{RunKeys([]ettlemesh.Knot{knot(ettlemesh.KindCollision, "a", "b")}, nil)})
	if res.MeanJaccard != 1.0 {
		t.Fatalf("a single run has no pairs to disagree; report 1.0, got %.2f", res.MeanJaccard)
	}
}
