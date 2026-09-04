package main

import (
	"context"
	"testing"
	"time"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"os"
	"path/filepath"
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
	t.Chdir(t.TempDir()) // no project pointer above us, so the fallbacks are the only input

	if got := captureIdentity("bob", "anyroom", ""); got != "bob" {
		t.Errorf("explicit --me should win: got %q, want bob", got)
	}
	if got := captureIdentity("", "", ""); got != "zoe" {
		t.Errorf("no me/room should fall back to $USER: got %q, want zoe", got)
	}
	// A configured room's agent is the default identity when --me is empty.
	if err := saveRoom(roomConfig{Name: "crew", RepoDir: "/tmp/x", Agent: "carol"}); err != nil {
		t.Fatal(err)
	}
	if got := captureIdentity("", "crew", ""); got != "carol" {
		t.Errorf("empty me + known room should use the room's agent: got %q, want carol", got)
	}
	// A Linear room stores no agent, so `ettle init`'s saved identity is what answers.
	if err := saveIdentity("linear://crew", "dana"); err != nil {
		t.Fatal(err)
	}
	if got := captureIdentity("", "", "linear://crew"); got != "dana" {
		t.Errorf("a Linear room should use the saved identity: got %q, want dana", got)
	}
}

func TestDueForCaptureDebounces(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	target := "room-x"
	const sess = "/t/room-x-session.jsonl"
	due, err := dueForCapture(target, sess, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !due {
		t.Fatal("first call should be due")
	}
	due, err = dueForCapture(target, sess, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if due {
		t.Fatal("second call inside the window should be debounced")
	}
	if due, _ = dueForCapture(target, sess, 0); !due {
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
	if due, _ := dueForCapture(room, "/t/sess-a.jsonl", time.Hour); !due {
		t.Fatal("capture should still be due despite a recent pull on the same room")
	}
}

// Several sessions run in parallel here as a matter of course — five worktrees, one
// room. Keying the debounce on the room alone made four of them silently skip their
// capture: distinct work, dropped because a sibling captured moments earlier, with no
// error and only a thin bus to show for it.
func TestCaptureDebouncesPerSessionNotPerRoom(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const room = "linear://dumpling"

	if due, _ := dueForCapture(room, "/t/session-a.jsonl", time.Hour); !due {
		t.Fatal("the first session should be due")
	}
	// A sibling session, same room, immediately after. It has its own atoms to
	// publish and must not be suppressed by the first.
	if due, _ := dueForCapture(room, "/t/session-b.jsonl", time.Hour); !due {
		t.Error("a parallel session was silently skipped — its atoms would never reach the bus")
	}
	// Within ONE session, Stop firing every turn must still collapse. That is what
	// the debounce was for, and it has to keep working.
	if due, _ := dueForCapture(room, "/t/session-a.jsonl", time.Hour); due {
		t.Error("the same session should still debounce, or a long session pays per turn")
	}
}

// Incremental capture: a long session must not be re-digested in full every couple of
// minutes, and the offset must never run ahead of a successful publish.
func TestCaptureOffsetRoundTripsPerTranscript(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const room = "linear://dumpling"

	if got := captureOffset(room, "", "/t/a.jsonl"); got != 0 {
		t.Fatalf("an unseen transcript must read from the start, got %d", got)
	}
	saveCaptureOffsets(room, "", map[string]int{"/t/a.jsonl": 120, "/t/b.jsonl": 7})

	if got := captureOffset(room, "", "/t/a.jsonl"); got != 120 {
		t.Errorf("offset should round-trip, got %d", got)
	}
	// Parallel sessions each keep their own place, exactly as the debounce now does.
	if got := captureOffset(room, "", "/t/b.jsonl"); got != 7 {
		t.Errorf("a sibling session has its own offset, got %d", got)
	}
	if got := captureOffset("linear://other", "", "/t/a.jsonl"); got != 0 {
		t.Errorf("a different room must not inherit an offset, got %d", got)
	}
}

func TestCaptureOffsetFallsBackToZeroOnJunk(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	path, err := captureOffsetPath("linear://dumpling", "", "/t/a.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	// Re-distilling costs money; skipping turns loses work. Only one is recoverable,
	// so anything unreadable means start from the beginning.
	for _, junk := range []string{"", "  ", "not-a-number", "-4"} {
		if err := os.WriteFile(path, []byte(junk), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := captureOffset("linear://dumpling", "", "/t/a.jsonl"); got != 0 {
			t.Errorf("junk offset %q should fall back to 0, got %d", junk, got)
		}
	}
}
