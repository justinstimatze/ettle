package ettlemesh

import "testing"

func TestDistinctPeopleFoldsCaseAndSpace(t *testing.T) {
	cases := []struct {
		name    string
		parties []string
		want    int
	}{
		{"empty", nil, 0},
		{"single", []string{"alice"}, 1},
		{"dupes fold", []string{"alice", "Alice", " alice "}, 1},
		{"two distinct", []string{"alice", "bob"}, 2},
		{"three distinct", []string{"alice", "bob", "cleo"}, 3},
		{"mixed dupes", []string{"alice", "Bob", "alice", "bob", "cleo"}, 3},
	}
	for _, tc := range cases {
		if got := distinctPeople(tc.parties); got != tc.want {
			t.Errorf("%s: distinctPeople(%v) = %d, want %d", tc.name, tc.parties, got, tc.want)
		}
	}
}

func TestFilterTeamwideQuorumDropsUnderThree(t *testing.T) {
	in := []Tangle{
		{Kind: KindTeamwideDivergence, Parties: []string{"alice", "bob"}, About: "two-party fabrication"},
		{Kind: KindTeamwideDivergence, Parties: []string{"alice", "bob", "cleo"}, About: "genuine teamwide"},
		{Kind: KindTeamwideDivergence, Parties: []string{"alice", "Alice", "alice"}, About: "one person dressed as three"},
	}
	out := filterTeamwideQuorum(in)
	if len(out) != 1 {
		t.Fatalf("expected only the >=3-distinct tangle to survive, got %d: %+v", len(out), out)
	}
	if out[0].About != "genuine teamwide" {
		t.Errorf("kept the wrong tangle: %q", out[0].About)
	}
}

// The quorum gate is keyed on the teamwide kind only; other kinds pass through
// untouched even with two parties (a pairwise collision is supposed to have two).
func TestFilterTeamwideQuorumIgnoresOtherKinds(t *testing.T) {
	in := []Tangle{
		{Kind: KindCollision, Parties: []string{"alice", "bob"}, About: "real pairwise"},
		{Kind: KindStaleAssumption, Parties: []string{"alice"}, About: "self tangle"},
	}
	out := filterTeamwideQuorum(in)
	if len(out) != 2 {
		t.Fatalf("non-teamwide kinds must pass through, got %d: %+v", len(out), out)
	}
}

func TestFilterTeamwideQuorumDoesNotAliasInput(t *testing.T) {
	in := []Tangle{
		{Kind: KindTeamwideDivergence, Parties: []string{"alice", "bob"}, About: "dropped"},
		{Kind: KindTeamwideDivergence, Parties: []string{"alice", "bob", "cleo"}, About: "kept"},
	}
	_ = filterTeamwideQuorum(in)
	// The fresh-backing-array contract: filtering must not stomp the caller's slice.
	if in[0].About != "dropped" || in[1].About != "kept" {
		t.Errorf("input slice was mutated: %+v", in)
	}
}

func TestDistinctAuthorsFoldsDuplicates(t *testing.T) {
	atoms := []Atom{
		{From: "alice"}, {From: "Alice"}, {From: " alice "},
		{From: "bob"}, {From: "cleo"},
	}
	if got := distinctAuthors(atoms); got != 3 {
		t.Errorf("distinctAuthors = %d, want 3", got)
	}
	if got := distinctAuthors([]Atom{{From: "kit"}, {From: "sol"}}); got != 2 {
		t.Errorf("two-author corpus = %d, want 2 (below teamwide quorum)", got)
	}
}
