package mcpserver

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/transport"
)

// emit publishes one participant's atoms straight to the bus, skipping the
// distiller — these tools make no model call, and the tests shouldn't either.
func emit(t *testing.T, s *server, who, role string, at time.Time, atoms ...ettlemesh.Atom) {
	t.Helper()
	env := transport.Envelope{Participant: who, Role: role, EmittedAt: at.Format(time.RFC3339), Atoms: atoms}
	if err := s.h.upsert(context.Background(), env); err != nil {
		t.Fatalf("upsert %s: %v", who, err)
	}
}

// The mirror's whole point is the belief a teammate still holds after the subject
// moved on, so the test has to cross a round boundary: seed the bus, force the
// baseline read, then re-emit changed atoms.
func TestMirrorFlagsStaleBeliefsHeldAboutYou(t *testing.T) {
	s := newServerWith(&fakeReconciler{})
	ctx := context.Background()
	now := time.Now().UTC()

	emit(t, s, "ivo", "orders", now,
		ettlemesh.Atom{From: "ivo", Typ: ettlemesh.Dependency, Subject: "pricing", Content: "calls pricing in-process", Confidence: 1})
	emit(t, s, "mara", "pricing", now,
		ettlemesh.Atom{From: "mara", Typ: ettlemesh.Intent, Subject: "extract", Content: "pulling pricing into a service", Confidence: 1})

	// Round 1: this read is what fixes the baseline every model is seeded from.
	if _, err := s.h.snapshot(ctx); err != nil {
		t.Fatalf("baseline snapshot: %v", err)
	}

	// Round 2: ivo's dependency changed. Mara's model of ivo still holds the old one.
	emit(t, s, "ivo", "orders", now,
		ettlemesh.Atom{From: "ivo", Typ: ettlemesh.Dependency, Subject: "pricing", Content: "calls pricing over HTTP now", Confidence: 1})

	_, out, err := s.mirror(ctx, nil, mirrorIn{Me: "ivo"})
	if err != nil {
		t.Fatalf("mirror: %v", err)
	}
	if out.Observed != 1 {
		t.Fatalf("mara should hold a model of ivo, teammates_modeling_you = %d", out.Observed)
	}
	if out.Stale != 1 {
		t.Fatalf("the changed dependency should read stale, got %d stale of %+v", out.Stale, out.Beliefs)
	}
	b := out.Beliefs[0]
	if b.Stale != "drifted" {
		t.Errorf("a belief with a current counterpart is drifted, not hedged: %+v", b)
	}
	if !strings.Contains(b.Content, "in-process") || !strings.Contains(b.NowHold, "HTTP") {
		t.Errorf("mirror must show both sides — believed %q vs now-held %q", b.Content, b.NowHold)
	}
	// Coarsened by default: naming the holder turns the mirror into a surveillance
	// surface pointed at the modelers, which is the thing it exists to undo.
	if !out.Coarse || b.Observer != "" {
		t.Errorf("attribution must be withheld unless asked for: coarse=%v observer=%q", out.Coarse, b.Observer)
	}
	if !strings.Contains(out.Contest, "contesting is not") {
		t.Errorf("every mirror result must carry the limit that a belief can't be disputed yet: %q", out.Contest)
	}

	_, attr, err := s.mirror(ctx, nil, mirrorIn{Me: "ivo", ByObserver: true})
	if err != nil {
		t.Fatalf("mirror --by-observer: %v", err)
	}
	if len(attr.Beliefs) == 0 || attr.Beliefs[0].Observer != "mara" {
		t.Errorf("by_observer must name who holds the belief, got %+v", attr.Beliefs)
	}
}

// An empty bus must say so rather than report a clean mirror — "nobody believes
// anything stale about you" and "nobody models you at all" are different answers.
func TestMirrorEmptyRoomIsNotACleanBill(t *testing.T) {
	s := newServerWith(&fakeReconciler{})
	res, out, err := s.mirror(context.Background(), nil, mirrorIn{Me: "ivo"})
	if err != nil {
		t.Fatalf("mirror: %v", err)
	}
	if out.Observed != 0 || len(out.Beliefs) != 0 {
		t.Fatalf("nothing on the bus should mean no models, got %+v", out)
	}
	if !strings.Contains(renderedText(t, res), "nobody holds a model") {
		t.Errorf("the empty case must be named, not rendered as zero stale beliefs")
	}
}

// renderedText pulls the human-facing line out of a tool result.
func renderedText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if res == nil || len(res.Content) == 0 {
		t.Fatal("tool returned no content")
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", res.Content[0])
	}
	return tc.Text
}

func TestMirrorRequiresSubject(t *testing.T) {
	if _, _, err := s0().mirror(context.Background(), nil, mirrorIn{}); err == nil {
		t.Fatal("mirror without `me` has no subject to be about and must error")
	}
}

func s0() *server { return newServerWith(&fakeReconciler{}) }

// drift is the emit side of the same two rounds: only the teammates whose model
// went stale get the delta, and the savings against a broadcast is the number the
// layer exists to produce.
func TestDriftRoutesOnlyToStaleModels(t *testing.T) {
	s := newServerWith(&fakeReconciler{})
	ctx := context.Background()
	now := time.Now().UTC()

	for _, who := range []string{"ivo", "mara", "priya"} {
		emit(t, s, who, "", now,
			ettlemesh.Atom{From: who, Typ: ettlemesh.Intent, Subject: who + "-work", Content: "starting " + who, Confidence: 1})
	}
	if _, err := s.h.snapshot(ctx); err != nil {
		t.Fatalf("baseline snapshot: %v", err)
	}
	emit(t, s, "ivo", "", now,
		ettlemesh.Atom{From: "ivo", Typ: ettlemesh.Intent, Subject: "ivo-work", Content: "switched to the HTTP client", Confidence: 1})

	_, out, err := s.drift(ctx, nil, driftIn{})
	if err != nil {
		t.Fatalf("drift: %v", err)
	}
	if len(out.Deltas) != 1 || out.Deltas[0].Subject != "ivo" {
		t.Fatalf("only ivo changed; deltas = %+v", out.Deltas)
	}
	if got := strings.Join(out.Deltas[0].Observers, ","); got != "mara,priya" {
		t.Errorf("the delta routes to whoever's model went stale, got %q", got)
	}
	if out.Routed != 2 || out.Possible != 2 {
		t.Errorf("routed=%d broadcast=%d — with one changed subject and three people they coincide", out.Routed, out.Possible)
	}

	_, mine, err := s.drift(ctx, nil, driftIn{Me: "priya"})
	if err != nil {
		t.Fatalf("drift --me: %v", err)
	}
	if len(mine.Deltas) != 1 || len(mine.Deltas[0].Observers) != 1 || mine.Deltas[0].Observers[0] != "priya" {
		t.Errorf("--me must narrow to what involves that person, got %+v", mine.Deltas)
	}
}

// Presence is a read off the bus: sorted, typed, and coarse about time.
func TestRoomStatusIsPresenceNotSurveillance(t *testing.T) {
	s := newServerWith(&fakeReconciler{})
	ctx := context.Background()
	now := time.Now().UTC()

	emit(t, s, "mara", "pricing", now.Add(-3*24*time.Hour),
		ettlemesh.Atom{From: "mara", Typ: ettlemesh.Commitment, Subject: "delete", Content: "deleting the pricing package", Confidence: 1})
	emit(t, s, "ivo", "orders", now.Add(-30*time.Minute),
		ettlemesh.Atom{From: "ivo", Typ: ettlemesh.Intent, Subject: "engine", Content: "building the discount engine", Confidence: 1},
		ettlemesh.Atom{From: "ivo", Typ: ettlemesh.Dependency, Subject: "pricing", Content: "calls pricing in-process", Confidence: 1})

	_, out, err := s.roomStatus(ctx, nil, roomStatusIn{})
	if err != nil {
		t.Fatalf("roomStatus: %v", err)
	}
	if out.Present != 2 || out.Members[0].Participant != "ivo" {
		t.Fatalf("two present, sorted, got %+v", out)
	}
	ivo, mara := out.Members[0], out.Members[1]
	if ivo.Freshness != "recently" || mara.Freshness != "3d ago" {
		t.Errorf("freshness buckets wrong: ivo=%q mara=%q", ivo.Freshness, mara.Freshness)
	}
	// A minute-resolution age across a team is a working-patterns feed — who starts
	// at six, who was up at three. The bucket is the privacy decision.
	if strings.Contains(ivo.Freshness, "30m") || strings.Contains(ivo.Freshness, "active") {
		t.Errorf("presence must not leak when someone works: %q", ivo.Freshness)
	}
	if len(ivo.WorkingOn) != 1 || len(ivo.DependsOn) != 1 || len(mara.CommittedTo) != 1 {
		t.Errorf("atoms must land in their typed buckets: %+v / %+v", ivo, mara)
	}
	if ivo.Role != "orders" {
		t.Errorf("role should ride along, got %q", ivo.Role)
	}
}

// `clear` is the undo, and the reason it is a separate verdict rather than a
// fourth judgment: un-muting says nothing about whether the detector was right,
// so it must not enter the calibration log as if it did.
func TestRespondClearUnmutesAndWritesNoLabel(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	sink := &memLabelSink{}
	s := collisionServer(t)
	s.labels = sink
	ctx := context.Background()

	_, ho, _ := s.horizon(ctx, nil, horizonIn{})
	if len(ho.Firm) != 1 {
		t.Fatalf("setup: expected the collision surfaced, got %+v", ho)
	}
	key := ho.Firm[0].Key

	if _, _, err := s.respond(ctx, nil, respondIn{Me: "alice", Tangle: key, Verdict: "not_real"}); err != nil {
		t.Fatalf("respond not_real: %v", err)
	}
	if _, ho2, _ := s.horizon(ctx, nil, horizonIn{}); len(ho2.Firm) != 0 {
		t.Fatalf("a not_real tangle should be muted, still see %+v", ho2.Firm)
	}

	_, out, err := s.respond(ctx, nil, respondIn{Me: "alice", Tangle: key, Verdict: "clear"})
	if err != nil {
		t.Fatalf("respond clear: %v", err)
	}
	if !out.Recorded {
		t.Error("clearing an existing mute should report that it did something")
	}
	if _, ho3, _ := s.horizon(ctx, nil, horizonIn{}); len(ho3.Firm) != 1 {
		t.Fatalf("clear must let the tangle surface again, got %+v", ho3.Firm)
	}
	if len(sink.got) != 1 || sink.got[0].Verdict != "not_real" {
		t.Errorf("clear is not ground truth and must not be logged: %+v", sink.got)
	}

	// Clearing something that was never muted is a no-op, reported as one.
	_, none, err := s.respond(ctx, nil, respondIn{Me: "alice", Tangle: "collision|nobody+else", Verdict: "clear"})
	if err != nil {
		t.Fatalf("respond clear on an unmuted key: %v", err)
	}
	if none.Recorded {
		t.Error("clearing an unmuted tangle removed nothing and should say so")
	}
}
