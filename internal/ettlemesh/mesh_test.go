package ettlemesh

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

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
	atoms, err := detWith(m).Distill(context.Background(), "alice", "backend", "some note", nil)
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
	_, err := detWith(m).Distill(context.Background(), "alice", "r", "note", nil)
	if err == nil {
		t.Fatal("expected the client error to propagate")
	}
}

// cannedCall is one scripted model reply (a response or an error).
type cannedCall struct {
	resp *anthropic.Message
	err  error
}

// seqMessager returns canned replies in order, one per call — lets a test model a
// garble-then-recover sequence the semantic retry loop is meant to survive.
type seqMessager struct {
	replies []cannedCall
	calls   int
}

func (s *seqMessager) New(ctx context.Context, body anthropic.MessageNewParams, opts ...option.RequestOption) (*anthropic.Message, error) {
	i := s.calls
	s.calls++
	if i >= len(s.replies) {
		return nil, errors.New("seqMessager: unexpected extra call")
	}
	return s.replies[i].resp, s.replies[i].err
}

func TestCallToolRetriesSchemaGarbleThenSucceeds(t *testing.T) {
	// First reply garbles the schema (atoms as a string, not an array) — exactly
	// the haiku infer_assumptions failure; the re-roll recovers.
	garbled := toolResp(`{"atoms":"not-an-array"}`)
	good := toolResp(`{"atoms":[{"type":"intent","subject":"s","content":"c"}]}`)
	m := &seqMessager{replies: []cannedCall{{resp: garbled}, {resp: good}}}
	atoms, err := detWith(m).Distill(context.Background(), "alice", "r", "note", nil)
	if err != nil {
		t.Fatalf("expected recovery on the second sample, got error: %v", err)
	}
	if len(atoms) != 1 {
		t.Fatalf("got %d atoms, want 1 from the recovered call", len(atoms))
	}
	if m.calls != 2 {
		t.Errorf("made %d calls, want exactly 2 (one garble + one recovery)", m.calls)
	}
}

func TestCallToolFailsLoudAfterMaxAttempts(t *testing.T) {
	garbled := toolResp(`{"atoms":"not-an-array"}`)
	m := &seqMessager{replies: []cannedCall{{resp: garbled}, {resp: garbled}, {resp: garbled}}}
	_, err := detWith(m).Distill(context.Background(), "alice", "r", "note", nil)
	if err == nil {
		t.Fatal("expected a loud error after the attempt budget is spent, not a silent empty")
	}
	if !strings.Contains(err.Error(), "after 3 attempts") {
		t.Errorf("error %q should report the spent budget", err)
	}
	if m.calls != maxToolAttempts {
		t.Errorf("made %d calls, want exactly %d (the bounded budget)", m.calls, maxToolAttempts)
	}
}

func TestCallToolTransportErrorNotRetried(t *testing.T) {
	// A transport error is already SDK-retried (WithMaxRetries); it must NOT be
	// re-rolled by the semantic loop, or a hard 4xx multiplies the spend.
	m := &seqMessager{replies: []cannedCall{{err: errors.New("400 bad request")}}}
	_, err := detWith(m).Distill(context.Background(), "alice", "r", "note", nil)
	if err == nil {
		t.Fatal("expected the transport error to propagate")
	}
	if m.calls != 1 {
		t.Errorf("made %d calls, want exactly 1 (transport errors are terminal here)", m.calls)
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
	// Multibyte content must never be cut mid-rune: a string of 2-byte runes
	// clipped at an odd byte cap must still be valid UTF-8 crossing the boundary.
	multibyte := strings.Repeat("é", 50) // 100 bytes, no spaces
	if out := clip(multibyte, 81); !utf8.ValidString(out) {
		t.Errorf("clip produced invalid UTF-8: %q", out)
	}
}

func TestSealAtom(t *testing.T) {
	// The single boundary chokepoint redacts BOTH a secret-shaped span and a
	// user-marked private phrase, and reports unusable (empty) fields.
	_, content, ok := sealAtom("alice",
		"rotate token", "set CI secret to ghp_AbCd1234EfGh5678IjKl before the Lisbon move",
		[]string{"Lisbon move"})
	if !ok {
		t.Fatal("expected a usable atom")
	}
	if strings.Contains(content, "ghp_AbCd1234EfGh5678IjKl") {
		t.Errorf("secret survived the seal: %q", content)
	}
	if strings.Contains(strings.ToLower(content), "lisbon move") {
		t.Errorf("private phrase survived the seal: %q", content)
	}
	if _, _, ok := sealAtom("alice", "  ", "real content", nil); ok {
		t.Error("an empty subject must make the atom unusable (ok=false)")
	}
}

func TestInferImplicitScrubsSecrets(t *testing.T) {
	// Regression for the dual-path bug: inferred atoms (and the questions rendered
	// from low-confidence inferences) must run through the SAME secret scanner as
	// Distill. A token folded into an inference must not cross unredacted.
	m := &fakeMessager{resp: toolResp(`{"inferences":[
		{"confidence":"high","subject":"deploy creds","content":"assumes ghp_AbCd1234EfGh5678IjKl still works"},
		{"confidence":"low","subject":"db dsn","content":"guesses pg://app:Hunter2Pg@db.internal still valid"}
	]}`)}
	inferred, questions, err := detWith(m).InferImplicit(context.Background(), "alice", "backend", "some post", nil)
	if err != nil {
		t.Fatalf("InferImplicit error: %v", err)
	}
	if len(inferred) != 1 {
		t.Fatalf("got %d inferred atoms, want 1 (the high-confidence one)", len(inferred))
	}
	if strings.Contains(inferred[0].Content, "ghp_AbCd1234EfGh5678IjKl") {
		t.Errorf("token survived in inferred atom: %q", inferred[0].Content)
	}
	if len(questions) != 1 {
		t.Fatalf("got %d questions, want 1 (the low-confidence one)", len(questions))
	}
	if strings.Contains(questions[0], "Hunter2Pg") {
		t.Errorf("password survived in rendered question: %q", questions[0])
	}
}

func TestVoteKnotsConfidenceDedupesWithinRun(t *testing.T) {
	// A single run can name one divergence in BOTH the pairwise and team-wide pass
	// (reconcileBoth concatenates them). That run must count once for the
	// confidence mean, not twice — so the mean is over distinct runs, not items.
	runs := [][]Knot{
		{ // run 0 names it twice at 0.9
			{Kind: KindCollision, Parties: []string{"alice", "bob"}, About: "GetUser rename collision", Confidence: 0.9},
			{Kind: KindTeamwideDivergence, Parties: []string{"alice", "bob"}, About: "the GetUser rename", Confidence: 0.9},
		},
		{ // run 1 names it once at 0.5
			{Kind: KindCollision, Parties: []string{"bob", "alice"}, About: "GetUser rename", Confidence: 0.5},
		},
	}
	got := voteKnots(runs)
	if len(got) != 1 {
		t.Fatalf("voteKnots kept %d, want 1 clustered knot: %+v", len(got), got)
	}
	// Per-run-deduped mean = (0.9 + 0.5)/2 = 0.70, NOT (0.9+0.9+0.5)/3 = 0.767.
	if got[0].Confidence < 0.69 || got[0].Confidence > 0.71 {
		t.Errorf("voted confidence = %v, want ~0.70 (run0 counted once, not twice)", got[0].Confidence)
	}
	if got[0].Votes != 2 {
		t.Errorf("votes = %d, want 2 distinct runs", got[0].Votes)
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
	// Single-run path (no votes): confidence decides.
	if !(Knot{Confidence: 0.5}).Firm() {
		t.Error("0.5 should be FIRM (>= threshold)")
	}
	if (Knot{Confidence: 0.49}).Firm() {
		t.Error("0.49 should be SOFT")
	}
	// Voted path (Samples>0): recurrence frequency decides, confidence ignored.
	if !(Knot{Votes: 3, Samples: 5, Confidence: 0.1}).Firm() {
		t.Error("3/5 (majority) should be FIRM regardless of low confidence")
	}
	if (Knot{Votes: 2, Samples: 5, Confidence: 0.99}).Firm() {
		t.Error("2/5 (minority) should be SOFT regardless of high confidence")
	}
	if !(Knot{Votes: 5, Samples: 5, Confidence: 0}).Firm() {
		t.Error("5/5 should be FIRM")
	}
	if (Knot{Votes: 1, Samples: 12}).Firm() {
		t.Error("1/12 (fabrication-frequency) should be SOFT")
	}
	// Per-kind bar: a flickery-but-real decision-rights knot (2/5 = 0.4) clears its
	// lowered 0.3 bar and asserts, where the same recurrence at the default bar is
	// soft. This is the recall fix for auth-migration K2.
	if !(Knot{Kind: KindDecisionRights, Votes: 2, Samples: 5}).Firm() {
		t.Error("decision-rights 2/5 should be FIRM at its lowered 0.3 bar")
	}
	if (Knot{Kind: KindCollision, Votes: 2, Samples: 5}).Firm() {
		t.Error("collision 2/5 should be SOFT at the default 0.5 bar")
	}
	// The lowered bar still clears the ~0.17 fabrication ceiling: a single stray
	// sample (1/5 = 0.2) does NOT assert even for decision-rights.
	if (Knot{Kind: KindDecisionRights, Votes: 1, Samples: 5}).Firm() {
		t.Error("decision-rights 1/5 should be SOFT — below the 0.3 bar, in the fabrication band")
	}
}

func TestFirmVoteFractionFor(t *testing.T) {
	if got := firmVoteFractionFor(KindDecisionRights); got != 0.3 {
		t.Errorf("decision-rights bar = %v, want 0.3", got)
	}
	if got := firmVoteFractionFor(KindCollision); got != firmVoteFractionDefault {
		t.Errorf("collision bar = %v, want default %v", got, firmVoteFractionDefault)
	}
	if got := firmVoteFractionFor("anything-unknown"); got != firmVoteFractionDefault {
		t.Errorf("unknown kind bar = %v, want default %v", got, firmVoteFractionDefault)
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
	// Keep-all: the 3-run GetUser cluster AND the 1-run carol/dave one-off both
	// survive (frequency ranks firm/soft, it does not drop). Output is sorted
	// most-voted first.
	if len(got) != 2 {
		t.Fatalf("voteKnots kept %d, want 2 (cluster + kept one-off): %+v", len(got), got)
	}
	k := got[0]
	if k.Votes != 3 || k.Samples != 3 {
		t.Errorf("top knot votes/samples = %d/%d, want 3/3", k.Votes, k.Samples)
	}
	if !k.Firm() {
		t.Error("a 3/3 knot must be FIRM (asserted)")
	}
	if k.Confidence < 0.89 || k.Confidence > 0.91 {
		t.Errorf("voted confidence = %v, want mean ~0.9", k.Confidence)
	}
	oneOff := got[1]
	if oneOff.Votes != 1 || oneOff.Samples != 3 {
		t.Errorf("one-off votes/samples = %d/%d, want 1/3", oneOff.Votes, oneOff.Samples)
	}
	if oneOff.Firm() {
		t.Error("a 1/3 one-off must be SOFT (a question), not dropped or asserted")
	}
}

// TestDropFloor pins the abstention gate (dropFloorFraction). At samples=5 the
// single-appearance tail (the fabrication signature) is DROPPED while >=2/5 knots
// survive; at samples=3 the floor is inert (default --ab is unaffected); and the
// floor never drops a knot the per-kind firm bar would assert.
func TestDropFloor(t *testing.T) {
	// 5 runs. Recurrence by knot:
	//   collision alice/bob   : 5/5 → kept, FIRM
	//   collision eve/finn    : 2/5 → kept (above floor, below the 0.5 collision firm
	//                                  bar → SOFT) — proves "kept but soft"
	//   decision-rights gid/hal: 2/5 → FIRM by the 0.3 bar (2>=1.5) AND kept (2>=1.25)
	//                                  — the floor∩firm invariant, operationally
	//   duplication carol/dave : 1/5 → DROPPED (1 < 0.25*5 = 1.25)
	clear := Knot{Kind: KindCollision, Parties: []string{"alice", "bob"}, About: "GetUser rename", Confidence: 0.9}
	twice := Knot{Kind: KindCollision, Parties: []string{"eve", "finn"}, About: "ledger schema", Confidence: 0.8}
	rights := Knot{Kind: KindDecisionRights, Parties: []string{"gid", "hal"}, About: "who owns the cutover", Confidence: 0.7}
	oneOff := Knot{Kind: KindDuplication, Parties: []string{"carol", "dave"}, About: "spurious overlap", Confidence: 0.8}
	runs := [][]Knot{
		{clear, twice, rights, oneOff},
		{clear, twice, rights},
		{clear},
		{clear},
		{clear},
	}
	got := voteKnots(runs)
	by := map[string]Knot{}
	for _, k := range got {
		by[k.Kind+"\x00"+strings.Join(k.Parties, "+")] = k
	}
	if _, dropped := by[KindDuplication+"\x00carol+dave"]; dropped {
		t.Errorf("a 1/5 one-off must be DROPPED by the floor, but it survived: %+v", got)
	}
	if len(got) != 3 {
		t.Fatalf("voteKnots kept %d, want 3 (5/5, 2/5, 2/5; the 1/5 dropped): %+v", len(got), got)
	}
	if c := by[KindCollision+"\x00alice+bob"]; c.Votes != 5 || !c.Firm() {
		t.Errorf("5/5 collision = %d/%d firm=%v, want 5/5 FIRM", c.Votes, c.Samples, c.Firm())
	}
	if e := by[KindCollision+"\x00eve+finn"]; e.Votes != 2 || e.Firm() {
		t.Errorf("2/5 collision = %d/%d firm=%v, want 2/5 KEPT-but-SOFT", e.Votes, e.Samples, e.Firm())
	}
	// The invariant: a knot the per-kind firm bar would assert is never floored away.
	r, ok := by[KindDecisionRights+"\x00gid+hal"]
	if !ok {
		t.Fatal("a FIRM 2/5 decision-rights knot was dropped by the floor — invariant violated")
	}
	if !r.Firm() {
		t.Errorf("2/5 decision-rights should be FIRM by the 0.3 bar, got firm=%v", r.Firm())
	}

	// Inertness at samples=3: the floor (threshold 0.75) cannot drop a Votes>=1 knot.
	small := voteKnots([][]Knot{
		{clear, oneOff},
		{clear},
		{clear},
	})
	foundOneOff := false
	for _, k := range small {
		if k.Kind == KindDuplication {
			foundOneOff = true
		}
	}
	if !foundOneOff {
		t.Errorf("at samples=3 the floor must be INERT (1/3 kept), but the one-off was dropped: %+v", small)
	}
}
