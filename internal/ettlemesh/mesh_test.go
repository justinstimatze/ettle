package ettlemesh

import "testing"

func TestParseConf(t *testing.T) {
	cases := []struct {
		in     string
		want   float64
		wantOK bool
	}{
		{"high", 0.9, true},
		{"medium", 0.5, true},
		{"low", 0.3, true},
		{" HIGH ", 0.9, true},
		{"- high", 0.9, true},
		{"0.7", 0.7, true},
		{"1.0", 1.0, true},
		{"0", 0, true},
		{"1.5", 0, false}, // out of range high
		{"banana", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := parseConf(c.in)
		if ok != c.wantOK || (ok && got != c.want) {
			t.Errorf("parseConf(%q) = (%v, %v), want (%v, %v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}

func TestParseKnots(t *testing.T) {
	atoms := []Atom{
		{From: "alice", Confidence: 1.0},
		{From: "bob", Confidence: 0.4, Inferred: true}, // an inferred atom drags a knot soft
	}
	out := `collision | alice, bob | the rename | they collide on GetUser | 0.9
duplication | alice, carol | caches | both build a cache | high
NONE
not-a-kind | alice, bob | x | y | 0.9
ragged line with no pipes
stale-assumption | alice, bob | window | conflict | `

	knots := parseKnots(out, atoms, pairwiseKinds)
	if len(knots) != 3 {
		t.Fatalf("got %d knots, want 3 (collision, duplication, stale-assumption); disallowed+ragged dropped", len(knots))
	}
	if knots[0].Kind != KindCollision || knots[0].About != "the rename" || knots[0].Confidence != 0.9 {
		t.Errorf("knot[0] = %+v", knots[0])
	}
	if len(knots[0].Parties) != 2 || knots[0].Parties[0] != "alice" || knots[0].Parties[1] != "bob" {
		t.Errorf("knot[0] parties = %v, want [alice bob] trimmed", knots[0].Parties)
	}
	// "high" word-form confidence is parsed.
	if knots[1].Confidence != 0.9 {
		t.Errorf("knot[1] conf = %v, want 0.9 from 'high'", knots[1].Confidence)
	}
	// No CONF field (trailing empty) → fallback to minConfForParties: bob's
	// inferred 0.4 drags it below the FIRM threshold.
	if knots[2].Confidence != 0.4 || knots[2].Firm() {
		t.Errorf("knot[2] conf = %v firm=%v, want 0.4 fallback / soft", knots[2].Confidence, knots[2].Firm())
	}
}

func TestParseKnotsTeamwideGate(t *testing.T) {
	out := "collision | a, b | x | y | 0.9\nteamwide-divergence | a, b, c | deadline | they diverge | 0.8"
	// pairwise gate rejects teamwide; teamwide gate rejects collision.
	if got := parseKnots(out, nil, pairwiseKinds); len(got) != 1 || got[0].Kind != KindCollision {
		t.Errorf("pairwise gate = %+v, want only collision", got)
	}
	if got := parseKnots(out, nil, teamwideKinds); len(got) != 1 || got[0].Kind != KindTeamwideDivergence {
		t.Errorf("teamwide gate = %+v, want only teamwide-divergence", got)
	}
}

func TestFirm(t *testing.T) {
	if !(Knot{Confidence: 0.5}).Firm() {
		t.Error("0.5 should be FIRM (>= threshold)")
	}
	if (Knot{Confidence: 0.49}).Firm() {
		t.Error("0.49 should be SOFT")
	}
}

func TestSamePerson(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"alice", "alice", true},
		{"Alice", "alice", true},
		{" alice ", "alice", true},
		{"alice", "bob", false},
		{"", "", true},
	}
	for _, c := range cases {
		if got := SamePerson(c.a, c.b); got != c.want {
			t.Errorf("SamePerson(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestMinConfForParties(t *testing.T) {
	atoms := []Atom{
		{From: "alice", Confidence: 1.0},
		{From: "bob", Confidence: 0.4},
	}
	if got := minConfForParties(atoms, []string{"alice", "bob"}); got != 0.4 {
		t.Errorf("got %v, want 0.4 (lowest among parties)", got)
	}
	if got := minConfForParties(atoms, []string{"alice"}); got != 1.0 {
		t.Errorf("got %v, want 1.0", got)
	}
	if got := minConfForParties(atoms, []string{"nobody"}); got != 1.0 {
		t.Errorf("got %v, want 1.0 fallback when no party matches", got)
	}
}

func TestSameKnot(t *testing.T) {
	base := Knot{Parties: []string{"alice", "bob"}, About: "the GetUser rename"}
	cases := []struct {
		name string
		b    Knot
		want bool
	}{
		// shared party + shared salient keyword, even with the Kind relabeled and
		// the parties reshuffled — this is the case voting must catch.
		{"relabeled + reordered", Knot{Kind: KindDecisionRights, Parties: []string{"bob", "alice"}, About: "rename of GetUser"}, true},
		// party overlaps but subjects share no salient keyword.
		{"same parties, different subject", Knot{Parties: []string{"alice", "bob"}, About: "cache invalidation"}, false},
		// subject overlaps but no shared party.
		{"shared subject, no party", Knot{Parties: []string{"carol", "dave"}, About: "the GetUser rename"}, false},
		// stopwords + short words alone must not count as overlap.
		{"only stopwords overlap", Knot{Parties: []string{"alice"}, About: "the will of the"}, false},
	}
	for _, c := range cases {
		if got := SameKnot(base, c.b); got != c.want {
			t.Errorf("%s: SameKnot = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSingleAuthor(t *testing.T) {
	if !singleAuthor([]string{"alice"}) {
		t.Error("one party is a single author")
	}
	if !singleAuthor([]string{"alice", "Alice "}) {
		t.Error("same person twice (case/space) is a single author")
	}
	if singleAuthor([]string{"alice", "bob"}) {
		t.Error("two distinct people is not a self-knot")
	}
	if singleAuthor(nil) {
		t.Error("no party is not a single author")
	}
}

func TestDedupeSelf(t *testing.T) {
	self := []Knot{
		{Kind: KindStaleAssumption, Parties: []string{"alice"}, About: "the launch deadline"},
		{Kind: KindStaleAssumption, Parties: []string{"bob"}, About: "cache ownership"},
	}
	cross := []Knot{
		// a team-wide knot already covering alice's launch-deadline drift.
		{Kind: KindTeamwideDivergence, Parties: []string{"alice", "bob", "carol"}, About: "launch deadline"},
	}
	got := DedupeSelf(self, cross)
	if len(got) != 1 || !SamePerson(got[0].Parties[0], "bob") {
		t.Fatalf("DedupeSelf = %+v, want only bob's self-knot (alice's covered team-wide)", got)
	}
}

func TestVoteKnots(t *testing.T) {
	// Three runs. The rename knot recurs in all three (relabeled/reworded each
	// time); a one-off hallucination appears in a single run and must be dropped.
	runs := [][]Knot{
		{
			{Kind: KindCollision, Parties: []string{"alice", "bob"}, About: "GetUser rename", Confidence: 0.9},
			{Kind: KindDuplication, Parties: []string{"carol", "dave"}, About: "hallucinated overlap", Confidence: 0.8},
		},
		{
			{Kind: KindDecisionRights, Parties: []string{"bob", "alice"}, About: "the rename of GetUser", Confidence: 1.0},
		},
		{
			{Kind: KindCollision, Parties: []string{"alice", "bob"}, About: "GetUser rename collision", Confidence: 0.8},
		},
	}
	got := voteKnots(runs)
	if len(got) != 1 {
		t.Fatalf("voteKnots kept %d knots, want 1 (the one-off dropped by majority): %+v", len(got), got)
	}
	k := got[0]
	if k.Votes != 3 || k.Samples != 3 {
		t.Errorf("votes/samples = %d/%d, want 3/3", k.Votes, k.Samples)
	}
	// representative is the highest-confidence member (1.0); confidence is the mean.
	if k.Confidence < 0.89 || k.Confidence > 0.91 {
		t.Errorf("voted confidence = %v, want mean ~0.9", k.Confidence)
	}
}
