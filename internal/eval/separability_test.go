package eval

import (
	"testing"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
)

func TestClassifyTangle(t *testing.T) {
	a := map[string]bool{"alice": true, "bob": true}
	b := map[string]bool{"cleo": true, "dao": true}
	cases := []struct {
		parties []string
		want    TangleClass
	}{
		{[]string{"alice", "bob"}, ClassIntra},
		{[]string{"cleo", "dao"}, ClassIntra},
		{[]string{"alice", "dao"}, ClassCrossBoundary},
		{[]string{"Alice ", "cleo"}, ClassCrossBoundary}, // folding
		{[]string{"zane"}, ClassOrphan},
	}
	for _, tc := range cases {
		if got := ClassifyTangle(tc.parties, a, b); got != tc.want {
			t.Errorf("ClassifyTangle(%v) = %d, want %d", tc.parties, got, tc.want)
		}
	}
}

func TestComputeSeparabilitySplitsAndScores(t *testing.T) {
	a := map[string]bool{"alice": true, "bob": true}
	b := map[string]bool{"cleo": true, "dao": true}
	real := ettlemesh.Tangle{Kind: "collision", Parties: []string{"alice", "bob"}, Confidence: 0.9}
	fab := ettlemesh.Tangle{Kind: "duplication", Parties: []string{"alice", "dao"}, Confidence: 0.4}
	// real recurs in all 3 runs; fabricated in only 1.
	runs := [][]ettlemesh.Tangle{
		{real, fab},
		{real},
		{real},
	}
	rep := ComputeSeparability(runs, a, b)
	if len(rep.Real) != 1 || rep.Real[0].Seen != 3 {
		t.Fatalf("real tangle should be seen 3/3: %+v", rep.Real)
	}
	if len(rep.Fabricated) != 1 || rep.Fabricated[0].Seen != 1 {
		t.Fatalf("fabricated tangle should be seen 1/3: %+v", rep.Fabricated)
	}
	if rep.RealFreqMedian != 3 || rep.FabFreqMedian != 1 {
		t.Errorf("freq medians: real %v fab %v, want 3 and 1", rep.RealFreqMedian, rep.FabFreqMedian)
	}
	if rep.RealConfMean <= rep.FabConfMean {
		t.Errorf("real conf %.2f should exceed fab conf %.2f", rep.RealConfMean, rep.FabConfMean)
	}
}

// A tangle found twice in ONE run (pairwise + teamwide pass) counts once toward
// frequency, but both confidences fold into its mean.
func TestComputeSeparabilityDedupesWithinRun(t *testing.T) {
	a := map[string]bool{"alice": true, "bob": true, "cleo": true}
	b := map[string]bool{"dao": true}
	k1 := ettlemesh.Tangle{Kind: "teamwide-divergence", Parties: []string{"alice", "bob", "cleo"}, Confidence: 0.6}
	k2 := ettlemesh.Tangle{Kind: "teamwide-divergence", Parties: []string{"cleo", "bob", "alice"}, Confidence: 0.8}
	rep := ComputeSeparability([][]ettlemesh.Tangle{{k1, k2}}, a, b)
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

func TestProjectVotingCurve(t *testing.T) {
	a := map[string]bool{"alice": true, "bob": true}
	b := map[string]bool{"cleo": true, "dao": true}
	fab := ettlemesh.Tangle{Kind: "collision", Parties: []string{"alice", "dao"}, Confidence: 0.6}
	real := ettlemesh.Tangle{Kind: "collision", Parties: []string{"alice", "bob"}, Confidence: 0.9}
	// 10 runs: fabricated tangle in 2/10 (low freq), real tangle in all 10.
	var runs [][]ettlemesh.Tangle
	for i := 0; i < 10; i++ {
		run := []ettlemesh.Tangle{real}
		if i < 2 {
			run = append(run, fab)
		}
		runs = append(runs, run)
	}
	pts := ProjectVotingCurve(runs, a, b, []int{1, 5}, 2000, 42)
	if len(pts) != 2 {
		t.Fatalf("want 2 points, got %d", len(pts))
	}
	// samples=1 reproduces the raw single-shot fabrication mean (~0.2).
	if pts[0].Samples != 1 || pts[0].FabMean < 0.1 || pts[0].FabMean > 0.3 {
		t.Errorf("samples=1 should reproduce raw mean ~0.2, got %.3f", pts[0].FabMean)
	}
	// samples=5 (majority 3): a 2/10 tangle almost never clears the vote → ~0.
	if pts[1].Samples != 5 || pts[1].Majority != 3 {
		t.Fatalf("samples=5 majority should be 3, got %d", pts[1].Majority)
	}
	// P(>=3 of 5 | p=0.2) ≈ 0.058, so expect a ~3× reduction well under 0.1.
	if pts[1].FabMean >= pts[0].FabMean || pts[1].FabMean > 0.1 {
		t.Errorf("voting should crush a 2/10 fabrication: s1=%.3f s5=%.3f", pts[0].FabMean, pts[1].FabMean)
	}
}
