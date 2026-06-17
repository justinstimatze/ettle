package ettlemesh

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// fakeMessager is the canned model boundary: it returns a pre-baked tool_use
// response (or an error / a no-tool response) so every model-calling path is
// unit-testable without a network call or an API key.
type fakeMessager struct {
	resp  *anthropic.Message
	err   error
	calls int
}

func (f *fakeMessager) New(ctx context.Context, body anthropic.MessageNewParams, opts ...option.RequestOption) (*anthropic.Message, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func toolResp(input string) *anthropic.Message {
	return &anthropic.Message{
		StopReason: "tool_use",
		Content:    []anthropic.ContentBlockUnion{{Type: "tool_use", Input: json.RawMessage(input)}},
	}
}

func detWith(m messager) *Detector { return &Detector{msgs: m, Model: "test"} }

func TestDistillStructured(t *testing.T) {
	// valid intent kept; bogus type dropped; empty content dropped.
	m := &fakeMessager{resp: toolResp(`{"atoms":[
		{"type":"intent","subject":"rename","content":"renaming GetUser"},
		{"type":"bogus","subject":"x","content":"y"},
		{"type":"dependency","subject":"cache","content":""}
	]}`)}
	atoms, err := detWith(m).Distill(context.Background(), "alice", "backend", "some note")
	if err != nil {
		t.Fatalf("Distill error: %v", err)
	}
	if len(atoms) != 1 {
		t.Fatalf("got %d atoms, want 1 (bogus type + empty content dropped): %+v", len(atoms), atoms)
	}
	if atoms[0].Typ != Intent || atoms[0].From != "alice" || atoms[0].Confidence != 1.0 {
		t.Errorf("atom = %+v, want intent/alice/1.0", atoms[0])
	}
}

func TestReconcileGateAndConfidence(t *testing.T) {
	atoms := []Atom{{From: "alice", Confidence: 1.0}, {From: "bob", Confidence: 1.0}}
	// a collision at 0.9 (model-reported, trusted) plus a teamwide-divergence the
	// pairwise gate must reject.
	m := &fakeMessager{resp: toolResp(`{"knots":[
		{"kind":"collision","parties":["alice","bob"],"about":"the rename","explanation":"they collide","confidence":0.9},
		{"kind":"teamwide-divergence","parties":["alice","bob"],"about":"deadline","explanation":"x","confidence":0.9}
	]}`)}
	knots, err := detWith(m).Reconcile(context.Background(), atoms)
	if err != nil {
		t.Fatalf("Reconcile error: %v", err)
	}
	if len(knots) != 1 || knots[0].Kind != KindCollision {
		t.Fatalf("got %+v, want only the collision (teamwide gated out)", knots)
	}
	// model confidence is the designed "min over depended atoms" signal — trusted.
	if knots[0].Confidence != 0.9 || !knots[0].Firm() {
		t.Errorf("conf = %v firm=%v, want 0.9 / firm (model confidence trusted)", knots[0].Confidence, knots[0].Firm())
	}
}

func TestBuildKnotsConfidenceFallback(t *testing.T) {
	atoms := []Atom{{From: "alice", Confidence: 1.0}, {From: "bob", Confidence: 0.4}}
	// confidence omitted (0) → fall back to party-atom minimum (bob 0.4).
	p := knotsPayload{}
	_ = json.Unmarshal([]byte(`{"knots":[{"kind":"collision","parties":["alice","bob"],"about":"x","explanation":"y","confidence":0}]}`), &p)
	got := buildKnots(p, atoms, pairwiseKinds, false)
	if len(got) != 1 || got[0].Confidence != 0.4 {
		t.Fatalf("fallback conf = %+v, want 0.4 (party-atom min)", got)
	}
	// confidence omitted + no matching party atoms → low 0.3 fallback (unanchored).
	p2 := knotsPayload{}
	_ = json.Unmarshal([]byte(`{"knots":[{"kind":"collision","parties":["ghost","phantom"],"about":"x","explanation":"y","confidence":0}]}`), &p2)
	got2 := buildKnots(p2, atoms, pairwiseKinds, false)
	if len(got2) != 1 || got2[0].Confidence != 0.3 {
		t.Fatalf("unanchored fallback conf = %+v, want 0.3", got2)
	}
}

func TestReconcileSelfRejectsTwoParties(t *testing.T) {
	atoms := []Atom{{From: "dana", Confidence: 1.0}}
	m := &fakeMessager{resp: toolResp(`{"knots":[
		{"kind":"stale-assumption","parties":["dana"],"about":"retry logic","explanation":"x","confidence":1.0},
		{"kind":"stale-assumption","parties":["dana","sam"],"about":"other","explanation":"y","confidence":1.0}
	]}`)}
	knots, err := detWith(m).ReconcileSelf(context.Background(), atoms)
	if err != nil {
		t.Fatalf("ReconcileSelf error: %v", err)
	}
	if len(knots) != 1 || !singleAuthor(knots[0].Parties) {
		t.Fatalf("got %+v, want only the single-author self-knot", knots)
	}
}

func TestCallToolNoToolUseIsLoud(t *testing.T) {
	// model produced only prose (no tool_use block) — must be a loud error, NOT a
	// silent empty/all-clear.
	m := &fakeMessager{resp: &anthropic.Message{
		StopReason: "end_turn",
		Content:    []anthropic.ContentBlockUnion{{Type: "text", Text: "I'm not sure."}},
	}}
	_, err := detWith(m).Reconcile(context.Background(), nil)
	if err == nil {
		t.Fatal("expected a loud error when the model returns no tool_use block")
	}
}

func TestCallToolPropagatesError(t *testing.T) {
	m := &fakeMessager{err: errors.New("429 rate limited")}
	_, err := detWith(m).Distill(context.Background(), "alice", "r", "note")
	if err == nil {
		t.Fatal("expected the client error to propagate")
	}
}

func TestClip(t *testing.T) {
	// collapses whitespace/newlines to single spaces (no multi-line prose).
	if got := clip("  a\n\nb   c ", 80); got != "a b c" {
		t.Errorf("clip whitespace = %q, want %q", got, "a b c")
	}
	// truncates past the cap, at a word boundary, bounding how much can leak.
	long := "this is a deliberately long content string that should be clipped at a word boundary well before its natural end so the boundary stays a clause"
	got := clip(long, 40)
	if len(got) > 40 {
		t.Errorf("clip len = %d, want <= 40", len(got))
	}
	if strings.HasSuffix(got, " ") || strings.Contains(got, "  ") {
		t.Errorf("clip should trim trailing space / collapse: %q", got)
	}
}

func TestConfFromWord(t *testing.T) {
	cases := []struct {
		in     string
		want   float64
		wantOK bool
	}{
		{"high", 0.6, true}, {"HIGH", 0.6, true}, {"medium", 0.4, true},
		{"low", 0, false}, {"", 0, false}, {"banana", 0, false},
	}
	for _, c := range cases {
		got, ok := confFromWord(c.in)
		if ok != c.wantOK || (ok && got != c.want) {
			t.Errorf("confFromWord(%q) = (%v,%v), want (%v,%v)", c.in, got, ok, c.want, c.wantOK)
		}
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
	// no party matches → LOW (0.3), so an unanchored/hallucinated-party knot is a
	// question, not asserted (previously this returned 1.0).
	if got := minConfForParties(atoms, []string{"nobody"}); got != 0.3 {
		t.Errorf("got %v, want 0.3 low fallback when no party matches", got)
	}
}

func TestSameKnot(t *testing.T) {
	base := Knot{Parties: []string{"alice", "bob"}, About: "the GetUser rename"}
	cases := []struct {
		name string
		b    Knot
		want bool
	}{
		{"relabeled + reordered", Knot{Kind: KindDecisionRights, Parties: []string{"bob", "alice"}, About: "rename of GetUser"}, true},
		{"same parties, different subject", Knot{Parties: []string{"alice", "bob"}, About: "cache invalidation"}, false},
		{"shared subject, no party", Knot{Parties: []string{"carol", "dave"}, About: "the GetUser rename"}, false},
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
		{Kind: KindTeamwideDivergence, Parties: []string{"alice", "bob", "carol"}, About: "launch deadline"},
	}
	got := DedupeSelf(self, cross)
	if len(got) != 1 || !SamePerson(got[0].Parties[0], "bob") {
		t.Fatalf("DedupeSelf = %+v, want only bob's self-knot (alice's covered team-wide)", got)
	}
}

func TestVoteKnots(t *testing.T) {
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
		t.Fatalf("voteKnots kept %d, want 1 (one-off dropped by majority): %+v", len(got), got)
	}
	k := got[0]
	if k.Votes != 3 || k.Samples != 3 {
		t.Errorf("votes/samples = %d/%d, want 3/3", k.Votes, k.Samples)
	}
	if k.Confidence < 0.89 || k.Confidence > 0.91 {
		t.Errorf("voted confidence = %v, want mean ~0.9", k.Confidence)
	}
}
