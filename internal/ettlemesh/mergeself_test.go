package ettlemesh

import "testing"

// Incremental capture publishes prev+new rather than replacing wholesale, so the
// merge is now the thing standing between a long session and a slowly corrupting bus.
func TestMergeSelfSupersedesARewordedBelief(t *testing.T) {
	prev := []Atom{atom("me", Intent, "the retry wrapper in the parser", "adding a retry")}
	// The same belief, phrased as a stochastic distiller phrases it the second time.
	next := []Atom{atom("me", Intent, "retry wrapper for the parser", "retry now catches timeouts only")}

	got := MergeSelf(prev, next)
	if len(got) != 1 {
		t.Fatalf("a reworded subject must supersede, not accumulate — got %d atoms: %+v", len(got), got)
	}
	if got[0].Content != "retry now catches timeouts only" {
		t.Errorf("the newer belief should win, got %q", got[0].Content)
	}
}

func TestMergeSelfKeepsGenuinelyDifferentBeliefs(t *testing.T) {
	prev := []Atom{atom("me", Intent, "the retry wrapper", "adding a retry")}
	next := []Atom{atom("me", Intent, "the Linear transport audience check", "auditing who can read the room")}

	got := MergeSelf(prev, next)
	if len(got) != 2 {
		t.Fatalf("two unrelated beliefs must both survive — got %d: %+v", len(got), got)
	}
}

func TestMergeSelfKeepsEarlierWorkTheNewSliceNeverMentions(t *testing.T) {
	// The whole point: distilling only the last few turns must not erase the morning.
	prev := []Atom{
		atom("me", Intent, "the wrong-workspace guard", "shipped it"),
		atom("me", Dependency, "the Linear member key", "scoped to one workspace"),
	}
	next := []Atom{atom("me", Intent, "incremental capture", "building it now")}

	got := MergeSelf(prev, next)
	if len(got) != 3 {
		t.Fatalf("earlier atoms must survive a partial capture — got %d: %+v", len(got), got)
	}
}

func TestMergeSelfDoesNotCrossAtomTypes(t *testing.T) {
	// Same words, different type, is a different claim — what you are working on is
	// not what you depend on.
	prev := []Atom{atom("me", Intent, "the retry wrapper", "adding a retry")}
	next := []Atom{atom("me", Dependency, "the retry wrapper", "waiting on it to land")}

	if got := MergeSelf(prev, next); len(got) != 2 {
		t.Errorf("a same-subject atom of another type must not supersede — got %d: %+v", len(got), got)
	}
}

func TestMergeSelfIsIdempotent(t *testing.T) {
	prev := []Atom{atom("me", Intent, "the retry wrapper", "adding a retry")}
	once := MergeSelf(prev, prev)
	twice := MergeSelf(once, prev)
	if len(once) != 1 || len(twice) != 1 {
		t.Errorf("re-merging the same atoms must not grow the model: %d then %d", len(once), len(twice))
	}
}
