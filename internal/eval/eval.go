// Package eval is ettle's calibration harness: it scores the detector against a
// committed, synthetic, INSPECTABLE corpus of curated coordination knots.
//
// It exists to answer the sharpest adversarial critique of the project — that
// the "~0.6 precision" number was unfalsifiable: its denominator lived in a
// gitignored sidecar, rested on one private thread, and used a single circular
// human rater. Here the ground truth is committed (testdata/eval/*.json), so a
// stranger can read exactly what counts as a real knot, run the detector, and
// see the precision/recall for themselves. It also runs the honest A/B the
// review demanded: does multi-sample voting actually reduce false positives, or
// only paraphrase variance? — reported with a McNemar significance test so a
// non-significant result (the likely one on a small corpus) is stated as such.
//
// The matcher is lifted from inkling's threadeval scoreMatch: a detected knot
// matches a curated label when they share a party AND either a multi-word
// keyword phrase appears verbatim or their token sets overlap past a Jaccard
// threshold.
package eval

import (
	"encoding/json"
	"math"
	"os"
	"strings"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
)

// Label is a curated expected knot — the inspectable ground truth. Real=true
// means a genuine coordination knot the scenario contains; Real=false marks a
// plausible-but-wrong knot the detector should NOT assert (a planted distractor).
type Label struct {
	ID       string   `json:"id"`
	Parties  []string `json:"parties"`
	About    string   `json:"about"`
	Keywords []string `json:"keywords"`
	Real     bool     `json:"real"`
}

// Corpus is one synthetic scenario: participant inputs (note or .jsonl session
// paths, relative to the repo root) plus the curated expected knots.
type Corpus struct {
	Name     string   `json:"name"`
	Inputs   []string `json:"inputs"`
	Expected []Label  `json:"expected_knots"`
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
	RecallHits  int             // real labels recovered by a FIRM knot
	RecallTotal int             // real labels in the corpus
	TP          int             // FIRM knots matching some real label
	FP          int             // FIRM knots matching no real label (false positives)
	WouldAsk    int             // soft knots (questions, not asserted — not scored against precision)
	Missed      []string        // real label IDs no FIRM knot recovered
	Recovered   map[string]bool // real label ID → recovered by a FIRM knot (for the paired A/B)
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

// Adjudicate scores FIRM knots for precision and recall against the labels; soft
// knots are tracked separately as "would-ask" (they're questions, not assertions,
// so they don't count against precision).
func Adjudicate(firm, soft []ettlemesh.Knot, labels []Label) Score {
	var s Score
	s.WouldAsk = len(soft)
	s.Recovered = map[string]bool{}

	// Recall = coverage: did the detector surface the real knot AT ALL, firm or
	// soft? A soft knot is surfaced as a question, so it still counts as the team
	// being made aware. (Precision, below, is the stricter FIRM-only assertion
	// quality.) Greedy match each real label to a not-yet-used knot.
	all := append(append([]ettlemesh.Knot{}, firm...), soft...)
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

	// Precision: a FIRM knot is a true positive if it matches ANY real label,
	// else a false positive.
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
		} else {
			s.FP++
		}
	}
	return s
}

// ScoreMatch scores how well a detected knot corresponds to a label. A match
// requires sharing at least one party AND either a multi-word keyword phrase
// present verbatim, or at least minTokenHits of the label's curated keyword
// tokens appearing in the detected text. Uses an OVERLAP COUNT, not a Jaccard
// ratio: a knot's explanation is a full sentence, and dividing by its token
// count (Jaccard) wrongly drives a good match below threshold just because the
// explanation is verbose. Counting how many curated discriminators actually show
// up is robust to that. (Adapted from inkling threadeval, which used Jaccard
// over shorter labels.)
func ScoreMatch(l Label, k ettlemesh.Knot) (float64, bool) {
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
	knot := tokens(hay)
	hits := 0
	for t := range label {
		if knot[t] {
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
// which is the honest result on a small corpus. Lifted from inkling rifts/stats.
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
