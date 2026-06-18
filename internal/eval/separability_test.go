package eval

import (
	"testing"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
)

func TestClassifyKnot(t *testing.T) {
	a := map[string]bool{"alice": true, "bob": true}
	b := map[string]bool{"cleo": true, "dao": true}
	cases := []struct {
		parties []string
		want    KnotClass
	}{
		{[]string{"alice", "bob"}, ClassIntra},
		{[]string{"cleo", "dao"}, ClassIntra},
		{[]string{"alice", "dao"}, ClassCrossBoundary},
		{[]string{"Alice ", "cleo"}, ClassCrossBoundary}, // folding
		{[]string{"zane"}, ClassOrphan},
	}
	for _, tc := range cases {
		if got := ClassifyKnot(tc.parties, a, b); got != tc.want {
			t.Errorf("ClassifyKnot(%v) = %d, want %d", tc.parties, got, tc.want)
		}
	}
}

func TestComputeSeparabilitySplitsAndScores(t *testing.T) {
	a := map[string]bool{"alice": true, "bob": true}
	b := map[string]bool{"cleo": true, "dao": true}
	real := ettlemesh.Knot{Kind: "collision", Parties: []string{"alice", "bob"}, Confidence: 0.9}
	fab := ettlemesh.Knot{Kind: "duplication", Parties: []string{"alice", "dao"}, Confidence: 0.4}
	// real recurs in all 3 runs; fabricated in only 1.
	runs := [][]ettlemesh.Knot{
		{real, fab},
		{real},
		{real},
	}
	rep := ComputeSeparability(runs, a, b)
	if len(rep.Real) != 1 || rep.Real[0].Seen != 3 {
		t.Fatalf("real knot should be seen 3/3: %+v", rep.Real)
	}
	if len(rep.Fabricated) != 1 || rep.Fabricated[0].Seen != 1 {
		t.Fatalf("fabricated knot should be seen 1/3: %+v", rep.Fabricated)
	}
	if rep.RealFreqMedian != 3 || rep.FabFreqMedian != 1 {
		t.Errorf("freq medians: real %v fab %v, want 3 and 1", rep.RealFreqMedian, rep.FabFreqMedian)
	}
	if rep.RealConfMean <= rep.FabConfMean {
		t.Errorf("real conf %.2f should exceed fab conf %.2f", rep.RealConfMean, rep.FabConfMean)
	}
}

// A knot found twice in ONE run (pairwise + teamwide pass) counts once toward
// frequency, but both confidences fold into its mean.
func TestComputeSeparabilityDedupesWithinRun(t *testing.T) {
	a := map[string]bool{"alice": true, "bob": true, "cleo": true}
	b := map[string]bool{"dao": true}
	k1 := ettlemesh.Knot{Kind: "teamwide-divergence", Parties: []string{"alice", "bob", "cleo"}, Confidence: 0.6}
	k2 := ettlemesh.Knot{Kind: "teamwide-divergence", Parties: []string{"cleo", "bob", "alice"}, Confidence: 0.8}
	rep := ComputeSeparability([][]ettlemesh.Knot{{k1, k2}}, a, b)
	if len(rep.Real) != 1 {
		t.Fatalf("party-order variants are the same key: %+v", rep.Real)
	}
	if rep.Real[0].Seen != 1 {
		t.Errorf("two finds in one run = frequency 1, got %d", rep.Real[0].Seen)
	}
	if rep.Real[0].MeanConf != 0.7 {
		t.Errorf("mean conf should fold both (0.6,0.8)=0.7, got %.2f", rep.Real[0].MeanConf)
	}
}
