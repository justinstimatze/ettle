// Package eval is ettle's calibration harness: it scores the detector against a
// committed, synthetic, INSPECTABLE corpus of curated coordination tangles.
//
// It exists so the precision claim is falsifiable: rather than a number whose
// denominator lives in a gitignored sidecar resting on one private thread and a
// single rater, the ground truth is committed (testdata/eval/*.json), so a
// stranger can read exactly what counts as a real tangle, run the detector, and
// see the precision/recall for themselves. It also runs an honest A/B: does
// multi-sample voting actually reduce false positives, or only paraphrase
// variance? — reported with a McNemar significance test so a non-significant
// result (the likely one on a small corpus) is stated as such.
//
// The matcher: a detected tangle matches a curated label when they share a party
// AND either a multi-word keyword phrase appears verbatim or at least
// minTokenHits curated keyword tokens appear in the detected text (an overlap
// COUNT, not a Jaccard ratio — see ScoreMatch for why dividing by a verbose
// explanation's length would wrongly sink a good match).
//
// The corpus also carries plausible-but-wrong distractors (Real=false) —
// single-person open questions a miscalibrated detector might wrongly assert as
// cross-person tangles. A FIRM tangle matching a distractor is reported as a named
// trap the detector fell for, more diagnostic than a bare false-positive count.
// The A/B's McNemar test is pooled across corpora, because per-corpus the
// discordant N is always too small to reach significance.
package eval

import (
	"encoding/json"
	"math"
	"os"
	"strings"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
)

// Label is a curated expected tangle — the inspectable ground truth. Real=true
// means a genuine coordination tangle the scenario contains; Real=false marks a
// plausible-but-wrong tangle the detector should NOT assert (a planted distractor).
type Label struct {
	ID       string   `json:"id"`
	Parties  []string `json:"parties"`
	About    string   `json:"about"`
	Keywords []string `json:"keywords"`
	Real     bool     `json:"real"`
}

// Corpus is one synthetic scenario: participant inputs (note or .jsonl session
// paths, relative to the repo root) plus the curated expected tangles.
type Corpus struct {
	Name     string   `json:"name"`
	Inputs   []string `json:"inputs"`
	Expected []Label  `json:"expected_tangles"`
}

// LoadCorpus reads a corpus JSON file.
func LoadCorpus(path string) (Corpus, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Corpus{}, err
	}
	var c Corpus
	return c, json.Unmarshal(b, &c)
}

// Score is the adjudication result for one detection run against a corpus.
type Score struct {
	RecallHits  int             // real labels recovered by a FIRM tangle
	RecallTotal int             // real labels in the corpus
	TP          int             // FIRM tangles matching some real label
	FP          int             // FIRM tangles matching no real label (false positives)
	WouldAsk    int             // soft tangles (questions, not asserted — not scored against precision)
	Missed      []string        // real label IDs no FIRM tangle recovered
	Recovered   map[string]bool // real label ID → recovered by a FIRM tangle (for the paired A/B)
	TrapHits    []string        // distractor (Real=false) IDs a FIRM tangle wrongly asserted — a named trap fallen for
}

func (s Score) Precision() float64 {
	if s.TP+s.FP == 0 {
		return 0
	}
	return float64(s.TP) / float64(s.TP+s.FP)
}

func (s Score) Recall() float64 {
	if s.RecallTotal == 0 {
		return 0
	}
	return float64(s.RecallHits) / float64(s.RecallTotal)
}

// Adjudicate scores FIRM tangles for precision and recall against the labels; soft
// tangles are tracked separately as "would-ask" (they're questions, not assertions,
// so they don't count against precision).
func Adjudicate(firm, soft []ettlemesh.Tangle, labels []Label) Score {
	var s Score
	s.WouldAsk = len(soft)
	s.Recovered = map[string]bool{}

	// Recall = coverage: did the detector surface the real tangle AT ALL, firm or
	// soft? A soft tangle is surfaced as a question, so it still counts as the team
	// being made aware. (Precision, below, is the stricter FIRM-only assertion
	// quality.) Greedy match each real label to a not-yet-used tangle. NOTE: this is
	// a greedy first-best assignment, not an optimal bipartite matching — a label
	// can grab a tangle a later label would have scored higher on, so the reported
	// recall is a conservative LOWER BOUND on the best achievable matching, never
	// an over-count.
	all := append(append([]ettlemesh.Tangle{}, firm...), soft...)
	used := make([]bool, len(all))
	for _, l := range labels {
		if !l.Real {
			continue
		}
		s.RecallTotal++
		best, bestScore := -1, 0.0
		for i, k := range all {
			if used[i] {
				continue
			}
			if sc, ok := ScoreMatch(l, k); ok && sc > bestScore {
				best, bestScore = i, sc
			}
		}
		if best >= 0 {
			used[best] = true
			s.RecallHits++
			s.Recovered[l.ID] = true
		} else {
			s.Missed = append(s.Missed, l.ID)
		}
	}

	// Precision: a FIRM tangle is a true positive if it matches ANY real label,
	// else a false positive. A false positive that matches a planted distractor
	// (Real=false) is recorded as a named trap the detector fell for — more
	// diagnostic than a bare FP count, and the reason the corpus carries
	// plausible-but-wrong tangles at all.
	for _, k := range firm {
		tp := false
		for _, l := range labels {
			if !l.Real {
				continue
			}
			if _, ok := ScoreMatch(l, k); ok {
				tp = true
				break
			}
		}
		if tp {
			s.TP++
			continue
		}
		s.FP++
		for _, l := range labels {
			if l.Real {
				continue
			}
			if _, ok := ScoreMatch(l, k); ok {
				s.TrapHits = append(s.TrapHits, l.ID)
				break
			}
		}
	}
	return s
}

// ScoreMatch scores how well a detected tangle corresponds to a label. A match
// requires sharing at least one party AND either a multi-word keyword phrase
// present verbatim, or at least minTokenHits of the label's curated keyword
// tokens appearing in the detected text. Uses an OVERLAP COUNT, not a Jaccard
// ratio: a tangle's explanation is a full sentence, and dividing by its token
// count (Jaccard) wrongly drives a good match below threshold just because the
// explanation is verbose. Counting how many curated discriminators actually show
// up is robust to that (a Jaccard ratio over the shorter labels would not be).
func ScoreMatch(l Label, k ettlemesh.Tangle) (float64, bool) {
	if !sharesParty(l.Parties, k.Parties) {
		return 0, false
	}
	hay := strings.ToLower(k.About + " " + k.Explanation)
	for _, kw := range l.Keywords { // a verbatim multi-word phrase is a strong match
		if len(strings.Fields(kw)) >= 2 && strings.Contains(hay, strings.ToLower(kw)) {
			return 100, true
		}
	}
	label := tokens(l.About + " " + strings.Join(l.Keywords, " "))
	tangle := tokens(hay)
	hits := 0
	for t := range label {
		if tangle[t] {
			hits++
		}
	}
	if hits < minTokenHits {
		return 0, false
	}
	return float64(hits), true
}

const minTokenHits = 2

// McNemarTwoTailed returns the two-tailed p-value for the discordant pair counts
// of two binary classifiers on the same items (b: A right & B wrong; c: A wrong
// & B right). Continuity-corrected chi-square via the erfc tail. Returns 1.0
// (no evidence of a difference) when the discordant N is too small to test —
// which is the honest result on a small corpus.
func McNemarTwoTailed(b, c int) float64 {
	n := b + c
	if n == 0 {
		return 1.0
	}
	diff := math.Abs(float64(b - c))
	cc := diff - 1.0
	if cc < 0 {
		cc = 0
	}
	stat := (cc * cc) / float64(n)
	if n < 6 {
		return 1.0 // approximation unreliable at small N; don't claim significance
	}
	return math.Erfc(math.Sqrt(stat) / math.Sqrt2)
}

// CalibBin is one confidence bucket of the ECE readout: how many tangles fell in
// it, their mean stated confidence, and the fraction that actually matched a real
// label. A well-calibrated detector has MeanConf ≈ Accuracy in every populated
// bin (when it says 0.8 it is right ~80% of the time).
type CalibBin struct {
	Lo, Hi   float64
	N        int
	MeanConf float64
	Accuracy float64
}

// Calibration bins ALL tangles (firm ∪ soft) by stated confidence and returns the
// per-bin mean-confidence-vs-accuracy gap, plus the Expected Calibration Error
// (the count-weighted mean |conf − accuracy| across populated bins). A tangle is
// "correct" when it matches some real label via ScoreMatch. ECE near 0 means the
// confidence numbers mean what they say; a large gap means the detector's
// confidence is decorative. On this tiny corpus the number is DESCRIPTIVE, not
// significant — same honesty caveat as the McNemar test. nbins ≤ 0 defaults to 5.
func Calibration(tangles []ettlemesh.Tangle, labels []Label, nbins int) ([]CalibBin, float64) {
	if nbins <= 0 {
		nbins = 5
	}
	type acc struct {
		n       int
		sumConf float64
		correct int
	}
	buckets := make([]acc, nbins)
	for _, k := range tangles {
		c := k.Confidence
		if c < 0 {
			c = 0
		} else if c > 1 {
			c = 1
		}
		bi := int(c * float64(nbins))
		if bi >= nbins {
			bi = nbins - 1
		}
		buckets[bi].n++
		buckets[bi].sumConf += c
		if matchesAnyReal(k, labels) {
			buckets[bi].correct++
		}
	}
	bins := make([]CalibBin, 0, nbins)
	var ece float64
	total := len(tangles)
	for i := 0; i < nbins; i++ {
		b := buckets[i]
		bin := CalibBin{Lo: float64(i) / float64(nbins), Hi: float64(i+1) / float64(nbins), N: b.n}
		if b.n > 0 {
			bin.MeanConf = b.sumConf / float64(b.n)
			bin.Accuracy = float64(b.correct) / float64(b.n)
			if total > 0 {
				ece += (float64(b.n) / float64(total)) * math.Abs(bin.MeanConf-bin.Accuracy)
			}
		}
		bins = append(bins, bin)
	}
	return bins, ece
}

// matchesAnyReal reports whether a tangle matches at least one Real=true label.
func matchesAnyReal(k ettlemesh.Tangle, labels []Label) bool {
	for _, l := range labels {
		if !l.Real {
			continue
		}
		if _, ok := ScoreMatch(l, k); ok {
			return true
		}
	}
	return false
}

func sharesParty(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if ettlemesh.SamePerson(x, y) {
				return true
			}
		}
	}
	return false
}

func tokens(s string) map[string]bool {
	out := map[string]bool{}
	for _, f := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		if len(f) >= 3 && !stop[f] {
			out[f] = true
		}
	}
	return out
}

var stop = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "that": true, "this": true,
	"from": true, "into": true, "over": true, "whose": true, "about": true, "vs": true,
	"their": true, "them": true, "they": true, "between": true, "will": true, "who": true,
}
