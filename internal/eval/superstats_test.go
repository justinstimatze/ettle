package eval

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestComputeSuperStatsEmpty(t *testing.T) {
	s := ComputeSuperStats(nil)
	if s.Runs != 0 || s.MeanPerRun != 0 || s.Rate != 0 {
		t.Errorf("empty input must be all-zero: %+v", s)
	}
}

func TestComputeSuperStatsAllZeroTightBand(t *testing.T) {
	s := ComputeSuperStats([]int{0, 0, 0, 0})
	if s.MeanPerRun != 0 || s.Rate != 0 {
		t.Errorf("no fabrication → mean and rate 0, got %+v", s)
	}
	// Zero variance → CI collapses on the point.
	if !approx(s.MeanCILow, 0) || !approx(s.MeanCIHigh, 0) {
		t.Errorf("zero-variance mean CI must be [0,0], got [%f,%f]", s.MeanCILow, s.MeanCIHigh)
	}
	// Wilson at k=0 stays in [0,1] and is non-trivial (upper bound > 0). The lower
	// bound is 0 up to float noise (center-half via sqrt isn't bit-exact).
	if s.RateCILow > 1e-9 || s.RateCIHigh <= 0 || s.RateCIHigh > 1 {
		t.Errorf("wilson(0,4) band looks wrong: [%f,%f]", s.RateCILow, s.RateCIHigh)
	}
}

func TestComputeSuperStatsMeanAndRate(t *testing.T) {
	// counts: 0,1,3,0 → mean 1.0, fabricated in 2/4 runs = 0.5
	s := ComputeSuperStats([]int{0, 1, 3, 0})
	if !approx(s.MeanPerRun, 1.0) {
		t.Errorf("mean = %f, want 1.0", s.MeanPerRun)
	}
	if !approx(s.Rate, 0.5) {
		t.Errorf("rate = %f, want 0.5", s.Rate)
	}
	if s.MeanCIHigh <= s.MeanCILow {
		t.Errorf("non-degenerate spread must give a real band: [%f,%f]", s.MeanCILow, s.MeanCIHigh)
	}
	if s.MeanCILow < 0 {
		t.Errorf("mean CI low must clamp at 0, got %f", s.MeanCILow)
	}
}

// The continuous mean separates two conditions the binary rate conflates: both
// fabricate on every run (rate=1.0), but one fabricates 3× as many links.
func TestMeanDistinguishesWhatRateConflates(t *testing.T) {
	light := ComputeSuperStats([]int{1, 1, 1, 1})
	heavy := ComputeSuperStats([]int{3, 3, 3, 3})
	if !approx(light.Rate, 1.0) || !approx(heavy.Rate, 1.0) {
		t.Fatalf("both should be rate 1.0: %f vs %f", light.Rate, heavy.Rate)
	}
	if !(heavy.MeanPerRun > light.MeanPerRun) {
		t.Errorf("mean must distinguish them: light %f vs heavy %f", light.MeanPerRun, heavy.MeanPerRun)
	}
}

func TestWilsonBounds(t *testing.T) {
	for _, tc := range []struct{ k, n int }{{0, 8}, {8, 8}, {4, 8}, {1, 100}} {
		lo, hi := wilson(tc.k, tc.n)
		if lo < 0 || hi > 1 || lo > hi {
			t.Errorf("wilson(%d,%d) out of bounds: [%f,%f]", tc.k, tc.n, lo, hi)
		}
	}
}
