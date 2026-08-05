package main

import (
	"context"
	"testing"
	"time"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
)

// emptyDistiller distills everything to nothing — to prove a zero-atom session
// publishes NO envelope (an empty one would wipe the person's atoms off the bus).
type emptyDistiller struct{}

func (emptyDistiller) Distill(context.Context, string, string, string, []string) ([]ettlemesh.Atom, error) {
	return nil, nil
}

func TestPublishCaptureAttributesToMeAndPublishesOnce(t *testing.T) {
	bus := &fakeBus{}
	n, err := publishCapture(context.Background(), fakeDistiller{}, bus, "alice", "shipping the cache work today")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("atoms=%d, want 1", n)
	}
	if len(bus.published) != 1 {
		t.Fatalf("want one envelope, got %d", len(bus.published))
	}
	e := bus.published[0]
	if e.Participant != "alice" {
		t.Errorf("envelope participant = %q, want alice", e.Participant)
	}
	if len(e.Atoms) != 1 || e.Atoms[0].From != "alice" {
		t.Errorf("atom not attributed to alice: %+v", e.Atoms)
	}
}

func TestPublishCaptureZeroAtomsPublishesNothing(t *testing.T) {
	bus := &fakeBus{}
	// A non-empty note that distills to zero atoms must not publish — otherwise an
	// empty envelope replaces (erases) whatever the person already had on the bus.
	n, err := publishCapture(context.Background(), emptyDistiller{}, bus, "alice", "just chatting, no decisions")
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 || len(bus.published) != 0 {
		t.Fatalf("expected no publish, got n=%d pub=%d", n, len(bus.published))
	}
}

func TestPublishCaptureEmptyNoteNoPublish(t *testing.T) {
	bus := &fakeBus{}
	n, err := publishCapture(context.Background(), fakeDistiller{}, bus, "alice", "   ")
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 || len(bus.published) != 0 {
		t.Fatalf("blank note should publish nothing, got n=%d pub=%d", n, len(bus.published))
	}
}

func TestCaptureIdentity(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("USER", "zoe")

	if got := captureIdentity("bob", "anyroom"); got != "bob" {
		t.Errorf("explicit --me should win: got %q, want bob", got)
	}
	if got := captureIdentity("", ""); got != "zoe" {
		t.Errorf("no me/room should fall back to $USER: got %q, want zoe", got)
	}
	// A configured room's agent is the default identity when --me is empty.
	if err := saveRoom(roomConfig{Name: "crew", RepoDir: "/tmp/x", Agent: "carol"}); err != nil {
		t.Fatal(err)
	}
	if got := captureIdentity("", "crew"); got != "carol" {
		t.Errorf("empty me + known room should use the room's agent: got %q, want carol", got)
	}
}

func TestDueForCaptureDebounces(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	target := "room-x"
	due, err := dueForCapture(target, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !due {
		t.Fatal("first call should be due")
	}
	due, err = dueForCapture(target, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if due {
		t.Fatal("second call inside the window should be debounced")
	}
	if due, _ = dueForCapture(target, 0); !due {
		t.Fatal("zero window should always be due")
	}
}

// The capture marker is separate from pull's, so the two hooks debounce
// independently — a recent pull must not suppress a capture on the same room.
func TestCaptureAndPullDebounceIndependently(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	room := "shared"
	if due, _ := dueForPull(room, time.Hour); !due {
		t.Fatal("pull should be due first")
	}
	if due, _ := dueForCapture(room, time.Hour); !due {
		t.Fatal("capture should still be due despite a recent pull on the same room")
	}
}
