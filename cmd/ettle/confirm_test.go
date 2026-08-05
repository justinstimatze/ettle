package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/tanglestate"
)

// A fixed clock, so a rendered block is compared for content and never for when the
// test happened to run.
func nowForTest() time.Time { return time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC) }

// Confirming is the positive half of muting, and the payoff has to be real: the
// tangle stays on the horizon because it is a live conflict, and stops being asked
// about. A confirmation that dropped it would be muting under another name.
func TestConfirmKeepsTheTangleSurfacedAndStopsTheAsk(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const room = "linear://crew"
	k := firmT("collision", "alice", "bob")
	key := tanglestate.Key(k.Kind, k.Parties)

	if err := runConfirm([]string{"--transport", room, "collision", "alice", "bob"}); err != nil {
		t.Fatal(err)
	}
	confirmed, err := tanglestate.Load(tanglestate.Confirmed, room)
	if err != nil {
		t.Fatal(err)
	}
	if !confirmed[key] {
		t.Fatalf("the confirmation should be in the shared store: %+v", confirmed)
	}

	res := tagHorizon(horizonResult{firm: []ettlemesh.Tangle{k}}, nil, confirmed, nil)
	if len(res.firm) != 1 {
		t.Fatalf("a confirmed tangle is live and must stay surfaced, got %+v", res.firm)
	}
	if res.unanswered() {
		t.Error("every surfaced tangle is answered; the block should not ask again")
	}
	block := renderHorizonBlock(res, "alice", nowForTest())
	if !strings.Contains(block, "you confirmed this") {
		t.Errorf("a confirmed line should be marked:\n%s", block)
	}
	if strings.Contains(block, "Tell ettle what these are") {
		t.Errorf("the verdict ask should be gone once everything is answered:\n%s", block)
	}
}

// One answered tangle must not silence the ask for the others, or confirming the
// first thing you see buys quiet on everything behind it.
func TestTheAskSurvivesWhileAnyTangleIsUnanswered(t *testing.T) {
	answered := firmT("collision", "alice", "bob")
	open := firmT("duplication", "alice", "carol")
	confirmed := map[string]bool{tanglestate.Key(answered.Kind, answered.Parties): true}

	res := tagHorizon(horizonResult{firm: []ettlemesh.Tangle{answered, open}}, nil, confirmed, nil)
	if !res.unanswered() {
		t.Fatal("one tangle still has no verdict; the ask must stand")
	}
	block := renderHorizonBlock(res, "alice", nowForTest())
	if !strings.Contains(block, "Tell ettle what these are") {
		t.Errorf("the ask should print while anything is unanswered:\n%s", block)
	}
}

// The ask names all three verdicts. Inviting only the two negative ones is what
// biased the verdict log — see internal/tanglestate (Confirmed) and `ettle calibrate`.
func TestTheAskOffersRealAlongsideTheNegativeVerdicts(t *testing.T) {
	res := horizonResult{firm: []ettlemesh.Tangle{firmT("collision", "alice", "bob")}}
	block := renderHorizonBlock(res, "alice", nowForTest())
	for _, want := range []string{"`real`", "`not_real`", "`handled`", "ettle confirm"} {
		if !strings.Contains(block, want) {
			t.Errorf("the ask should offer %s:\n%s", want, block)
		}
	}
}

// Confirming writes the same label ettle_respond would, into the same log, or the
// calibration data splits by which surface someone happened to reach for.
func TestConfirmRecordsTheRealVerdictInTheSharedLog(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	log := filepath.Join(t.TempDir(), "labels.jsonl")
	t.Setenv("ETTLE_LABELS_PATH", log)

	if err := runConfirm([]string{"--transport", "linear://crew", "--me", "alice", "collision", "alice", "bob"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("confirming should record a verdict: %v", err)
	}
	line := string(b)
	if !strings.Contains(line, `"verdict":"real"`) {
		t.Errorf("expected a real verdict, got %s", line)
	}
	if !strings.Contains(line, `"kind":"collision"`) {
		t.Errorf("the kind should be recovered from the key, got %s", line)
	}
}

// Withdrawing is the way back, and it has to put the tangle back in the ask.
func TestClearWithdrawsTheConfirmationAndTheAskReturns(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const room = "linear://crew"
	if err := runConfirm([]string{"--transport", room, "collision", "alice", "bob"}); err != nil {
		t.Fatal(err)
	}
	if err := runConfirm([]string{"--transport", room, "--clear", "collision", "alice", "bob"}); err != nil {
		t.Fatal(err)
	}
	confirmed, _ := tanglestate.Load(tanglestate.Confirmed, room)
	if len(confirmed) != 0 {
		t.Fatalf("--clear should withdraw it: %+v", confirmed)
	}
	res := tagHorizon(horizonResult{firm: []ettlemesh.Tangle{firmT("collision", "alice", "bob")}}, nil, confirmed, nil)
	if !res.unanswered() {
		t.Error("a withdrawn confirmation puts the tangle back in the ask")
	}
}

// Confirm and mute are opposite answers to the same question and must not share a
// room's state: confirming one tangle cannot quietly unmute another.
func TestConfirmAndMuteAreSeparateStores(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("ETTLE_LABELS_PATH", filepath.Join(t.TempDir(), "labels.jsonl"))
	const room = "linear://crew"

	if err := runMute([]string{"--transport", room, "--wrong", "duplication", "alice", "carol"}); err != nil {
		t.Fatal(err)
	}
	if err := runConfirm([]string{"--transport", room, "collision", "alice", "bob"}); err != nil {
		t.Fatal(err)
	}
	muted, _ := tanglestate.Load(tanglestate.Muted, room)
	confirmed, _ := tanglestate.Load(tanglestate.Confirmed, room)
	if len(muted) != 1 || len(confirmed) != 1 {
		t.Fatalf("each store holds its own answer: muted=%+v confirmed=%+v", muted, confirmed)
	}
	if muted[tanglestate.Key("collision", []string{"alice", "bob"})] {
		t.Error("confirming must not mute")
	}
	if confirmed[tanglestate.Key("duplication", []string{"alice", "carol"})] {
		t.Error("muting must not confirm")
	}
}
