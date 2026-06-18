package eval

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"strings"
)

// SuperCorpus names two groups of participant inputs asserted to be INDEPENDENT
// of each other (the precondition for the locality law). Knots within a group are
// fine and expected; the test is that joining the groups invents no cross-group
// coordination and loses no within-group coordination.
type SuperCorpus struct {
	Name   string   `json:"name"`
	GroupA []string `json:"groupA"`
	GroupB []string `json:"groupB"`
}

// LoadSuperCorpus reads a superposition corpus JSON file.
func LoadSuperCorpus(path string) (SuperCorpus, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return SuperCorpus{}, err
	}
	var c SuperCorpus
	return c, json.Unmarshal(b, &c)
}

// Superposition is the locality probe — a metamorphic test that needs NO human
// labels. Coordination is supposed to be local to actual dependencies, so for two
// genuinely independent groups A and B the joint horizon must be the union of the
// separate ones:
//
//	f(A ⊎ B)  ==  f(A) ∪ f(B)
//
// We get ground truth for free from that law instead of labeling knots. The
// headline is the CROSS-BOUNDARY count: a knot in the joint run whose parties
// span both groups links two people who were never in the same run when we
// computed A and B alone — so it is *provably fabricated*, and (unlike the
// stability metric) the signal is immune to run-to-run flicker, because no amount
// of stochasticity in an A-only or B-only run can produce a knot mentioning a
// B-only or A-only person.
//
// Two secondary failures are also visible but are CONFOUNDED by flicker (the same
// non-determinism #5 measures), so they are reported as such, not as the headline:
//   - Dropped: an intra-group knot present alone but gone in the joint run —
//     the other group's noise distracted the detector off a real knot.
//   - SpuriousIntra: an intra-group knot in the joint run absent from that
//     group's solo run — the other group's mere presence conjured it.

// SuperpositionResult classifies the joint-run knot keys against the law above.
type SuperpositionResult struct {
	Preserved     []string // intra-group keys in BOTH the solo and joint runs (the law holding)
	CrossBoundary []string // joint keys spanning A and B — fabricated, flicker-PROOF (the headline)
	Dropped       []string // solo keys missing from the joint run — distraction (flicker-confounded)
	SpuriousIntra []string // intra-group joint keys absent from that group's solo run (flicker-confounded)
	Orphan        []string // joint keys with no party in either group (shouldn't happen; surfaces a roster bug)
}

// LocalityScore is the fraction of accounted-for keys that obey the law. 1.0 =
// perfect locality. Cross-boundary fabrications weigh against it most directly,
// but every violation class counts.
func (r SuperpositionResult) LocalityScore() float64 {
	good := len(r.Preserved)
	bad := len(r.CrossBoundary) + len(r.Dropped) + len(r.SpuriousIntra) + len(r.Orphan)
	if good+bad == 0 {
		return 1.0
	}
	return float64(good) / float64(good+bad)
}

// SuperStats aggregates the per-run cross-boundary fabrication counts across K
// joint runs WITH their uncertainty bands — the point being that a single point
// estimate (rate=2/8) is uninterpretable without knowing how wide its interval
// is. Two signals:
//
//   - MeanPerRun (continuous): the expected number of fabricated cross-group knots
//     per joint run. This is the PRIMARY A/B signal because it has lower variance
//     than the binary rate — a run that fabricates 3 links and one that fabricates
//     1 are both "fabricated" to the binary metric but differ 3× here, so the same
//     K runs buy a tighter estimate. CI is the 95% normal interval on the mean.
//   - Rate (binary): the fraction of runs that fabricated >=1 link — interpretable
//     ("how often does ettle invent cross-group coordination?") but coarse. Its CI
//     is the 95% WILSON score interval, which (unlike the normal approximation) is
//     honest at small n and near 0/1 — exactly the regime here.
//
// When two conditions' intervals overlap, the experiment did not distinguish them:
// that is the "underpowered at n=8" verdict, made explicit instead of inferred.
type SuperStats struct {
	Runs                  int
	MeanPerRun            float64 // expected cross-boundary knots per run
	MeanCILow, MeanCIHigh float64 // 95% normal CI on MeanPerRun (== mean when Runs<2)
	Rate                  float64 // fraction of runs with >=1 fabrication
	RateCILow, RateCIHigh float64 // 95% Wilson interval on Rate
}

// ComputeSuperStats aggregates per-run cross-boundary counts (one int per joint
// run = len(SuperpositionResult.CrossBoundary)). Pure; no model call.
func ComputeSuperStats(perRun []int) SuperStats {
	n := len(perRun)
	s := SuperStats{Runs: n}
	if n == 0 {
		return s
	}
	var sum, fab float64
	for _, c := range perRun {
		sum += float64(c)
		if c > 0 {
			fab++
		}
	}
	s.MeanPerRun = sum / float64(n)
	s.Rate = fab / float64(n)

	// 95% normal CI on the continuous mean. Undefined for n<2 (no spread to
	// estimate) → collapse the band onto the point estimate.
	s.MeanCILow, s.MeanCIHigh = s.MeanPerRun, s.MeanPerRun
	if n >= 2 {
		var ss float64
		for _, c := range perRun {
			d := float64(c) - s.MeanPerRun
			ss += d * d
		}
		stderr := math.Sqrt(ss/float64(n-1)) / math.Sqrt(float64(n))
		s.MeanCILow = math.Max(0, s.MeanPerRun-1.96*stderr)
		s.MeanCIHigh = s.MeanPerRun + 1.96*stderr
	}

	s.RateCILow, s.RateCIHigh = wilson(int(fab), n)
	return s
}

// wilson returns the 95% Wilson score interval for k successes in n trials —
// honest at small n and near the 0/1 boundaries where the normal approximation
// produces intervals that escape [0,1] or collapse to a point.
func wilson(k, n int) (lo, hi float64) {
	if n == 0 {
		return 0, 0
	}
	const z = 1.96
	p := float64(k) / float64(n)
	nf := float64(n)
	denom := 1 + z*z/nf
	center := (p + z*z/(2*nf)) / denom
	half := (z / denom) * math.Sqrt(p*(1-p)/nf+z*z/(4*nf*nf))
	return math.Max(0, center-half), math.Min(1, center+half)
}

// partiesOf splits a stability key ("kind\x00a+b") into its lowercased party list.
func partiesOf(key string) []string {
	parts := strings.SplitN(key, "\x00", 2)
	if len(parts) != 2 || parts[1] == "" {
		return nil
	}
	return strings.Split(parts[1], "+")
}

// ComputeSuperposition applies the locality law. groupA/groupB are the lowercased
// person names in each independent group; keysA/keysB/keysAB are the stability-key
// sets (from RunKeys) of the solo-A, solo-B, and joint runs.
func ComputeSuperposition(keysA, keysB, keysAB map[string]bool, groupA, groupB map[string]bool) SuperpositionResult {
	var r SuperpositionResult

	for key := range keysAB {
		inA, inB := false, false
		for _, p := range partiesOf(key) {
			if groupA[p] {
				inA = true
			}
			if groupB[p] {
				inB = true
			}
		}
		switch {
		case inA && inB:
			r.CrossBoundary = append(r.CrossBoundary, key) // the clean, flicker-proof fabrication
		case inA:
			if !keysA[key] {
				r.SpuriousIntra = append(r.SpuriousIntra, key)
			} else {
				r.Preserved = append(r.Preserved, key)
			}
		case inB:
			if !keysB[key] {
				r.SpuriousIntra = append(r.SpuriousIntra, key)
			} else {
				r.Preserved = append(r.Preserved, key)
			}
		default:
			r.Orphan = append(r.Orphan, key) // a party in neither group — roster/identity bug
		}
	}

	// Dropped: a knot the detector found for a group ALONE but lost once the other
	// group's notes were in the room.
	for key := range keysA {
		if !keysAB[key] {
			r.Dropped = append(r.Dropped, key)
		}
	}
	for key := range keysB {
		if !keysAB[key] {
			r.Dropped = append(r.Dropped, key)
		}
	}

	sort.Strings(r.Preserved)
	sort.Strings(r.CrossBoundary)
	sort.Strings(r.Dropped)
	sort.Strings(r.SpuriousIntra)
	sort.Strings(r.Orphan)
	return r
}
