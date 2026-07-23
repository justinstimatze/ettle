package eval

import (
	"sort"
	"strings"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
)

// Stability is the run-to-run determinism probe (simulation #5). A coordination
// tool people check every morning cannot surface a DIFFERENT horizon each run
// from identical input — that flicker is itself a trust failure, independent of
// whether any single run is correct. So we run the same corpus K times and ask:
// how much does the set of surfaced tangles agree across runs?
//
// The unit of identity is the COORDINATION PROBLEM, not its wording. TangleKey is
// (kind, sorted parties) — deliberately NOT the About/Explanation text, because
// the distiller rewords prose run-to-run (the known subject-reword limit), and
// scoring stability on wording would conflate "the model phrased it differently"
// with "the model found a different problem." We care about the latter.
//
// Coarseness runs in the SAFE direction: two genuinely different problems between
// the same parties with the same kind collapse to one key, so the metric can only
// ever OVERSTATE agreement. A reported Jaccard is therefore a conservative upper
// bound on stability — the true reproducibility is at least that bad, never better.

// TangleKey is the stability identity of a tangle: its kind plus its party set,
// order-independent and case-folded. Two tangles with the same key are "the same
// coordination problem surfaced again," regardless of how either was worded.
func TangleKey(k ettlemesh.Tangle) string {
	parties := make([]string, len(k.Parties))
	for i, p := range k.Parties {
		parties[i] = strings.ToLower(strings.TrimSpace(p))
	}
	sort.Strings(parties)
	return k.Kind + "\x00" + strings.Join(parties, "+")
}

// RunKeys collapses one run's tangles to the set of distinct stability keys it
// surfaced. firm and soft are pooled: a tangle surfaced as a question still shapes
// the horizon, so it counts toward what the run "said."
func RunKeys(firm, soft []ettlemesh.Tangle) map[string]bool {
	set := map[string]bool{}
	for _, k := range firm {
		set[TangleKey(k)] = true
	}
	for _, k := range soft {
		set[TangleKey(k)] = true
	}
	return set
}

// StabilityResult is the report over K runs of the same corpus.
type StabilityResult struct {
	Runs        int            // K
	MeanJaccard float64        // mean pairwise Jaccard over all run pairs (1.0 = perfectly stable)
	MinJaccard  float64        // worst pair — the most two runs disagreed
	Frequency   map[string]int // stability key → how many of the K runs surfaced it
}

// Stable returns the keys present in EVERY run (the dependable core of the horizon).
func (r StabilityResult) Stable() []string { return r.keysWithFreq(r.Runs) }

// Flickering returns keys present in some runs but not all — the tangles a user
// would see appear and vanish across mornings. This is the headline failure: a
// non-empty list means the horizon is not reproducible.
func (r StabilityResult) Flickering() []string {
	var out []string
	for k, n := range r.Frequency {
		if n > 0 && n < r.Runs {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func (r StabilityResult) keysWithFreq(want int) []string {
	var out []string
	for k, n := range r.Frequency {
		if n == want {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// ComputeStability scores K runs (each a set of stability keys) for agreement.
// Jaccard of two runs = |intersection| / |union|; two empty runs are defined as
// perfectly agreeing (Jaccard 1.0) — surfacing nothing twice IS consistent, which
// is exactly the correct behavior on an independent-work corpus. With fewer than
// two runs there are no pairs to compare, so MeanJaccard/MinJaccard report 1.0.
func ComputeStability(runs []map[string]bool) StabilityResult {
	res := StabilityResult{Runs: len(runs), MeanJaccard: 1.0, MinJaccard: 1.0, Frequency: map[string]int{}}
	for _, run := range runs {
		for k := range run {
			res.Frequency[k]++
		}
	}
	if len(runs) < 2 {
		return res
	}
	var sum, worst float64 = 0, 1.0
	pairs := 0
	for i := 0; i < len(runs); i++ {
		for j := i + 1; j < len(runs); j++ {
			jac := jaccard(runs[i], runs[j])
			sum += jac
			if jac < worst {
				worst = jac
			}
			pairs++
		}
	}
	res.MeanJaccard = sum / float64(pairs)
	res.MinJaccard = worst
	return res
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	inter := 0
	for k := range a {
		if b[k] {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 1.0
	}
	return float64(inter) / float64(union)
}
