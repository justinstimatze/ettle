package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/mcpserver"
	"github.com/justinstimatze/ettle/internal/tanglestate"
)

func votedT(kind string, votes, samples int, parties ...string) ettlemesh.Tangle {
	return ettlemesh.Tangle{Kind: kind, Parties: parties, About: "auth", Confidence: 0.9, Votes: votes, Samples: samples}
}

// The point of the whole file: a verdict typed at the shell should carry the
// recurrence it was answering. Without this the hooks-only install — the default one,
// and so the one that produces most verdicts — writes rows `ettle calibrate` counts
// and can never use.
func TestAShellVerdictCarriesTheRecurrenceItAnswered(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	log := filepath.Join(t.TempDir(), "labels.jsonl")
	t.Setenv("ETTLE_LABELS_PATH", log)
	const room = "linear://crew"

	k := votedT("collision", 4, 5, "alice", "bob")
	if err := writeSurfaced(room, horizonResult{firm: []ettlemesh.Tangle{k}}, nowForTest()); err != nil {
		t.Fatal(err)
	}
	if err := runConfirm([]string{"--transport", room, "--me", "alice", "collision", "alice", "bob"}); err != nil {
		t.Fatal(err)
	}

	var lbl mcpserver.Label
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(b))), &lbl); err != nil {
		t.Fatal(err)
	}
	if lbl.Votes != 4 || lbl.Samples != 5 {
		t.Errorf("the verdict should carry the surfaced recurrence, got votes=%d samples=%d", lbl.Votes, lbl.Samples)
	}
	if !lbl.Firm {
		t.Error("4 of 5 clears the default bar; the row should say it was shown firm")
	}
}

// A tangle that is not in the last horizon leaves recurrence at zero. An invented
// number is worse than a missing one, because `ettle calibrate` cannot tell a
// fabricated row from a real one and would report confidence it does not have.
func TestAVerdictOnSomethingNotSurfacedRecordsNoRecurrence(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	log := filepath.Join(t.TempDir(), "labels.jsonl")
	t.Setenv("ETTLE_LABELS_PATH", log)
	const room = "linear://crew"

	// The horizon showed a different tangle.
	if err := writeSurfaced(room, horizonResult{firm: []ettlemesh.Tangle{votedT("collision", 4, 5, "alice", "bob")}}, nowForTest()); err != nil {
		t.Fatal(err)
	}
	if err := runMute([]string{"--transport", room, "--me", "alice", "--wrong", "duplication", "alice", "carol"}); err != nil {
		t.Fatal(err)
	}

	var lbl mcpserver.Label
	b, _ := os.ReadFile(log)
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(b))), &lbl); err != nil {
		t.Fatal(err)
	}
	if lbl.Votes != 0 || lbl.Samples != 0 {
		t.Errorf("recurrence must not be invented for an unsurfaced tangle, got votes=%d samples=%d", lbl.Votes, lbl.Samples)
	}
	if lbl.Kind != "duplication" {
		t.Errorf("the kind still comes from the key, got %q", lbl.Kind)
	}
}

// Held-back tangles were never shown, so recording features for them would describe
// a surfacing that did not happen — the same reason no verdict about a floor-dropped
// tangle can exist at all.
func TestHeldTanglesAreNotRecordedAsSurfaced(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const room = "linear://crew"
	shown := votedT("collision", 4, 5, "alice", "bob")
	held := votedT("duplication", 1, 5, "alice", "carol")

	if err := writeSurfaced(room, horizonResult{firm: []ettlemesh.Tangle{shown}, held: []ettlemesh.Tangle{held}}, nowForTest()); err != nil {
		t.Fatal(err)
	}
	got := loadSurfaced(room)
	if _, ok := got[tanglestate.Key(shown.Kind, shown.Parties)]; !ok {
		t.Error("a shown tangle should be recorded")
	}
	if _, ok := got[tanglestate.Key(held.Kind, held.Parties)]; ok {
		t.Error("a held tangle was never shown and must not be recorded as surfaced")
	}
}

// Replaced, not merged. A tangle that stopped surfacing must not leave a feature
// behind for a later verdict to pick up and present as the recurrence it answered.
func TestEachHorizonReplacesTheSurfacedSet(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const room = "linear://crew"
	old := votedT("collision", 4, 5, "alice", "bob")
	fresh := votedT("duplication", 3, 5, "alice", "carol")

	if err := writeSurfaced(room, horizonResult{firm: []ettlemesh.Tangle{old}}, nowForTest()); err != nil {
		t.Fatal(err)
	}
	if err := writeSurfaced(room, horizonResult{firm: []ettlemesh.Tangle{fresh}}, nowForTest()); err != nil {
		t.Fatal(err)
	}
	got := loadSurfaced(room)
	if _, ok := got[tanglestate.Key(old.Kind, old.Parties)]; ok {
		t.Error("the previous horizon's tangle should be gone, not merged forward")
	}
	if len(got) != 1 {
		t.Errorf("the file describes one horizon, got %d entries", len(got))
	}
}

// A missing feature cache must never cost the human the thing they asked for. Muting
// a nuisance has to work on a cold cache; the row is just weaker.
func TestAColdCacheStillLetsTheVerdictLand(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("ETTLE_LABELS_PATH", filepath.Join(t.TempDir(), "labels.jsonl"))
	const room = "linear://crew"

	if err := runMute([]string{"--transport", room, "--wrong", "collision", "alice", "bob"}); err != nil {
		t.Fatalf("a cold feature cache must not fail the mute: %v", err)
	}
	muted, _ := tanglestate.Load(tanglestate.Muted, room)
	if !muted[tanglestate.Key("collision", []string{"alice", "bob"})] {
		t.Error("the mute should still have landed")
	}
}

// Features are a property of the reconcile, not of the reader: --me picks which
// tangles you see, not what their recurrence was. Two people on one room read the
// same numbers.
func TestFeaturesAreKeyedByRoomNotByPerson(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const mine, theirs = "linear://crew", "github://acme/widgets"
	k := votedT("collision", 4, 5, "alice", "bob")

	if err := writeSurfaced(mine, horizonResult{firm: []ettlemesh.Tangle{k}}, nowForTest()); err != nil {
		t.Fatal(err)
	}
	if got := loadSurfaced(mine); len(got) != 1 {
		t.Errorf("this room should have the features, got %+v", got)
	}
	if got := loadSurfaced(theirs); len(got) != 0 {
		t.Errorf("another room must not see them, got %+v", got)
	}
}
