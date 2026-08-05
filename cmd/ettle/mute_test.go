package main

import (
	"testing"

	"github.com/justinstimatze/ettle/internal/tanglestate"
)

func TestMuteKeyAcceptsWhatTheHorizonShows(t *testing.T) {
	want := tanglestate.Key("duplication", []string{"ivo", "mara"})

	// The loose form: read straight off the horizon line, "duplication · ivo, mara".
	for _, args := range [][]string{
		{"duplication", "ivo", "mara"},
		{"duplication", "mara", "ivo"},  // order can't matter — Key sorts
		{"duplication", "ivo,", "mara"}, // a comma survives the copy/paste
		{"duplication", "ivo, mara"},    // or the whole party list as one argument
		{"Duplication", "Ivo", "MARA"},  // case is the human's, not the store's
		{"duplication|ivo+mara"},        // and the exact key still works
		{"duplication|mara+ivo"},
	} {
		if got := muteKey(args); got != want {
			t.Errorf("muteKey(%q) = %q, want %q", args, got, want)
		}
	}

	if got := muteKey(nil); got != "" {
		t.Errorf("no arguments should name no tangle, got %q", got)
	}
	if got := muteKey([]string{"  "}); got != "" {
		t.Errorf("whitespace should name no tangle, got %q", got)
	}
}

func TestMuteRoundTripsThroughTheSharedStore(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const room = "linear://crew"
	key := tanglestate.Key("collision", []string{"ivo", "mara"})

	if err := runMute([]string{"--transport", room, "collision", "ivo", "mara"}); err != nil {
		t.Fatal(err)
	}
	set, err := tanglestate.Load(tanglestate.Muted, room)
	if err != nil {
		t.Fatal(err)
	}
	if !set[key] {
		t.Fatalf("the muted tangle should be in the store both surfaces read: %+v", set)
	}

	// Reversible, or it's a trap: the person who silenced the wrong one needs a way back.
	if err := runMute([]string{"--transport", room, "--clear", "collision", "ivo", "mara"}); err != nil {
		t.Fatal(err)
	}
	set, _ = tanglestate.Load(tanglestate.Muted, room)
	if set[key] {
		t.Errorf("--clear should unmute: %+v", set)
	}

	// Clearing something that was never muted is a no-op, not an error.
	if err := runMute([]string{"--transport", room, "--clear", "collision", "ivo", "mara"}); err != nil {
		t.Errorf("unmuting nothing should not fail: %v", err)
	}
}

func TestMuteClearAllEmptiesOnlyThatRoom(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const mine, theirs = "linear://crew", "github://acme/widgets"
	for _, room := range []string{mine, theirs} {
		if err := runMute([]string{"--transport", room, "collision", "ivo", "mara"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := runMute([]string{"--transport", mine, "--clear"}); err != nil {
		t.Fatal(err)
	}
	if set, _ := tanglestate.Load(tanglestate.Muted, mine); len(set) != 0 {
		t.Errorf("--clear with no tangle should empty this room: %+v", set)
	}
	if set, _ := tanglestate.Load(tanglestate.Muted, theirs); len(set) != 1 {
		t.Errorf("and leave every other room alone: %+v", set)
	}
}

func TestMuteRefusesToGuessTheRoom(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir()) // no .ettle-room anywhere above
	if err := runMute([]string{"collision", "ivo", "mara"}); err == nil {
		t.Error("with no room to mute in, muting into a default bucket would silence the wrong room")
	}
}
