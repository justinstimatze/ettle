package mcpserver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/justinstimatze/ettle/internal/crux"
	"github.com/justinstimatze/ettle/internal/ettlemesh"
)

// serverFor builds a horizon server whose reconcile returns exactly these tangles,
// with one participant already emitted so the horizon has atoms to run on.
func serverFor(t *testing.T, tangles ...ettlemesh.Tangle) *server {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	f := &fakeReconciler{
		atoms: []ettlemesh.Atom{{Typ: ettlemesh.Dependency, Subject: "pricing", Confidence: 1}},
		voted: tangles,
	}
	s := &server{det: f, h: newHorizon(), labels: &memLabelSink{}, stateKey: "test://crux", resolver: crux.Inline{}}
	if _, _, err := s.emit(context.Background(), nil, emitIn{Participant: "alice", Notes: "n"}); err != nil {
		t.Fatal(err)
	}
	return s
}

// A contested tangle must reach the session already framed as the choice, because
// the only thing the mesh is allowed to do with a values call is stage it.
func TestHorizonPreStagesContestedTangles(t *testing.T) {
	s := serverFor(t, ettlemesh.Tangle{
		Kind:       ettlemesh.KindTeamwideDivergence,
		Parties:    []string{"alice", "bob", "cleo"},
		About:      "the freeze timeline",
		Confidence: 0.6,
	})

	_, out, err := s.horizon(context.Background(), nil, horizonIn{})
	if err != nil {
		t.Fatalf("horizon: %v", err)
	}
	if len(out.Firm) != 1 {
		t.Fatalf("setup: expected one firm tangle, got %+v", out)
	}
	c := out.Firm[0].Crux
	if c == nil {
		t.Fatal("a firm team-wide divergence is a values call and must arrive pre-staged")
	}
	if c.Source != "inline" || c.Crux != "the freeze timeline" {
		t.Errorf("crux should name the contested point from the inline resolver, got %+v", c)
	}
	if len(c.Branches) != 2 {
		t.Errorf("an either/or needs two branches, got %v", c.Branches)
	}
	if !strings.Contains(c.Branches[0], "alice") {
		t.Errorf("the first branch frames it as the first party sees it, got %q", c.Branches[0])
	}
	// The note is what stops an agent treating a staged choice as a decision it may
	// take. It ships on every crux for that reason.
	if !strings.Contains(c.Note, "let the human") {
		t.Errorf("every crux must carry the do-not-decide instruction, got %q", c.Note)
	}
}

// Everything bindable stays bindable — staging a choice for a tangle that has one
// right answer would put friction exactly where it doesn't belong.
func TestHorizonLeavesBindableTanglesAlone(t *testing.T) {
	s := serverFor(t,
		ettlemesh.Tangle{Kind: ettlemesh.KindCollision, Parties: []string{"alice", "bob"}, About: "pricing deletion", Confidence: 0.9},
		ettlemesh.Tangle{Kind: ettlemesh.KindDuplication, Parties: []string{"alice", "bob"}, About: "discount rules", Confidence: 0.9},
	)
	_, out, err := s.horizon(context.Background(), nil, horizonIn{})
	if err != nil {
		t.Fatalf("horizon: %v", err)
	}
	for _, v := range out.Firm {
		if v.Crux != nil {
			t.Errorf("%s is bindable and must not be staged as a values call: %+v", v.Kind, v.Crux)
		}
	}
}

// A soft decision-rights tangle is below the bar: crux.Contested requires firm,
// because staging a choice off a guess is worse than not staging one.
func TestHorizonDoesNotStageASoftValuesCall(t *testing.T) {
	s := serverFor(t, ettlemesh.Tangle{
		Kind: ettlemesh.KindDecisionRights, Parties: []string{"alice", "bob"},
		About: "who owns the schema", Confidence: 0.3,
	})
	_, out, err := s.horizon(context.Background(), nil, horizonIn{})
	if err != nil {
		t.Fatalf("horizon: %v", err)
	}
	if len(out.Soft) != 1 {
		t.Fatalf("setup: expected one soft tangle, got %+v", out)
	}
	if out.Soft[0].Crux != nil {
		t.Errorf("a soft tangle is not confident enough to stage a choice from: %+v", out.Soft[0].Crux)
	}
}

// failingResolver stands in for an unreachable gemot.
type failingResolver struct{}

func (failingResolver) Resolve(context.Context, ettlemesh.Tangle, []ettlemesh.Atom) (*crux.Resolution, error) {
	return nil, errors.New("dial tcp: connection refused")
}

// A resolver that can't be reached must not take the rest of the horizon down with
// it, and the tangle still has to arrive — the humans' choice doesn't stop being
// theirs because a service was down.
func TestHorizonSurvivesAnUnreachableResolver(t *testing.T) {
	s := serverFor(t, ettlemesh.Tangle{
		Kind: ettlemesh.KindTeamwideDivergence, Parties: []string{"alice", "bob", "cleo"},
		About: "the freeze timeline", Confidence: 0.6,
	})
	s.resolver = failingResolver{}

	_, out, err := s.horizon(context.Background(), nil, horizonIn{})
	if err != nil {
		t.Fatalf("an unreachable resolver must not fail the whole horizon: %v", err)
	}
	if len(out.Firm) != 1 || out.Firm[0].Crux == nil {
		t.Fatalf("the tangle must still surface, got %+v", out.Firm)
	}
	c := out.Firm[0].Crux
	if c.Source != "unavailable" || !strings.Contains(c.Branches[0], "connection refused") {
		t.Errorf("the failure must be reported, not swallowed: %+v", c)
	}
}

// A nil resolver is the zero value a hand-built server has; it must still stage,
// because "no resolver configured" is not a reason to decide for someone.
func TestNilResolverFallsBackToInline(t *testing.T) {
	s := serverFor(t, ettlemesh.Tangle{
		Kind: ettlemesh.KindDecisionRights, Parties: []string{"alice", "bob"},
		About: "who owns the schema", Confidence: 0.9,
	})
	s.resolver = nil

	_, out, err := s.horizon(context.Background(), nil, horizonIn{})
	if err != nil {
		t.Fatalf("horizon: %v", err)
	}
	if len(out.Firm) != 1 || out.Firm[0].Crux == nil || out.Firm[0].Crux.Source != "inline" {
		t.Fatalf("nil must fall back to the infra-free resolver, got %+v", out.Firm)
	}
}
