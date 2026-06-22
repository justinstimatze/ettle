package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
)

// fakeReconciler implements the reconciler seam with canned returns — no API
// key, no ettlemesh internals. This is the whole reason the server depends on an
// interface rather than *ettlemesh.Detector.
type fakeReconciler struct {
	mu                                  sync.Mutex
	distillCalls, votedCalls, selfCalls int
	lastNotes                           string
	atoms                               []ettlemesh.Atom // returned by Distill (From overwritten with the caller)
	voted                               []ettlemesh.Knot // returned by ReconcileVoted
	self                                []ettlemesh.Knot // returned by ReconcileSelf
}

func (f *fakeReconciler) Distill(_ context.Context, from, _, text string, _ []string) ([]ettlemesh.Atom, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.distillCalls++
	f.lastNotes = text
	out := make([]ettlemesh.Atom, len(f.atoms))
	for i, a := range f.atoms {
		a.From = from
		out[i] = a
	}
	return out, nil
}

func (f *fakeReconciler) ReconcileVoted(_ context.Context, _ []ettlemesh.Atom, _ int) (knots []ettlemesh.Knot, floorDropped int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.votedCalls++
	return f.voted, 0, nil
}

// GroundKnots is a pass-through in the fake: the direction-check is exercised in
// ettlemesh's own tests; here the server just needs the seam satisfied.
func (f *fakeReconciler) GroundKnots(_ context.Context, knots []ettlemesh.Knot, _ []ettlemesh.Atom) (kept, suppressed []ettlemesh.Knot, err error) {
	return knots, nil, nil
}

func (f *fakeReconciler) ReconcileSelf(_ context.Context, _ []ettlemesh.Atom) ([]ettlemesh.Knot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.selfCalls++
	return f.self, nil
}

func newServerWith(f *fakeReconciler) *server { return &server{det: f, h: newHorizon()} }

// memLabelSink captures verdicts in memory for tests — no filesystem.
type memLabelSink struct {
	mu  sync.Mutex
	got []Label
}

func (m *memLabelSink) record(l Label) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.got = append(m.got, l)
	return nil
}

// Stage 0c-2: ettle_respond captures the human verdict as a label (the calibration
// loop's future input), validates the verdict, and never mutates the horizon.
func TestRespondCapturesLabel(t *testing.T) {
	sink := &memLabelSink{}
	s := &server{det: &fakeReconciler{}, h: newHorizon(), labels: sink}
	ctx := context.Background()

	_, out, err := s.respond(ctx, nil, respondIn{Me: "mabel", Knot: "collision|mabel+opal", Verdict: "not_real", Note: "pipeline, not a clash"})
	if err != nil {
		t.Fatalf("respond errored: %v", err)
	}
	if !out.Recorded || out.Verdict != "not_real" {
		t.Fatalf("unexpected out: %+v", out)
	}
	if len(sink.got) != 1 {
		t.Fatalf("want 1 label captured, got %d", len(sink.got))
	}
	if l := sink.got[0]; l.Key != "collision|mabel+opal" || l.By != "mabel" || l.Verdict != "not_real" || l.TS == "" {
		t.Fatalf("label not recorded faithfully: %+v", l)
	}

	// A bad verdict is rejected and nothing is captured.
	if _, _, err := s.respond(ctx, nil, respondIn{Me: "mabel", Knot: "k", Verdict: "maybe"}); err == nil {
		t.Fatal("expected an error for an invalid verdict")
	}
	// Missing fields rejected.
	if _, _, err := s.respond(ctx, nil, respondIn{Me: "", Knot: "k", Verdict: "real"}); err == nil {
		t.Fatal("expected an error for a missing responder")
	}
	if len(sink.got) != 1 {
		t.Fatalf("rejected calls must not capture; got %d labels", len(sink.got))
	}
}

// The knot key an agent answers must match the key the horizon hands out (wording-
// independent: same kind + parties => same key regardless of order/case/About).
func TestKnotKeyStableAndCrossCallMatch(t *testing.T) {
	a := knotKey("collision", []string{"Opal", " mabel "})
	b := knotKey("collision", []string{"mabel", "opal"})
	if a != b {
		t.Fatalf("key not order/case-stable: %q vs %q", a, b)
	}
	if a != "collision|mabel+opal" {
		t.Fatalf("unexpected key form: %q", a)
	}
}

func TestEmitDistillsStoresAndDropsRaw(t *testing.T) {
	f := &fakeReconciler{atoms: []ettlemesh.Atom{
		{Typ: ettlemesh.Dependency, Subject: "user-service/cache", Content: "relies on it", Confidence: 1},
		{Typ: ettlemesh.Commitment, Subject: "rename", Content: "lands Thursday", Confidence: 1},
	}}
	s := newServerWith(f)

	res, out, err := s.emit(context.Background(), nil, emitIn{Participant: "Alice", Role: "backend", Notes: "secret raw reasoning"})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	if out.Count != 2 || len(out.Atoms) != 2 {
		t.Fatalf("expected 2 atoms returned, got count=%d len=%d", out.Count, len(out.Atoms))
	}
	if res == nil || len(res.Content) == 0 {
		t.Error("expected a human-readable summary in the result content")
	}
	// The raw notes went to Distill but are never stored — the horizon holds only
	// distilled atoms (Envelope has no notes field, so this is structural).
	if f.lastNotes != "secret raw reasoning" {
		t.Errorf("Distill should receive the raw notes, got %q", f.lastNotes)
	}
	envs := s.h.snapshot()
	if len(envs) != 1 || len(envs[0].Atoms) != 2 {
		t.Fatalf("horizon should hold Alice's 2 atoms, got %+v", envs)
	}
	if envs[0].Atoms[0].From != "Alice" {
		t.Errorf("stored atoms should be attributed to Alice, got %q", envs[0].Atoms[0].From)
	}
}

func TestEmitUpsertReplaces(t *testing.T) {
	f := &fakeReconciler{atoms: []ettlemesh.Atom{{Typ: ettlemesh.Commitment, Subject: "x", Confidence: 1}}}
	s := newServerWith(f)
	_, _, _ = s.emit(context.Background(), nil, emitIn{Participant: "alice", Notes: "first"})
	_, _, _ = s.emit(context.Background(), nil, emitIn{Participant: "Alice ", Notes: "second"})
	// Same person (folded) → one entry, not two.
	if envs := s.h.snapshot(); len(envs) != 1 {
		t.Fatalf("upsert should fold alice/'Alice ' to one participant, got %d", len(envs))
	}
}

func TestEmitRejectsBlank(t *testing.T) {
	s := newServerWith(&fakeReconciler{})
	if _, _, err := s.emit(context.Background(), nil, emitIn{Participant: "", Notes: "x"}); err == nil {
		t.Error("blank participant should error")
	}
	if _, _, err := s.emit(context.Background(), nil, emitIn{Participant: "a", Notes: ""}); err == nil {
		t.Error("blank notes should error")
	}
}

func TestHorizonFirmSoftSplitAndMeFilter(t *testing.T) {
	firmKnot := ettlemesh.Knot{Kind: ettlemesh.KindCollision, Parties: []string{"alice", "bob"}, Confidence: 0.6}
	softKnot := ettlemesh.Knot{Kind: ettlemesh.KindDuplication, Parties: []string{"alice", "carol"}, Confidence: 0.4}
	f := &fakeReconciler{
		atoms: []ettlemesh.Atom{{Typ: ettlemesh.Dependency, Subject: "x", Confidence: 1}},
		voted: []ettlemesh.Knot{firmKnot, softKnot},
	}
	s := newServerWith(f)
	// Need atoms in the horizon so the empty-guard doesn't short-circuit.
	_, _, _ = s.emit(context.Background(), nil, emitIn{Participant: "alice", Notes: "n"})

	// Full team view: firm/soft split on Knot.Firm() (Samples==0 → confidence>=0.5).
	_, all, err := s.horizon(context.Background(), nil, horizonIn{})
	if err != nil {
		t.Fatalf("horizon: %v", err)
	}
	if len(all.Firm) != 1 || all.Firm[0].Kind != ettlemesh.KindCollision {
		t.Fatalf("expected 1 firm collision, got %+v", all.Firm)
	}
	if len(all.Soft) != 1 || all.Soft[0].Kind != ettlemesh.KindDuplication {
		t.Fatalf("expected 1 soft duplication, got %+v", all.Soft)
	}

	// me=bob (with whitespace/case to exercise the SamePerson fold) → only the
	// collision, which is firm; the duplication doesn't involve bob.
	_, bob, _ := s.horizon(context.Background(), nil, horizonIn{Me: " Bob"})
	if len(bob.Firm) != 1 || len(bob.Soft) != 0 {
		t.Fatalf("bob should see 1 firm, 0 soft; got firm=%d soft=%d", len(bob.Firm), len(bob.Soft))
	}

	// me=dave → involved in nothing.
	_, dave, _ := s.horizon(context.Background(), nil, horizonIn{Me: "dave"})
	if len(dave.Firm) != 0 || len(dave.Soft) != 0 {
		t.Fatalf("dave should see nothing, got firm=%d soft=%d", len(dave.Firm), len(dave.Soft))
	}
}

func TestHorizonEmptyGuardSkipsModel(t *testing.T) {
	f := &fakeReconciler{}
	s := newServerWith(f)
	_, out, err := s.horizon(context.Background(), nil, horizonIn{})
	if err != nil {
		t.Fatalf("horizon: %v", err)
	}
	if len(out.Firm) != 0 || len(out.Soft) != 0 || len(out.Participants) != 0 {
		t.Errorf("empty horizon should be empty, got %+v", out)
	}
	if f.votedCalls != 0 {
		t.Errorf("empty horizon must NOT call the model, got %d voted calls", f.votedCalls)
	}
	// Slices are non-nil so the JSON renders [] not null.
	if out.Firm == nil || out.Soft == nil {
		t.Error("firm/soft should be empty non-nil slices")
	}
}

func TestSelfCheckSinglePartyAndStateless(t *testing.T) {
	f := &fakeReconciler{
		atoms: []ettlemesh.Atom{{Typ: ettlemesh.Assumption, Subject: "timeline", Confidence: 1}},
		self:  []ettlemesh.Knot{{Kind: ettlemesh.KindStaleAssumption, Parties: []string{"alice"}, Confidence: 0.6}},
	}
	s := newServerWith(f)
	_, out, err := s.selfCheck(context.Background(), nil, selfIn{Participant: "alice", Notes: "n"})
	if err != nil {
		t.Fatalf("selfCheck: %v", err)
	}
	if len(out.Knots) != 1 || out.Knots[0].Kind != ettlemesh.KindStaleAssumption {
		t.Fatalf("expected 1 stale-assumption knot, got %+v", out.Knots)
	}
	if f.selfCalls != 1 || f.votedCalls != 0 {
		t.Errorf("self-check should call ReconcileSelf only, got self=%d voted=%d", f.selfCalls, f.votedCalls)
	}
	// Stateless: it must NOT touch the shared horizon.
	if envs := s.h.snapshot(); len(envs) != 0 {
		t.Errorf("self-check should not store to the horizon, got %d participants", len(envs))
	}
}

// TestMCPRoundTripInMemory drives the REAL MCP server (tool registration +
// transport + In/Out marshaling) over an in-memory transport pair, with the fake
// reconciler — proving the wiring end-to-end without a key or stdio. This is the
// free equivalent of the optional live Claude-Code smoke.
func TestMCPRoundTripInMemory(t *testing.T) {
	f := &fakeReconciler{
		atoms: []ettlemesh.Atom{{Typ: ettlemesh.Dependency, Subject: "cache", Confidence: 1}},
		voted: []ettlemesh.Knot{{Kind: ettlemesh.KindCollision, Parties: []string{"alice", "bob"}, Confidence: 0.6}},
	}
	srv := newMCPServer(&server{det: f, h: newHorizon()}, "test")
	clientT, serverT := mcp.NewInMemoryTransports()
	ctx := context.Background()

	ss, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer ss.Close()

	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer cs.Close()

	// tools/list registers all three.
	lt, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	got := map[string]bool{}
	for _, tool := range lt.Tools {
		got[tool.Name] = true
	}
	for _, want := range []string{"ettle_emit", "ettle_horizon", "ettle_self_check"} {
		if !got[want] {
			t.Errorf("tools/list missing %q (got %v)", want, got)
		}
	}

	// emit → the typed Out round-trips as structured content.
	er, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "ettle_emit", Arguments: map[string]any{"participant": "alice", "notes": "n"}})
	if err != nil {
		t.Fatalf("CallTool emit: %v", err)
	}
	if er.IsError {
		t.Fatalf("emit returned tool error: %+v", er.Content)
	}
	var eo emitOut
	decodeStructured(t, er, &eo)
	if eo.Count != 1 {
		t.Errorf("emit structured count = %d, want 1", eo.Count)
	}

	// horizon → the seeded knot comes back, firm.
	hr, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "ettle_horizon", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool horizon: %v", err)
	}
	var ho horizonOut
	decodeStructured(t, hr, &ho)
	if len(ho.Firm) != 1 || ho.Firm[0].Kind != ettlemesh.KindCollision {
		t.Errorf("horizon firm = %+v, want 1 collision", ho.Firm)
	}
}

func decodeStructured(t *testing.T, res *mcp.CallToolResult, v any) {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured: %v", err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("unmarshal structured into %T: %v", v, err)
	}
}

func TestConcurrentEmitIsRaceFree(t *testing.T) {
	f := &fakeReconciler{atoms: []ettlemesh.Atom{{Typ: ettlemesh.Commitment, Subject: "x", Confidence: 1}}}
	s := newServerWith(f)
	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, _ = s.emit(context.Background(), nil, emitIn{Participant: fmt.Sprintf("p%d", i), Notes: "n"})
		}(i)
	}
	wg.Wait()
	if envs := s.h.snapshot(); len(envs) != n {
		t.Errorf("expected %d distinct participants after concurrent emit, got %d", n, len(envs))
	}
}
