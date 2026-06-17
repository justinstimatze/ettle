package eval

import (
	"testing"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
)

func TestScoreMatch(t *testing.T) {
	l := Label{Parties: []string{"alice", "bob"}, About: "GetUser breaking change", Keywords: []string{"getuser", "signature change", "rename"}, Real: true}
	cases := []struct {
		name string
		k    ettlemesh.Knot
		want bool
	}{
		{"party + phrase verbatim", ettlemesh.Knot{Parties: []string{"bob", "alice"}, About: "the rename", Explanation: "alice is doing a signature change to GetUser"}, true},
		{"party + token overlap", ettlemesh.Knot{Parties: []string{"alice", "carol"}, About: "GetUser rename collision", Explanation: "breaking"}, true},
		{"no shared party", ettlemesh.Knot{Parties: []string{"carol", "dave"}, About: "GetUser rename", Explanation: "breaking signature"}, false},
		{"party but unrelated subject", ettlemesh.Knot{Parties: []string{"alice"}, About: "cache duplication", Explanation: "two caches"}, false},
	}
	for _, c := range cases {
		_, ok := ScoreMatch(l, c.k)
		if ok != c.want {
			t.Errorf("%s: ScoreMatch ok=%v, want %v", c.name, ok, c.want)
		}
	}
}

func TestAdjudicate(t *testing.T) {
	labels := []Label{
		{ID: "K1", Parties: []string{"alice", "bob"}, About: "GetUser breaking change", Keywords: []string{"getuser", "rename"}, Real: true},
		{ID: "K2", Parties: []string{"alice", "carol"}, About: "duplicate cache", Keywords: []string{"cache", "duplicate"}, Real: true},
	}
	firm := []ettlemesh.Knot{
		{Parties: []string{"alice", "bob"}, About: "GetUser rename", Explanation: "breaking", Confidence: 0.9},    // matches K1 (TP)
		{Parties: []string{"alice", "dave"}, About: "totally unrelated thing", Explanation: "x", Confidence: 0.8}, // matches nothing (FP)
	}
	soft := []ettlemesh.Knot{{Parties: []string{"alice"}, About: "an inferred worry", Confidence: 0.4}}
	s := Adjudicate(firm, soft, labels)
	if s.TP != 1 || s.FP != 1 {
		t.Errorf("TP/FP = %d/%d, want 1/1", s.TP, s.FP)
	}
	if s.Precision() != 0.5 {
		t.Errorf("precision = %v, want 0.5", s.Precision())
	}
	if s.RecallHits != 1 || s.RecallTotal != 2 {
		t.Errorf("recall = %d/%d, want 1/2 (K2 missed)", s.RecallHits, s.RecallTotal)
	}
	if !s.Recovered["K1"] || s.Recovered["K2"] {
		t.Errorf("recovered = %v, want K1 only", s.Recovered)
	}
	if s.WouldAsk != 1 {
		t.Errorf("would-ask = %d, want 1", s.WouldAsk)
	}
}

func TestAdjudicateTrap(t *testing.T) {
	labels := []Label{
		{ID: "K1", Parties: []string{"alice", "bob"}, About: "GetUser breaking change", Keywords: []string{"getuser", "rename"}, Real: true},
		// a planted distractor: bob's own open question, not a cross-person knot.
		{ID: "D1", Parties: []string{"bob"}, About: "payment provider choice", Keywords: []string{"provider", "payment"}, Real: false},
	}
	// a firm knot that wrongly asserts the distractor as a cross-person knot.
	firm := []ettlemesh.Knot{
		{Parties: []string{"bob", "alice"}, About: "payment provider decision", Explanation: "who owns the provider choice", Confidence: 0.9},
	}
	s := Adjudicate(firm, nil, labels)
	if s.TP != 0 || s.FP != 1 {
		t.Fatalf("TP/FP = %d/%d, want 0/1", s.TP, s.FP)
	}
	if len(s.TrapHits) != 1 || s.TrapHits[0] != "D1" {
		t.Errorf("TrapHits = %v, want [D1] (the distractor the firm knot fell for)", s.TrapHits)
	}
	if s.RecallTotal != 1 {
		t.Errorf("RecallTotal = %d, want 1 (distractor must NOT count toward recall)", s.RecallTotal)
	}
}

func TestCalibration(t *testing.T) {
	labels := []Label{
		{ID: "K1", Parties: []string{"alice", "bob"}, About: "GetUser breaking change", Keywords: []string{"getuser", "rename"}, Real: true},
	}
	// Two high-confidence knots: one correct (K1), one wrong. A perfectly honest
	// detector at 1.0 would be right 100% of the time; here it's right 50%, so the
	// top bin shows a 0.5 gap and ECE (single populated bin) is ~0.5.
	knots := []ettlemesh.Knot{
		{Parties: []string{"alice", "bob"}, About: "GetUser rename", Explanation: "breaking", Confidence: 1.0},
		{Parties: []string{"alice", "dave"}, About: "unrelated", Explanation: "x", Confidence: 0.9},
	}
	bins, ece := Calibration(knots, labels, 5)
	if len(bins) != 5 {
		t.Fatalf("want 5 bins, got %d", len(bins))
	}
	top := bins[4] // [0.8,1.0]
	if top.N != 2 {
		t.Errorf("top bin N = %d, want 2", top.N)
	}
	if top.Accuracy != 0.5 {
		t.Errorf("top bin accuracy = %v, want 0.5", top.Accuracy)
	}
	if ece < 0.4 || ece > 0.6 {
		t.Errorf("ECE = %v, want ~0.5", ece)
	}
	// No knots → ECE 0, all bins empty (no divide-by-zero).
	if _, e := Calibration(nil, labels, 5); e != 0 {
		t.Errorf("empty ECE = %v, want 0", e)
	}
}

func TestMcNemar(t *testing.T) {
	// no discordance → no evidence of difference.
	if p := McNemarTwoTailed(0, 0); p != 1.0 {
		t.Errorf("McNemar(0,0) = %v, want 1.0", p)
	}
	// small discordant N → unreliable, return 1.0 (no false claim of significance).
	if p := McNemarTwoTailed(2, 1); p != 1.0 {
		t.Errorf("McNemar(2,1) = %v, want 1.0 (N<6 guard)", p)
	}
	// strongly lopsided, enough N → should be significant (p < 0.05).
	if p := McNemarTwoTailed(12, 1); p >= 0.05 {
		t.Errorf("McNemar(12,1) = %v, want < 0.05", p)
	}
}
