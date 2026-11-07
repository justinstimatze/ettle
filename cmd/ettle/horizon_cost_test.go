package main

import (
	"testing"
	"time"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/transport"
)

func TestDropStaleLeavesOutOldEnvelopesAndKeepsUndated(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	envs := []transport.Envelope{
		{Participant: "fresh", EmittedAt: now.Add(-24 * time.Hour).Format(time.RFC3339)},
		{Participant: "old", EmittedAt: now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)},
		{Participant: "undated"},
	}
	got := dropStale(envs, 14*24*time.Hour, now)
	if len(got) != 2 || got[0].Participant != "fresh" || got[1].Participant != "undated" {
		t.Fatalf("want fresh+undated, got %+v", got)
	}
	if n := len(dropStale(envs, 0, now)); n != 3 {
		t.Errorf("after=0 must keep everyone, kept %d", n)
	}
}

func TestCapAtomsKeepsTheTailAndLeavesTheInputAlone(t *testing.T) {
	atoms := make([]ettlemesh.Atom, 5)
	for i := range atoms {
		atoms[i].Subject = string(rune('a' + i))
	}
	envs := []transport.Envelope{{Participant: "me", Atoms: atoms}}
	got := capAtoms(envs, 2)
	if len(got[0].Atoms) != 2 || got[0].Atoms[0].Subject != "d" || got[0].Atoms[1].Subject != "e" {
		t.Fatalf("want the last two atoms, got %+v", got[0].Atoms)
	}
	if len(envs[0].Atoms) != 5 {
		t.Errorf("capAtoms must not shorten the caller's envelope")
	}
}
