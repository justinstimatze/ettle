package mcpserver

import (
	"context"
	"testing"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/tanglestate"
)

// A `real` verdict has to change something the human can observe, or nobody records
// one and the verdict log arrives with only its negative arm — the bias `ettle
// calibrate` reports and cannot correct for. The observable change is the
// confirmation: the tangle stays surfaced (it's a live conflict) and stops being
// asked about.
func TestRespondRealConfirmsWithoutMuting(t *testing.T) {
	ctx := context.Background()
	k := ettlemesh.Tangle{Kind: ettlemesh.KindCollision, Parties: []string{"alice", "bob"}, About: "auth", Confidence: 0.9, Votes: 5, Samples: 5}
	s := serverFor(t, k)
	key := tanglestate.Key(k.Kind, k.Parties)

	_, out, err := s.respond(ctx, nil, respondIn{Me: "alice", Tangle: key, Verdict: "real"})
	if err != nil {
		t.Fatalf("respond real: %v", err)
	}
	if !out.Recorded || out.Verdict != "real" {
		t.Fatalf("expected a recorded real verdict, got %+v", out)
	}

	confirmed, err := tanglestate.Load(tanglestate.Confirmed, s.stateKey)
	if err != nil {
		t.Fatal(err)
	}
	if !confirmed[key] {
		t.Errorf("a real verdict should confirm the tangle: %+v", confirmed)
	}
	muted, _ := tanglestate.Load(tanglestate.Muted, s.stateKey)
	if muted[key] {
		t.Error("confirming must not mute — the conflict is live and the human should keep seeing it")
	}
	if _, ho, _ := s.horizon(ctx, nil, horizonIn{}); len(ho.Firm) == 0 {
		t.Error("a confirmed tangle stays on the horizon")
	}
}

// The verdict also lands in the label log, with the recurrence features the bar
// thresholds on — which is the whole reason to want `real` verdicts at all.
func TestRespondRealIsLearnable(t *testing.T) {
	k := ettlemesh.Tangle{Kind: ettlemesh.KindCollision, Parties: []string{"alice", "bob"}, About: "auth", Confidence: 0.9, Votes: 4, Samples: 5}
	ctx := context.Background()
	s := serverFor(t, k)
	sink := s.labels.(*memLabelSink)

	// Read the horizon first: recurrence rides along only when the verdict answers a
	// tangle THIS server surfaced, which is the common path and the only one that
	// holds the features. Answering cold records the kind and leaves recurrence zero
	// rather than inventing it.
	if _, _, err := s.horizon(ctx, nil, horizonIn{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.respond(ctx, nil, respondIn{Me: "alice", Tangle: tanglestate.Key(k.Kind, k.Parties), Verdict: "real"}); err != nil {
		t.Fatal(err)
	}
	if len(sink.got) != 1 {
		t.Fatalf("one verdict, one row: %+v", sink.got)
	}
	got := sink.got[0]
	if got.Verdict != "real" {
		t.Errorf("verdict = %q, want real", got.Verdict)
	}
	if got.Samples != 5 || got.Votes != 4 {
		t.Errorf("the surfaced recurrence should ride along, got votes=%d samples=%d", got.Votes, got.Samples)
	}
}

// Clear is the undo, and there are two things to undo now. A human saying "I answered
// that by mistake" means the answer, not the store it happened to land in.
func TestClearWithdrawsAConfirmationToo(t *testing.T) {
	ctx := context.Background()
	k := ettlemesh.Tangle{Kind: ettlemesh.KindCollision, Parties: []string{"alice", "bob"}, About: "auth", Confidence: 0.9, Votes: 5, Samples: 5}
	s := serverFor(t, k)
	key := tanglestate.Key(k.Kind, k.Parties)

	if _, _, err := s.respond(ctx, nil, respondIn{Me: "alice", Tangle: key, Verdict: "real"}); err != nil {
		t.Fatal(err)
	}
	_, out, err := s.respond(ctx, nil, respondIn{Me: "alice", Tangle: key, Verdict: "clear"})
	if err != nil {
		t.Fatalf("respond clear: %v", err)
	}
	if !out.Recorded {
		t.Error("clearing an existing confirmation should report that it did something")
	}
	confirmed, _ := tanglestate.Load(tanglestate.Confirmed, s.stateKey)
	if confirmed[key] {
		t.Errorf("clear should withdraw the confirmation: %+v", confirmed)
	}

	// And clear still writes no label: "I answered by mistake" is not ground truth
	// about whether the detector was right.
	sink := s.labels.(*memLabelSink)
	for _, r := range sink.got {
		if r.Verdict == "clear" {
			t.Error("clear must not write a label")
		}
	}
}
