package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/transport"
)

// fakeDistiller stands in for the LLM-backed Detector: one atom per non-empty
// reply, attributed to the author (as the real Distill force-sets From).
type fakeDistiller struct{}

func (fakeDistiller) Distill(_ context.Context, from, _, text string, _ []string) ([]ettlemesh.Atom, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	return []ettlemesh.Atom{{From: from, Typ: ettlemesh.Intent, Subject: text, Content: text, Confidence: 1}}, nil
}

// fakePuller returns canned replies and records the cursor it was asked for.
type fakePuller struct {
	replies  []transport.Reply
	cursor   string
	gotSince string
}

func (f *fakePuller) Pull(_ context.Context, since string) ([]transport.Reply, string, bool, error) {
	f.gotSince = since
	return f.replies, f.cursor, false, nil
}

// fakeBus records what pull publishes.
type fakeBus struct{ published []transport.Envelope }

func (f *fakeBus) Publish(_ context.Context, e transport.Envelope) error {
	f.published = append(f.published, e)
	return nil
}
func (f *fakeBus) Collect(_ context.Context) ([]transport.Envelope, error) { return f.published, nil }
func (f *fakeBus) Close() error                                            { return nil }

func TestPullRepliesDistillsAndPublishesPerAuthor(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // cursor writes to a temp config dir
	room := "testroom"
	puller := &fakePuller{
		replies: []transport.Reply{
			{Author: "Bob", Body: "use the shared cache", CreatedAt: "2026-08-05T00:40:00.000Z"},
			{Author: "Carol", Body: "I'll take the migration", CreatedAt: "2026-08-05T00:41:00.000Z"},
			{Author: "Bob", Body: "also bumping the timeout", CreatedAt: "2026-08-05T00:42:00.000Z"},
		},
		cursor: "2026-08-05T00:42:00.000Z",
	}
	bus := &fakeBus{}

	replies, authors, err := pullReplies(context.Background(), fakeDistiller{}, puller, bus, room)
	if err != nil {
		t.Fatal(err)
	}
	if replies != 3 || authors != 2 {
		t.Fatalf("replies=%d authors=%d, want 3/2", replies, authors)
	}
	if puller.gotSince != "" {
		t.Errorf("first pull asked since=%q, want empty (no cursor yet)", puller.gotSince)
	}
	if len(bus.published) != 2 {
		t.Fatalf("want one envelope per author (2), got %d", len(bus.published))
	}
	byWho := map[string][]ettlemesh.Atom{}
	for _, e := range bus.published {
		byWho[e.Participant] = e.Atoms
	}
	// Bob's two replies union into one envelope (Publish would otherwise overwrite).
	if len(byWho["Bob"]) != 2 {
		t.Errorf("Bob should carry both replies' atoms, got %d", len(byWho["Bob"]))
	}
	if len(byWho["Carol"]) != 1 {
		t.Errorf("Carol atoms = %d, want 1", len(byWho["Carol"]))
	}
	// Atoms are attributed to the teammate, not to ettle.
	if len(byWho["Bob"]) > 0 && byWho["Bob"][0].From != "Bob" {
		t.Errorf("atom not attributed to Bob: %+v", byWho["Bob"][0])
	}
	// Cursor persisted to the newest activity so the next pull is incremental.
	got, err := loadPullCursor(room)
	if err != nil {
		t.Fatal(err)
	}
	if got != "2026-08-05T00:42:00.000Z" {
		t.Errorf("saved cursor = %q, want the newest createdAt", got)
	}
}

func TestDueForPullDebounces(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	room := "r"
	// First call is due and records "now".
	due, err := dueForPull(room, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !due {
		t.Fatal("first call should be due")
	}
	// A second call inside the window is debounced.
	due, err = dueForPull(room, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if due {
		t.Fatal("second call inside the window should be debounced")
	}
	// A zero window means every call is due again.
	due, _ = dueForPull(room, 0)
	if !due {
		t.Fatal("zero window should always be due")
	}
}

func TestPullRepliesNoNewReplies(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	puller := &fakePuller{replies: nil, cursor: ""}
	bus := &fakeBus{}
	replies, authors, err := pullReplies(context.Background(), fakeDistiller{}, puller, bus, "r")
	if err != nil {
		t.Fatal(err)
	}
	if replies != 0 || authors != 0 || len(bus.published) != 0 {
		t.Fatalf("expected nothing ingested/published, got replies=%d authors=%d pub=%d", replies, authors, len(bus.published))
	}
}
