package ettlemesh

import (
	"sort"
	"strings"
)

// directed.go is L2 — the directed dyadic models, the layer between L1 (each
// person's self-model) and L3 (the collective reconcile) that the single-shot
// flat-pool path skipped. Per CONCEPT.md: an observer's agent's model OF a subject
// is asymmetric (Alice-of-Bob is not Bob-of-Alice), there are N×(N−1) of them, and
// they are carried across ROUNDS so they can go stale. Staleness is the point:
// "surprise" is the divergence between an observer's model of a subject (L2) and
// that subject's actual current self-model (L1), and the principled emit rule falls
// out of it — a subject emits exactly the deltas that would otherwise leave others'
// models of it stale, never its whole self-model.
//
// This layer is DETERMINISTIC and pure (no model call): it is a projection of the
// atoms that crossed the boundary plus a structural diff, not a second semantic
// pass. That keeps it O(1) per the no-machine-speed-loop invariant and fully
// unit-testable without an API key. The semantic work stays in L1 (Distill) and the
// L3 reconcile; L2 is the routing-and-staleness structure over their output. (The
// richer per-pair SEMANTIC model — the agent reasoning "what is Bob likely assuming
// that he didn't say" — is a future enrichment on top of this structural base; see
// docs/Status.)

// DirectedModel is one observer's model of one subject: the belief-atoms the
// observer currently holds about the subject, built only from atoms the subject
// emitted across the boundary (never the subject's raw note), plus the round it was
// last updated. The subject is the author (From) of every belief. Exported fields
// so the whole store serializes to JSON for cross-session persistence (Snapshot).
type DirectedModel struct {
	Observer string `json:"observer"`
	Subject  string `json:"subject"`
	Beliefs  []Atom `json:"beliefs"`
	Round    int    `json:"round"`
}

// normPerson is the canonical map key for a participant name (trim + lowercase).
// It is the SINGLE identity relation used INSIDE the L2 layer: the store keys on it
// (modelFor / ModelOf) and the self-pair skip in diff uses it too, so a model can't
// be filed under one spelling and looked up under another. NOTE it is NOT identical
// to SamePerson (mesh.go), which uses Unicode case folding (EqualFold): for exotic
// names where folding and ToLower disagree (e.g. Greek Σ vs final-sigma ς), L2 keeps
// them as distinct people — consistently, on both the key and the self-skip — rather
// than letting the two relations disagree and silently skip a real pair. ASCII names,
// the normal case, are unaffected.
func normPerson(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// beliefKey identifies the SLOT a belief occupies in a model: an atom of a given
// type about a given subject. Two atoms with the same key are the same belief; if
// their content (or confidence) differs, the belief CHANGED and the observer's copy
// is now stale. Keyed on type+subject, folded for case and whitespace only.
//
// KNOWN LIMIT (this is the structural layer's edge): the fold is case/whitespace,
// NOT semantic. Subjects are uncontrolled distiller output, so when a stochastic
// re-distill rewords the subject of a still-held belief, the old key vanishes and a
// new one appears — and the diff reads that as drop+new (a "dropped" in the surfaced
// view, an orphaned slot in the model) rather than the reword it is. Byte-identical
// note reuse upstream (cmd/ettle distillCurrent) avoids re-distilling an unchanged
// note at all, which sidesteps this for the common case; but any note that genuinely
// changed re-distills in full and its unchanged beliefs can reword. Closing this for
// real needs wording-INDEPENDENT slot identity (a deterministic fuzzy match over the
// existing tokenSet/jaccard, or distiller-stable slot ids) — tracked as the next
// step, not yet built. Until then the surfaced "dropped" signal is hedged, not
// asserted (see cmd/ettle printDrift).
//
// DESIGN CHOICE, made explicit: the (type, subject) slot IS the unit of belief — a
// model holds at most ONE belief per slot. If a self-model carries two atoms in the
// same slot (a terse distiller can), they are collapsed to the latest by canonical
// (below) before any diff, deterministically. Without that collapse the slot maps
// here would silently keep whichever atom landed last AND EmitDelta would re-emit
// the shadowed atom every round (it never matches the surviving slot occupant) — a
// phantom, unending emission. Collapsing up front makes the projection lossy but
// DEFINED and stable, which the surprise gate requires.
func beliefKey(a Atom) string {
	return string(a.Typ) + "\x00" + strings.ToLower(strings.Join(strings.Fields(a.Subject), " "))
}

// canonical collapses a self-model to one atom per (type, subject) slot, keeping the
// LAST occurrence and preserving first-seen order. It is idempotent and the single
// gate every diff runs its inputs through (EmitDelta, StaleBeliefs), so the slot
// invariant — at most one belief per slot — holds no matter how many same-slot atoms
// a distiller emits, and a shadowed atom can never become a phantom re-emission.
func canonical(atoms []Atom) []Atom {
	pos := make(map[string]int, len(atoms))
	out := make([]Atom, 0, len(atoms))
	for _, a := range atoms {
		k := beliefKey(a)
		if i, ok := pos[k]; ok {
			out[i] = a // later wins, in place — order is the first sighting's
			continue
		}
		pos[k] = len(out)
		out = append(out, a)
	}
	return out
}

// Canonical collapses atoms to one per (type, subject) slot — last wins, first-seen
// order — the same slot-dedup the L2 diff uses internally. Exported so a caller
// building a view over MULTIPLE models (the mirror's union of every teammate's beliefs
// ABOUT one person) dedups on the engine's slot identity rather than a re-implemented
// one. Pure and idempotent.
func Canonical(atoms []Atom) []Atom { return canonical(atoms) }

// sameBelief reports whether two atoms occupying the same slot carry the same
// belief — same content and same confidence. A confidence change (an inferred guess
// the subject later stated outright, say) is itself a delta worth emitting.
func sameBelief(a, b Atom) bool {
	return strings.EqualFold(strings.TrimSpace(a.Content), strings.TrimSpace(b.Content)) &&
		a.Confidence == b.Confidence
}

// EmitDelta is the principled emit rule, operationalized (CONCEPT.md, "A principled
// emit rule"): given what an observer already believes about a subject (prior) and
// the subject's current self-model (current), return exactly the atoms that would
// otherwise leave the observer's model stale — the NEW slots and the CHANGED ones.
// Atoms identical to what the observer already holds are withheld: re-emitting them
// is noise and the machine-speed feedback the design forbids (emit is surprise-
// gated). Pure; no model call.
//
// The returned atoms ARE the surprise measure for this ordered pair: the points
// where the observer's model of the subject diverges from the subject's reality
// (surprise = L2-vs-L1 divergence), computed before the observer absorbs them.
func EmitDelta(prior, current []Atom) []Atom {
	priorByKey := make(map[string]Atom, len(prior))
	for _, a := range canonical(prior) {
		priorByKey[beliefKey(a)] = a
	}
	var delta []Atom
	for _, a := range canonical(current) {
		if p, ok := priorByKey[beliefKey(a)]; !ok || !sameBelief(p, a) {
			delta = append(delta, a)
		}
	}
	return delta
}

// Drift kinds: a belief in an observer's model has either drifted (the subject now
// holds that slot differently) or been dropped (the subject no longer holds it).
const (
	DriftDrifted = "drifted"
	DriftDropped = "dropped"
)

// Drift is one stale belief in an observer's model of a subject: a concrete
// instance of surprise — a place the observer would be wrong about the subject
// because no atom corrected them. Actual is the subject's current atom in the same
// slot (nil when the belief was dropped).
type Drift struct {
	Observer string
	Subject  string
	Believed Atom
	Actual   *Atom
	Kind     string
}

// StaleBeliefs compares an observer's model of a subject (L2) against the subject's
// actual current self-model (L1) and returns every belief that has gone stale:
// drifted or dropped. This is the surprise detector run from the side that can see
// both models — only the subject's own machine holds its true L1, so a real
// deployment runs this there and emits the corrections (the emit rule). Pure;
// complements EmitDelta (which reports new+changed from the subject's side, where
// this reports drifted+dropped from the observer's).
func StaleBeliefs(model DirectedModel, currentSelf []Atom) []Drift {
	curByKey := make(map[string]Atom, len(currentSelf))
	for _, a := range canonical(currentSelf) {
		curByKey[beliefKey(a)] = a
	}
	var out []Drift
	for _, b := range canonical(model.Beliefs) {
		believed := b
		cur, ok := curByKey[beliefKey(b)]
		switch {
		case !ok:
			out = append(out, Drift{Observer: model.Observer, Subject: model.Subject, Believed: believed, Kind: DriftDropped})
		case !sameBelief(cur, b):
			actual := cur
			out = append(out, Drift{Observer: model.Observer, Subject: model.Subject, Believed: believed, Actual: &actual, Kind: DriftDrifted})
		}
	}
	return out
}

// Emission is one directed delta crossing the boundary in a round: the atoms the
// subject must send to a SPECIFIC observer to keep that observer's model fresh.
// This is the L2 routing payoff — not a broadcast, but exactly who needs to hear
// what, and (across rounds) how little.
type Emission struct {
	Subject  string
	Observer string
	Atoms    []Atom
}

// MeshState is the persisted L2 store: every observer's DirectedModel of every
// other participant, carried across rounds. It is what the single-shot flat-pool
// reconcile never had — asymmetric, per-pair, and longitudinal, so a model can be
// stale and the emit gate can send only deltas. Serializable through Snapshot so a
// real deployment persists it between sessions; the PoC holds it in process.
type MeshState struct {
	models map[string]map[string]*DirectedModel // [observer][subject]
	round  int
}

// NewMeshState returns an empty L2 store (round 0, no models yet).
func NewMeshState() *MeshState {
	return &MeshState{models: map[string]map[string]*DirectedModel{}}
}

// Round reports how many rounds have been advanced.
func (s *MeshState) Round() int { return s.round }

// ModelOf returns the observer's current model of the subject and whether one
// exists yet. The returned value is a copy of the header but shares the Beliefs
// slice; treat as read-only.
func (s *MeshState) ModelOf(observer, subject string) (DirectedModel, bool) {
	if subs, ok := s.models[normPerson(observer)]; ok {
		if m, ok := subs[normPerson(subject)]; ok {
			return *m, true
		}
	}
	return DirectedModel{}, false
}

// Surprise computes, without mutating the store, the divergence between every
// observer's current model and each subject's given self-model — the emit deltas
// that WOULD cross if Advance were called now. It is the pre-round surprise
// readout: who is about to learn what about whom. Advance performs the same diff
// and then absorbs it.
func (s *MeshState) Surprise(selfModels map[string][]Atom) []Emission {
	return s.diff(selfModels, false)
}

// Advance runs one round: each subject's current self-model is diffed against what
// every other observer currently believes about them; the deltas (and only the
// deltas) cross, each observer absorbs the delta it received, and the round counter
// advances. Returns every emission, so the caller can show who was sent what — and,
// across rounds, how little (round 2 re-sends a changed atom, not the whole
// self-model). selfModels is keyed by participant; each atom's From should match
// its key.
func (s *MeshState) Advance(selfModels map[string][]Atom) []Emission {
	s.round++
	return s.diff(selfModels, true)
}

// diff is the shared body of Surprise (absorb=false) and Advance (absorb=true): for
// every ordered (observer, subject) pair, compute the emit delta and optionally fold
// it into the observer's model. Deterministic order (sorted names) so output and
// persistence are stable.
func (s *MeshState) diff(selfModels map[string][]Atom, absorb bool) []Emission {
	people := make([]string, 0, len(selfModels))
	for p := range selfModels {
		people = append(people, p)
	}
	sort.Strings(people)
	var emissions []Emission
	for _, subject := range people {
		for _, observer := range people {
			if normPerson(observer) == normPerson(subject) {
				// No self-directed model: a subject is not an observer of itself. Use
				// normPerson (the store's key relation), NOT SamePerson — so the
				// self-skip and the model key agree by construction and an exotic
				// fold/lowercase disagreement can't skip a genuine cross-person pair.
				continue
			}
			m := s.modelFor(observer, subject)
			delta := EmitDelta(m.Beliefs, selfModels[subject])
			if len(delta) == 0 {
				continue
			}
			emissions = append(emissions, Emission{Subject: subject, Observer: observer, Atoms: delta})
			if absorb {
				m.absorb(delta, s.round)
			}
		}
	}
	return emissions
}

// modelFor returns the (observer, subject) model, creating an empty one on first
// touch. Created lazily so the store is sparse until a pair actually interacts.
func (s *MeshState) modelFor(observer, subject string) *DirectedModel {
	o, sub := normPerson(observer), normPerson(subject)
	if s.models[o] == nil {
		s.models[o] = map[string]*DirectedModel{}
	}
	m := s.models[o][sub]
	if m == nil {
		m = &DirectedModel{Observer: observer, Subject: subject}
		s.models[o][sub] = m
	}
	return m
}

// absorb folds a received delta into the model: changed slots are replaced in
// place, new slots appended, untouched beliefs kept (their staleness, if the
// subject silently changes them in a later round, is exactly what StaleBeliefs
// catches against the subject's then-current self-model).
func (m *DirectedModel) absorb(delta []Atom, round int) {
	byKey := make(map[string]int, len(m.Beliefs))
	for i, b := range m.Beliefs {
		byKey[beliefKey(b)] = i
	}
	for _, a := range delta {
		if i, ok := byKey[beliefKey(a)]; ok {
			m.Beliefs[i] = a
		} else {
			byKey[beliefKey(a)] = len(m.Beliefs)
			m.Beliefs = append(m.Beliefs, a)
		}
	}
	m.Round = round
}

// Snapshot returns every directed model in the store, sorted by (observer,
// subject), for inspection or JSON persistence between sessions. The PoC keeps the
// store in process; a deployment writes this out and reloads it next round.
func (s *MeshState) Snapshot() []DirectedModel {
	var out []DirectedModel
	for _, subs := range s.models {
		for _, m := range subs {
			out = append(out, *m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Observer != out[j].Observer {
			return out[i].Observer < out[j].Observer
		}
		return out[i].Subject < out[j].Subject
	})
	return out
}
