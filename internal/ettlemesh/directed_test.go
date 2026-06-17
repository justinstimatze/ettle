package ettlemesh

import "testing"

// atom is a tiny constructor so the L2 tests read as belief slots, not struct
// literals. Confidence defaults to 1.0 (stated) — the common case.
func atom(from string, typ AtomType, subject, content string) Atom {
	return Atom{From: from, Typ: typ, Subject: subject, Content: content, Confidence: 1.0}
}

func TestEmitDeltaNewAndChanged(t *testing.T) {
	prior := []Atom{
		atom("mara", Intent, "pricing-extract", "pulling pricing into a service"),
		atom("mara", Commitment, "delete package", "deleting the in-process package next week"),
	}
	current := []Atom{
		atom("mara", Intent, "pricing-extract", "pulling pricing into a service"),            // unchanged → withheld
		atom("mara", Commitment, "delete package", "deleting the in-process package MONDAY"), // changed → emit
		atom("mara", Dependency, "auth", "still relies on the shared auth lib"),              // new slot → emit
	}
	delta := EmitDelta(prior, current)
	if len(delta) != 2 {
		t.Fatalf("delta = %d atoms, want 2 (the changed commitment + the new dependency); got %+v", len(delta), delta)
	}
	// The unchanged intent must NOT be re-emitted (surprise-gated: no machine-speed re-broadcast).
	for _, a := range delta {
		if a.Typ == Intent {
			t.Errorf("unchanged intent was re-emitted: %+v", a)
		}
	}
}

func TestEmitDeltaIdenticalIsEmpty(t *testing.T) {
	a := []Atom{atom("ivo", Dependency, "pricing", "calls pricing in-process")}
	if d := EmitDelta(a, a); len(d) != 0 {
		t.Fatalf("identical self-model should emit nothing, got %+v", d)
	}
}

func TestEmitDeltaConfidenceChange(t *testing.T) {
	// Same slot, same content, but an inferred guess (0.4) the subject later stated
	// outright (1.0) — a real correction that must cross.
	prior := []Atom{{From: "ivo", Typ: Assumption, Subject: "deadline", Content: "shipping next week", Confidence: 0.4, Inferred: true}}
	current := []Atom{{From: "ivo", Typ: Assumption, Subject: "deadline", Content: "shipping next week", Confidence: 1.0}}
	if d := EmitDelta(prior, current); len(d) != 1 {
		t.Fatalf("a confidence change should emit, got %+v", d)
	}
}

func TestStaleBeliefsDriftedAndDropped(t *testing.T) {
	model := DirectedModel{
		Observer: "ivo", Subject: "mara",
		Beliefs: []Atom{
			atom("mara", Commitment, "delete package", "deleting the package next week"),
			atom("mara", Intent, "pricing-extract", "pulling pricing into a service"),
		},
	}
	// Mara now deletes MONDAY (drifted) and has dropped the extract intent entirely.
	current := []Atom{atom("mara", Commitment, "delete package", "deleting the package MONDAY")}
	drifts := StaleBeliefs(model, current)
	if len(drifts) != 2 {
		t.Fatalf("want 2 stale beliefs (1 drifted, 1 dropped); got %+v", drifts)
	}
	var sawDrifted, sawDropped bool
	for _, d := range drifts {
		switch d.Kind {
		case DriftDrifted:
			sawDrifted = true
			if d.Actual == nil || d.Actual.Content != "deleting the package MONDAY" {
				t.Errorf("drifted belief missing the subject's current atom: %+v", d)
			}
		case DriftDropped:
			sawDropped = true
			if d.Actual != nil {
				t.Errorf("dropped belief should have no current atom: %+v", d)
			}
		}
	}
	if !sawDrifted || !sawDropped {
		t.Errorf("want one drifted + one dropped; drifted=%v dropped=%v", sawDrifted, sawDropped)
	}
}

func TestMeshStateTwoRoundIncremental(t *testing.T) {
	r1 := map[string][]Atom{
		"mara":  {atom("mara", Commitment, "delete package", "deleting next week")},
		"ivo":   {atom("ivo", Dependency, "pricing", "calls pricing in-process")},
		"priya": {atom("priya", Intent, "freeze", "release freeze starts Monday")},
	}
	s := NewMeshState()
	seed := s.Advance(r1)
	// Round 1 seeds every ordered pair: 3 subjects × 2 observers each = 6 emissions.
	if len(seed) != 6 {
		t.Fatalf("round 1 emissions = %d, want 6 (3 subjects × 2 observers)", len(seed))
	}
	if s.Round() != 1 {
		t.Fatalf("round counter = %d, want 1", s.Round())
	}
	// No self-directed emission slipped in.
	for _, e := range seed {
		if SamePerson(e.Subject, e.Observer) {
			t.Errorf("self-directed emission: %+v", e)
		}
	}

	// Round 2: only Mara changes (deletes MONDAY now). Ivo and Priya are unchanged.
	r2 := map[string][]Atom{
		"mara":  {atom("mara", Commitment, "delete package", "deleting MONDAY")},
		"ivo":   {atom("ivo", Dependency, "pricing", "calls pricing in-process")},
		"priya": {atom("priya", Intent, "freeze", "release freeze starts Monday")},
	}
	// Surprise (read-only) and Advance must agree before the absorb.
	pre := s.Surprise(r2)
	got := s.Advance(r2)
	if len(pre) != len(got) {
		t.Fatalf("Surprise (%d) and Advance (%d) disagree", len(pre), len(got))
	}
	// Mara's one changed belief reaches her 2 observers; nothing else crosses.
	if len(got) != 2 {
		t.Fatalf("round 2 emissions = %d, want 2 (Mara's delta to ivo + priya only); got %+v", len(got), got)
	}
	for _, e := range got {
		if !SamePerson(e.Subject, "mara") {
			t.Errorf("round 2 emitted a non-Mara change (should be withheld): %+v", e)
		}
	}
	// Ivo's model of Mara is now current (absorbed the drift).
	m, ok := s.ModelOf("ivo", "mara")
	if !ok || len(m.Beliefs) != 1 || m.Beliefs[0].Content != "deleting MONDAY" {
		t.Fatalf("ivo's model of mara not updated: %+v (ok=%v)", m, ok)
	}
	if s.Round() != 2 {
		t.Fatalf("round counter = %d, want 2", s.Round())
	}
}

// --- adversarial pressure tests (pure; no API key) ---

// TestEmitDeltaSameSlotCollision is the regression guard for the silent-data-loss /
// phantom-re-emit bug: two distinct atoms in one (type, subject) slot. Before
// canonicalization the shadowed atom re-emitted every round (it never matched the
// surviving slot occupant). Now the slot collapses to the latest, and an unchanged
// self-model emits nothing.
func TestEmitDeltaSameSlotCollision(t *testing.T) {
	depA := atom("mara", Dependency, "pricing", "uses pricing lib A")
	depB := atom("mara", Dependency, "pricing", "uses pricing lib B") // same slot, different content
	delta := EmitDelta(nil, []Atom{depA, depB})
	if len(delta) != 1 {
		t.Fatalf("same-slot collision should collapse to 1, got %d: %+v", len(delta), delta)
	}
	if delta[0].Content != "uses pricing lib B" {
		t.Errorf("collapse should keep the LATEST, got %q", delta[0].Content)
	}
	// The decisive check: an unchanged self-model with a same-slot pair must emit
	// NOTHING on the next round — no phantom re-emission of the shadowed atom.
	if d := EmitDelta([]Atom{depA, depB}, []Atom{depA, depB}); len(d) != 0 {
		t.Fatalf("unchanged same-slot self-model must emit nothing, got %+v", d)
	}
}

// TestAbsorbSameSlotHoldsOne: a model never accumulates two beliefs in one slot.
func TestAbsorbSameSlotHoldsOne(t *testing.T) {
	s := NewMeshState()
	s.Advance(map[string][]Atom{
		"mara": {
			atom("mara", Dependency, "pricing", "uses lib A"),
			atom("mara", Dependency, "pricing", "uses lib B"),
		},
		"ivo": {atom("ivo", Intent, "engine", "building discount engine")},
	})
	m, ok := s.ModelOf("ivo", "mara")
	if !ok {
		t.Fatal("ivo should have a model of mara")
	}
	if len(m.Beliefs) != 1 {
		t.Fatalf("model holds %d beliefs in one slot, want 1 (latest wins): %+v", len(m.Beliefs), m.Beliefs)
	}
}

// TestAdvanceN1NoPairs: a single participant has no one to model — zero emissions,
// but the round still advances (the N=1 path must not panic or stall).
func TestAdvanceN1NoPairs(t *testing.T) {
	s := NewMeshState()
	got := s.Advance(map[string][]Atom{"dana": {atom("dana", Assumption, "retry", "assuming retries are safe")}})
	if len(got) != 0 {
		t.Fatalf("N=1 should emit nothing (no pairs), got %+v", got)
	}
	if s.Round() != 1 {
		t.Fatalf("round should still advance to 1, got %d", s.Round())
	}
}

// TestAdvanceAbsentPersonGoesStale: a participant present in round 1 but ABSENT in
// round 2 must not panic, must produce no emissions, and the models others hold of
// them must persist UNCHANGED (stale-because-absent), not be dropped.
func TestAdvanceAbsentPersonGoesStale(t *testing.T) {
	s := NewMeshState()
	s.Advance(map[string][]Atom{
		"mara":  {atom("mara", Commitment, "delete", "deleting next week")},
		"ivo":   {atom("ivo", Dependency, "pricing", "needs pricing in-process")},
		"priya": {atom("priya", Intent, "freeze", "freeze Monday")},
	})
	// Round 2: priya is absent entirely.
	got := s.Advance(map[string][]Atom{
		"mara": {atom("mara", Commitment, "delete", "deleting next week")}, // unchanged
		"ivo":  {atom("ivo", Dependency, "pricing", "needs pricing in-process")},
	})
	for _, e := range got {
		if SamePerson(e.Subject, "priya") || SamePerson(e.Observer, "priya") {
			t.Errorf("absent priya should be in no emission, got %+v", e)
		}
	}
	// Mara's model of the absent priya retains the round-1 belief (stale, not dropped).
	m, ok := s.ModelOf("mara", "priya")
	if !ok || len(m.Beliefs) != 1 || m.Beliefs[0].Content != "freeze Monday" {
		t.Fatalf("absent person's prior model should persist unchanged, got %+v (ok=%v)", m, ok)
	}
}

// TestAdvanceEmptySelfModel: a participant present with NO atoms (a model hiccup, or
// a genuinely empty note) seeds no beliefs about themselves but does not break the
// models others get of the rest of the team.
func TestAdvanceEmptySelfModel(t *testing.T) {
	s := NewMeshState()
	got := s.Advance(map[string][]Atom{
		"mara":    {atom("mara", Intent, "extract", "extracting pricing")},
		" ghost ": nil, // present, zero atoms, whitespace-padded name
	})
	for _, e := range got {
		if SamePerson(e.Subject, "ghost") {
			t.Errorf("an empty self-model should emit no beliefs about that person, got %+v", e)
		}
	}
	// ghost still gets a (belief-less) model of mara — being empty yourself doesn't
	// stop you learning others.
	if m, ok := s.ModelOf("ghost", "mara"); !ok || len(m.Beliefs) != 1 {
		t.Fatalf("ghost should still model mara, got %+v (ok=%v)", m, ok)
	}
}

// TestIdentityKeyingConsistent: INSIDE the L2 layer, the store's map key
// (normPerson) and the self-pair skip (normPerson, post-fix) are the SAME relation,
// so a model can't be filed under one spelling and looked up under another. For the
// normal ASCII case this also agrees with SamePerson.
func TestIdentityKeyingConsistent(t *testing.T) {
	pairs := [][2]string{
		{"Mara", "mara"}, {"MARA", "mara"}, {" ivo ", "ivo"},
		{"Priya", "PRIYA"}, {"ma ra", "ma  ra"}, {"bob", "alice"},
	}
	for _, p := range pairs {
		sameKey := normPerson(p[0]) == normPerson(p[1])
		if sameKey != SamePerson(p[0], p[1]) {
			t.Errorf("ASCII identity disagreement on (%q,%q): normPerson-key=%v SamePerson=%v", p[0], p[1], sameKey, SamePerson(p[0], p[1]))
		}
	}
}

// TestIdentityExoticFoldHandledConsistently: where Unicode case folding (SamePerson)
// and ToLower (normPerson) disagree — Greek Σ vs final-sigma ς — the OLD code skipped
// the self-pair via SamePerson while keying the store via normPerson, so a genuine
// cross-person pair was silently skipped as "self." Post-fix the self-skip uses
// normPerson too, so the two names are treated as distinct people CONSISTENTLY:
// real cross-person emissions, not a silent drop.
func TestIdentityExoticFoldHandledConsistently(t *testing.T) {
	s := NewMeshState()
	got := s.Advance(map[string][]Atom{
		"Σ": {atom("Σ", Intent, "x", "doing x")},
		"ς": {atom("ς", Intent, "y", "doing y")},
	})
	// normPerson("Σ")="σ" != normPerson("ς")="ς" → distinct people → 2 directed models.
	if len(got) != 2 {
		t.Fatalf("exotic-fold names must be handled consistently as 2 distinct people, got %d emissions: %+v", len(got), got)
	}
}

func TestMeshStateSnapshotSorted(t *testing.T) {
	s := NewMeshState()
	s.Advance(map[string][]Atom{
		"bob":   {atom("bob", Intent, "x", "y")},
		"alice": {atom("alice", Intent, "p", "q")},
	})
	snap := s.Snapshot()
	if len(snap) != 2 { // alice-of-bob, bob-of-alice
		t.Fatalf("snapshot = %d models, want 2", len(snap))
	}
	// Sorted by (observer, subject): alice→bob before bob→alice.
	if snap[0].Observer != "alice" || snap[1].Observer != "bob" {
		t.Fatalf("snapshot not sorted by observer: %+v", snap)
	}
}
