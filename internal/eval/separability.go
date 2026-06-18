package eval

import (
	"sort"
	"strings"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
)

// Separability is the diagnostic that decides which fabrication FIX is worth
// building. Three failed in-call attempts (grounding verifier, prompt-hardening,
// party-count quorum) established that cross-group fabrication is a model BIAS,
// not a prompt or capability gap. Before building either a calibration loop or an
// upstream-grounding pass, the question is: do fabricated knots LOOK DIFFERENT
// from real ones on any signal we already have? If yes, there is a threshold to
// learn and the cheap fixes (voting, a confidence gate) apply. If no, the atom
// representation is too thin and grounding must move upstream to distill time.
//
// We get both signals from ONE batch of K single-shot joint runs over a
// superposition corpus — no extra model calls:
//   - FREQUENCY: how many of K runs each knot recurred in. This SIMULATES majority
//     voting offline: a knot seen in f/K runs survives a samples=S majority vote
//     iff f/K exceeds 1/2 (in expectation). If fabricated knots are low-frequency
//     and real ones high, the voting we ALREADY have (ReconcileVoted) is the fix.
//   - CONFIDENCE: the model's self-reported confidence per knot. If fabricated
//     knots are systematically lower-confidence, an abstention threshold works.
//
// A knot is REAL if its parties lie within ONE group (an intra-group knot, the
// only kind a solo run could also produce) and FABRICATED if they span both
// groups (provably invented — the people were never in a run together). Orphans
// (a party in neither group) indicate a roster bug and are reported separately.

// KnotClass labels a joint-run knot by where its parties fall relative to the two
// independent groups.
type KnotClass int

const (
	ClassIntra         KnotClass = iota // all parties within one group → REAL
	ClassCrossBoundary                  // parties span both groups → FABRICATED
	ClassOrphan                         // a party in neither group → roster/identity bug
)

// ClassifyKnot decides a knot's class from its parties and the two group rosters
// (both lowercased name sets, matching KnotKey's folding).
func ClassifyKnot(parties []string, groupA, groupB map[string]bool) KnotClass {
	inA, inB, known := false, false, false
	for _, p := range parties {
		lp := strings.ToLower(strings.TrimSpace(p))
		if groupA[lp] {
			inA, known = true, true
		}
		if groupB[lp] {
			inB, known = true, true
		}
	}
	switch {
	case inA && inB:
		return ClassCrossBoundary
	case !known:
		return ClassOrphan
	default:
		return ClassIntra
	}
}

// KnotStat is one distinct knot's behavior across the K runs.
type KnotStat struct {
	Key      string
	Kind     string
	Parties  []string
	Seen     int     // runs it appeared in
	Runs     int     // K
	MeanConf float64 // mean model confidence across the runs it appeared in
}

// SeparabilityReport contrasts fabricated vs real knots on frequency and
// confidence — the data that picks the fork.
type SeparabilityReport struct {
	Runs       int
	Fabricated []KnotStat // cross-boundary, frequency-desc
	Real       []KnotStat // intra-group, frequency-desc
	Orphan     []KnotStat

	// Separation summaries (medians/means over the distinct knots in each class).
	RealFreqMedian float64
	FabFreqMedian  float64
	RealConfMean   float64
	FabConfMean    float64
}

// ComputeSeparability aggregates per-run joint knots into the report. runs[i] is
// every knot (firm+soft) surfaced by joint run i.
func ComputeSeparability(runs [][]ettlemesh.Knot, groupA, groupB map[string]bool) SeparabilityReport {
	type acc struct {
		kind    string
		parties []string
		class   KnotClass
		seen    int     // distinct runs it appeared in (the frequency for voting)
		occ     int     // total emitted instances (a run can emit it twice: pairwise+teamwide)
		confSum float64 // summed over occ
	}
	byKey := map[string]*acc{}
	for _, run := range runs {
		// Dedupe within a run: a knot found by both the pairwise and team-wide pass
		// in one run still counts once toward its frequency, but every instance's
		// confidence folds into the mean.
		seenThisRun := map[string]bool{}
		for _, k := range run {
			key := KnotKey(k)
			a := byKey[key]
			if a == nil {
				a = &acc{kind: k.Kind, parties: k.Parties, class: ClassifyKnot(k.Parties, groupA, groupB)}
				byKey[key] = a
			}
			if !seenThisRun[key] {
				a.seen++
				seenThisRun[key] = true
			}
			a.occ++
			a.confSum += k.Confidence
		}
	}

	rep := SeparabilityReport{Runs: len(runs)}
	for key, a := range byKey {
		mean := 0.0
		if a.occ > 0 {
			mean = a.confSum / float64(a.occ)
		}
		st := KnotStat{Key: key, Kind: a.kind, Parties: a.parties, Seen: a.seen, Runs: len(runs), MeanConf: mean}
		switch a.class {
		case ClassCrossBoundary:
			rep.Fabricated = append(rep.Fabricated, st)
		case ClassOrphan:
			rep.Orphan = append(rep.Orphan, st)
		default:
			rep.Real = append(rep.Real, st)
		}
	}
	sortByFreq(rep.Fabricated)
	sortByFreq(rep.Real)
	sortByFreq(rep.Orphan)

	rep.RealFreqMedian = medianSeen(rep.Real)
	rep.FabFreqMedian = medianSeen(rep.Fabricated)
	rep.RealConfMean = meanConf(rep.Real)
	rep.FabConfMean = meanConf(rep.Fabricated)
	return rep
}

func sortByFreq(s []KnotStat) {
	sort.Slice(s, func(i, j int) bool {
		if s[i].Seen != s[j].Seen {
			return s[i].Seen > s[j].Seen
		}
		return s[i].Key < s[j].Key
	})
}

func medianSeen(s []KnotStat) float64 {
	if len(s) == 0 {
		return 0
	}
	v := make([]int, len(s))
	for i, k := range s {
		v[i] = k.Seen
	}
	sort.Ints(v)
	n := len(v)
	if n%2 == 1 {
		return float64(v[n/2])
	}
	return float64(v[n/2-1]+v[n/2]) / 2
}

func meanConf(s []KnotStat) float64 {
	if len(s) == 0 {
		return 0
	}
	var sum float64
	for _, k := range s {
		sum += k.MeanConf
	}
	return sum / float64(len(s))
}
