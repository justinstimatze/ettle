package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/transport"
)

func TestLoadParticipants(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	withHeader := write("zeb.md", "name: Alice\nrole: user-service\n\nrenaming GetUser today\n")
	noHeader := write("bob.md", "just some working notes\n")

	people, err := loadParticipants([]string{withHeader, noHeader})
	if err != nil {
		t.Fatal(err)
	}
	if len(people) != 2 {
		t.Fatalf("got %d participants, want 2", len(people))
	}
	// Sorted by name: "Alice" < "bob" (uppercase sorts first).
	if people[0].Name != "Alice" || people[0].Role != "user-service" || people[0].Notes != "renaming GetUser today" {
		t.Errorf("header parse: %+v", people[0])
	}
	// No header → filename (sans ext) is the name, role empty, whole file is the note.
	if people[1].Name != "bob" || people[1].Role != "" || people[1].Notes != "just some working notes" {
		t.Errorf("headerless parse: %+v", people[1])
	}
}

func TestHasParticipant(t *testing.T) {
	people := []participant{{Name: "alice"}, {Name: "Bob"}}
	if !hasParticipant(people, "ALICE") {
		t.Error("hasParticipant should be case-insensitive")
	}
	if hasParticipant(people, "alie") {
		t.Error("typo'd name must not match — that's the false-all-clear guard")
	}
}

func TestPartyOf(t *testing.T) {
	k := ettlemesh.Knot{Parties: []string{"alice", " bob "}}
	if !partyOf(k, "Bob") {
		t.Error("partyOf should trim + case-fold")
	}
	if partyOf(k, "carol") {
		t.Error("carol is not a party")
	}
}

func TestDistinctParticipants(t *testing.T) {
	envs := []transport.Envelope{
		{Participant: "alice"},
		{Participant: "Alice"}, // dup (case)
		{Participant: " bob "}, // dup-normalized distinct person
	}
	if got := distinctParticipants(envs); got != 2 {
		t.Errorf("got %d, want 2 distinct (alice, bob)", got)
	}
}
