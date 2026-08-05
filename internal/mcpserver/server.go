// Package mcpserver exposes ettle's coordination engine over the Model Context
// Protocol. Each participant's OWN agent emits that person's notes; the server
// distills them through the privacy boundary into typed atoms, reconciles the
// team's atoms into coordination tangles, and surfaces them per-person.
//
// Why MCP and not a Slack/meeting bot: docs/ADOPTION.md disqualifies the
// viral-harvest pattern (a bot enrolls a participant list nobody consented to).
// An MCP tool is invoked by a participant's own agent — no non-participant is
// ever modeled, contacted, or harvested. The tool surface IS the consent
// boundary. The differentiated thing it leads with is the TANGLE (cross-person
// reconciliation), not the per-person standup summary that shipped products
// already do.
//
// v1 is in-memory, single-team, single-process, with explicit-name identity
// (the caller is trusted to emit only its own person). Persistence, per-agent
// auth (the gemot bearer-token shape), and the continuous live-emit loop are
// deliberately out of scope — see the plan and docs/SCALING.md.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/justinstimatze/ettle/internal/crux"
	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/tanglestate"
	"github.com/justinstimatze/ettle/internal/transport"
)

// reconciler is the narrow seam over *ettlemesh.Detector that the server needs.
// Depending on an interface rather than the concrete Detector keeps the handlers
// testable: the Detector's model seam (the fake `messager`) is unexported and
// in-package ettlemesh, so an external test cannot build a key-free Detector —
// but it can supply its own fake reconciler.
type reconciler interface {
	Distill(ctx context.Context, from, role, text string, private []string) ([]ettlemesh.Atom, error)
	ReconcileVoted(ctx context.Context, atoms []ettlemesh.Atom, samples int) (tangles []ettlemesh.Tangle, floorDropped int, err error)
	ReconcileSelf(ctx context.Context, atoms []ettlemesh.Atom) ([]ettlemesh.Tangle, error)
	GroundTangles(ctx context.Context, tangles []ettlemesh.Tangle, atoms []ettlemesh.Atom) (kept, suppressed []ettlemesh.Tangle, err error)
}

// defaultSamples matches the CLI default (voting on); 1 disables voting.
const defaultSamples = 5

// horizon is the shared coordination state the tools read and write. It is backed
// by the same transport seam the CLI uses, so an MCP server started with --room
// shares a horizon with teammates on other machines; the default in-process bus
// keeps the single-process behavior for local runs and tests.
type horizon struct {
	bus transport.Transport

	// base is the team's atoms as of this session's FIRST bus read — the previous
	// round the L2 layer needs (see reflect.go). Captured lazily, because reading
	// the bus at startup would put a Linear round-trip in front of session start,
	// and never rewritten.
	baseMu  sync.Mutex
	base    map[string][]ettlemesh.Atom
	baseSet bool
}

// newHorizon backs the horizon with the zero-infra in-process bus (one process,
// no sharing) — the default for a local `ettle mcp` and for tests.
func newHorizon() *horizon { return newHorizonOn(transport.NewInProcess()) }

// newHorizonOn backs the horizon with a caller-chosen transport (e.g. a leat room).
func newHorizonOn(bus transport.Transport) *horizon { return &horizon{bus: bus} }

func foldName(p string) string { return strings.ToLower(strings.TrimSpace(p)) }

// upsert publishes this participant's atoms to the bus. Re-emit overwrites at the
// read side (see snapshot) — the emit-delta refinement, re-emit only what changed,
// is a later step.
func (h *horizon) upsert(ctx context.Context, env transport.Envelope) error {
	return h.bus.Publish(ctx, env)
}

// snapshot collects every participant's envelope from the bus, folded to one per
// participant with the latest winning. The fold lives here rather than in the
// transports because they disagree: the in-process bus is append-only (a re-emit
// is a second envelope), while a leat lane is already last-writer-wins per author.
// The (model-calling) reconcile then runs on the returned copy.
func (h *horizon) snapshot(ctx context.Context) ([]transport.Envelope, error) {
	envs, err := h.bus.Collect(ctx)
	if err != nil {
		return nil, err
	}
	at := map[string]int{}
	out := make([]transport.Envelope, 0, len(envs))
	for _, e := range envs {
		k := foldName(e.Participant)
		if i, ok := at[k]; ok {
			out[i] = e // later emit wins
			continue
		}
		at[k] = len(out)
		out = append(out, e)
	}
	h.rememberBaseline(out)
	return out, nil
}

func (h *horizon) close() error { return h.bus.Close() }

type server struct {
	det    reconciler
	h      *horizon
	labels labelSink // where ettle_respond writes verdicts; nil disables the tool

	// stateKey names the room for the per-room tangle stores (muted/escalated). It is
	// the full transport SPEC, not the Linear room, because muting has to work on
	// every bus: keying it off the Linear room made a mute on a github:// or leat
	// room land in a shared "default" bucket, so every non-Linear room on the machine
	// muted each other's tangles.
	stateKey string
	// room is the Linear room this server escalates into (""=none/non-Linear).
	room string
	// team is the Linear team id, needed only to create the coordination issue the
	// first time an escalation lands.
	team string
	// esc posts a tangle as a Linear agent elicitation; nil disables ettle_escalate
	// (no app token, or not a Linear room). *transport.LinearAgentWriter satisfies it.
	esc escalator

	// resolver pre-stages a CONTESTED tangle (a decision-rights conflict or a
	// team-wide divergence) as the either/or a human decides between. nil means
	// crux.Inline, which needs no service and no key; a gemot resolver can be
	// substituted for the same seam the CLI uses. It never decides anything —
	// pre-staging the choice is the whole job.
	resolver crux.Resolver

	// lastSurfaced remembers the features of the tangles shown by the most recent
	// horizon() call, keyed by tangleKey, so a later ettle_respond can join a verdict
	// to the tangle's recurrence/tier (label enrichment). lastView remembers the full
	// surfaced tangle so ettle_escalate can render a tangle by key. Last horizon wins;
	// guarded by mu because the tools can be called concurrently.
	mu           sync.Mutex
	lastSurfaced map[string]tangleFeat
	lastView     map[string]tangleView
}

// escalator is the slice of *transport.LinearAgentWriter the escalate tool needs,
// as an interface so tests inject a fake with no network or app token.
type escalator interface {
	EnsureCoordinationIssue(ctx context.Context, room, teamID string) (issueID string, created bool, err error)
	OpenSession(ctx context.Context, issueID string) (sessionID string, err error)
	PostTangle(ctx context.Context, sessionID, body string) (activityID string, err error)
}

// tangleFeat is the calibration-relevant slice of a surfaced tangle: its kind, the
// recurrence (Votes of Samples) from voting, and whether it was shown firm or soft.
type tangleFeat struct {
	Kind    string
	Votes   int
	Samples int
	Firm    bool
}

// rememberSurfaced records one horizon call's surfaced-tangle features and full
// views, replacing the previous set (only tangles actually shown are labelable /
// escalatable, so this mirrors exactly what crossed to the agent).
func (s *server) rememberSurfaced(feats map[string]tangleFeat, views map[string]tangleView) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastSurfaced = feats
	s.lastView = views
}

// surfacedFeat returns the remembered features for a tangle key from the most recent
// horizon, if it was shown there.
func (s *server) surfacedFeat(key string) (tangleFeat, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.lastSurfaced[key]
	return f, ok
}

// surfacedView returns the full surfaced tangle for a key from the most recent
// horizon, so ettle_escalate can render it.
func (s *server) surfacedView(key string) (tangleView, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.lastView[key]
	return v, ok
}

// --- shareable projections (exactly what crosses, as plain JSON) ---

type atomView struct {
	Type       string  `json:"type"`
	Subject    string  `json:"subject"`
	Content    string  `json:"content"`
	Confidence float64 `json:"confidence"`
	Inferred   bool    `json:"inferred"`
}

func atomViews(atoms []ettlemesh.Atom) []atomView {
	out := make([]atomView, 0, len(atoms))
	for _, a := range atoms {
		out = append(out, atomView{
			Type: string(a.Typ), Subject: a.Subject, Content: a.Content,
			Confidence: a.Confidence, Inferred: a.Inferred,
		})
	}
	return out
}

type tangleView struct {
	Kind        string   `json:"kind"`
	Parties     []string `json:"parties"`
	About       string   `json:"about"`
	Explanation string   `json:"explanation"`
	Confidence  float64  `json:"confidence"`
	Votes       int      `json:"votes,omitempty"`
	Samples     int      `json:"samples,omitempty"`
	// Question marks a cross-person tangle the agent must present as a QUESTION to its
	// human, not an assertion — the detector cannot certify a cross-person conflict
	// (docs/LEGIBILITY.md stage 0c). Self tangles (own drift) are assertable and omit it.
	Question bool `json:"question,omitempty"`
	// Key identifies the coordination problem (kind + sorted parties, wording-
	// independent) so a human can answer it via ettle_respond — the label-capture
	// channel (stage 0c-2). Same key across horizon calls = the same tangle recurring.
	Key string `json:"key"`
	// Escalated is true when this tangle has already been posted to the room's Linear
	// coordination issue (ettle_escalate / `ettle escalate`), so the agent offers to
	// escalate only what a non-adopter can't already see.
	Escalated bool `json:"escalated,omitempty"`
	// Crux is set only on a CONTESTED tangle — a firm decision-rights conflict or
	// team-wide divergence, the two kinds that are a genuine values call rather than
	// something bindable. It is the choice pre-staged, never taken: present the
	// branches to the human and let them pick.
	Crux *cruxView `json:"crux,omitempty"`
}

// cruxView is a crux.Resolution shaped for the wire. Controversy and Proposal are
// gemot's; the inline resolver leaves them empty and supplies branches only.
type cruxView struct {
	Source      string   `json:"source" jsonschema:"inline | gemot"`
	Crux        string   `json:"crux" jsonschema:"the contested point, sharpened"`
	Controversy float64  `json:"controversy,omitempty"`
	Proposal    string   `json:"proposal,omitempty" jsonschema:"a binding compromise, when the resolver produced one"`
	Branches    []string `json:"branches"`
	Note        string   `json:"note"`
}

func toTangleView(k ettlemesh.Tangle) tangleView {
	return tangleView{
		Kind: k.Kind, Parties: k.Parties, About: k.About,
		Explanation: k.Explanation, Confidence: k.Confidence,
		Votes: k.Votes, Samples: k.Samples,
		Question: ettlemesh.MultiPerson(k.Parties),
		Key:      tangleKey(k.Kind, k.Parties),
	}
}

// tangleKey is the wording-independent identity of a coordination problem, shared
// with the CLI (escalate) via tanglestate so a tangle is the same tangle on both surfaces.
func tangleKey(kind string, parties []string) string {
	return tanglestate.Key(kind, parties)
}

func partiesInclude(parties []string, me string) bool {
	for _, p := range parties {
		if ettlemesh.SamePerson(p, me) {
			return true
		}
	}
	return false
}

// text wraps a human-readable summary as tool content. The SDK additionally
// marshals the typed Out struct into StructuredContent, so an agent gets the
// structured tangles while a human-facing client sees the summary line.
func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

// --- ettle_emit ---

type emitIn struct {
	Participant string `json:"participant" jsonschema:"the name of the person whose notes these are — YOUR OWN human, never a teammate"`
	Role        string `json:"role,omitempty" jsonschema:"the person's role on the team (optional, e.g. 'backend')"`
	Notes       string `json:"notes,omitempty" jsonschema:"the person's raw working notes or reasoning-in-progress; distilled server-side into typed atoms (needs the server's API key) — only the typed atoms are stored, the raw text is dropped. Supply this OR atoms, not both"`
	// Atoms is the key-free path: the caller's own agent already applied the
	// distillation rules (get them from the `ettle_distill` prompt) and sends the
	// typed result, so the raw note never leaves that person's machine and the
	// server makes no model call. Still sealed server-side — see ettlemesh.SealAtoms.
	Atoms []atomIn `json:"atoms,omitempty" jsonschema:"already-distilled typed atoms, if YOUR agent did the distillation locally (see the ettle_distill prompt). Needs no API key. Supply this OR notes, not both"`
}

// atomIn is a caller-supplied atom. It deliberately has no `from` field: the
// server attributes every atom to `participant`, so a caller cannot put words in
// a teammate's mouth by construction rather than by validation.
type atomIn struct {
	Type       string  `json:"type" jsonschema:"one of: intent | assumption | commitment | dependency"`
	Subject    string  `json:"subject" jsonschema:"a short noun phrase — what the atom is about"`
	Content    string  `json:"content" jsonschema:"one clause stating it"`
	Confidence float64 `json:"confidence,omitempty" jsonschema:"1.0 if the person stated it outright; 0.3-0.7 if the agent inferred it. Default 1.0"`
	Inferred   bool    `json:"inferred,omitempty" jsonschema:"true if the person did not state this and the agent inferred it"`
}

type emitOut struct {
	Participant string     `json:"participant"`
	Count       int        `json:"count"`
	Atoms       []atomView `json:"atoms"`
}

func (s *server) emit(ctx context.Context, _ *mcp.CallToolRequest, in emitIn) (*mcp.CallToolResult, emitOut, error) {
	if strings.TrimSpace(in.Participant) == "" {
		return nil, emitOut{}, fmt.Errorf("participant is required")
	}
	hasNotes, hasAtoms := strings.TrimSpace(in.Notes) != "", len(in.Atoms) > 0
	switch {
	case hasNotes && hasAtoms:
		return nil, emitOut{}, fmt.Errorf("supply notes OR atoms, not both: notes are distilled server-side, atoms are already distilled")
	case !hasNotes && !hasAtoms:
		return nil, emitOut{}, fmt.Errorf("one of notes or atoms is required (use the ettle_distill prompt to produce atoms locally without an API key)")
	}

	var (
		atoms []ettlemesh.Atom
		how   string
	)
	if hasAtoms {
		// Key-free path: the caller's agent already applied the distillation rules.
		// The SEMANTIC half of the boundary ran on the client where it cannot be
		// verified; the DETERMINISTIC half (caps, secret scanner, privacy override,
		// forced attribution) runs here where it can. Never trust the client for it.
		raw := make([]ettlemesh.Atom, 0, len(in.Atoms))
		for _, a := range in.Atoms {
			raw = append(raw, ettlemesh.Atom{
				Typ: ettlemesh.AtomType(a.Type), Subject: a.Subject, Content: a.Content,
				Confidence: a.Confidence, Inferred: a.Inferred,
			})
		}
		atoms = ettlemesh.SealAtoms(in.Participant, raw, nil)
		if len(atoms) == 0 {
			return nil, emitOut{}, fmt.Errorf("no usable atoms: each needs a type of intent|assumption|commitment|dependency plus a non-empty subject and content")
		}
		how = "distilled by your agent; raw notes never left your machine"
	} else {
		// Distill applies the privacy boundary (contextual-integrity prompt + the
		// deterministic secret scrub + structural caps). Only the typed atoms are
		// kept; the raw notes are never stored.
		var err error
		atoms, err = s.det.Distill(ctx, in.Participant, in.Role, in.Notes, nil)
		if err != nil {
			return nil, emitOut{}, err
		}
		how = "raw notes dropped"
	}
	if err := s.h.upsert(ctx, transport.Envelope{Participant: in.Participant, Role: in.Role, Atoms: atoms}); err != nil {
		return nil, emitOut{}, fmt.Errorf("publish %s: %w", in.Participant, err)
	}
	out := emitOut{Participant: in.Participant, Count: len(atoms), Atoms: atomViews(atoms)}
	return text(fmt.Sprintf("%s emitted %d atom(s) to the horizon (%s).", in.Participant, len(atoms), how)), out, nil
}

// --- ettle_horizon ---

type horizonIn struct {
	Me      string `json:"me,omitempty" jsonschema:"surface only tangles involving this participant (their agent's view); empty = the whole team's horizon"`
	Samples int    `json:"samples,omitempty" jsonschema:"independent reconcile samples to vote across; recurrence ranks tangles firm vs soft. Default 5; 1 disables voting"`
}

type horizonOut struct {
	Participants []string     `json:"participants"`
	Firm         []tangleView `json:"firm"`
	Soft         []tangleView `json:"soft"`
	// HeldBack: tangles the coupling check judged not-a-real-conflict, surfaced off the
	// agenda so the lead surface can show what was suppressed (legible abstention;
	// docs/LEGIBILITY.md). Omitted when empty.
	HeldBack []tangleView `json:"held_back,omitempty"`
	// FloorHeld: how many low-recurrence candidates the abstention floor dropped —
	// a count, not a list (they're noise by design), so a clear horizon stays honest.
	FloorHeld int `json:"floor_held,omitempty"`
	// Muted: how many tangles were suppressed because the human marked them
	// handled/not-real via ettle_respond — kept as a count so a horizon that is clear
	// only because tangles were muted stays honest.
	Muted int `json:"muted,omitempty"`
}

func (s *server) horizon(ctx context.Context, _ *mcp.CallToolRequest, in horizonIn) (*mcp.CallToolResult, horizonOut, error) {
	envs, err := s.h.snapshot(ctx)
	if err != nil {
		return nil, horizonOut{}, fmt.Errorf("collect: %w", err)
	}
	parts := make([]string, 0, len(envs))
	for _, e := range envs {
		parts = append(parts, e.Participant)
	}
	sort.Strings(parts)

	out := horizonOut{Participants: parts, Firm: []tangleView{}, Soft: []tangleView{}}

	atoms := transport.Atoms(envs)
	if len(atoms) == 0 {
		// Empty-horizon guard: nothing emitted → no model call.
		return text("the horizon is empty — no atoms emitted yet (call ettle_emit first)."), out, nil
	}

	samples := in.Samples
	if samples == 0 {
		samples = defaultSamples
	}
	tangles, floorHeld, err := s.det.ReconcileVoted(ctx, atoms, samples)
	if err != nil {
		return nil, horizonOut{}, err
	}
	// Cross-person coupling check: drop collision/duplication/teamwide tangles that
	// bridge people on a shared topic word across independent scopes (no-op if the
	// detector has Ground off). suppressed = what it held back, surfaced off the
	// agenda so the lead surface stays honest (legible abstention; docs/LEGIBILITY.md).
	tangles, suppressed, err := s.det.GroundTangles(ctx, tangles, atoms)
	if err != nil {
		return nil, horizonOut{}, err
	}
	// Per-room tangle state, shared with the CLI: muted tangles the human has resolved are
	// suppressed (not re-surfaced); escalated tangles are tagged so the agent offers to
	// escalate only what a non-adopter can't already see. A store read error is
	// non-fatal — degrade to "nothing muted / nothing escalated" rather than fail the
	// horizon.
	muted, _ := tanglestate.Load(tanglestate.Muted, s.stateKey)
	escalated, _ := tanglestate.Load(tanglestate.Escalated, s.stateKey)

	// Remember exactly what we surface (firm AND soft are both shown, so both are
	// labelable) so a later ettle_respond can join its verdict to the tangle's
	// recurrence, and ettle_escalate can render a tangle by key. The coupling-suppressed
	// and floor-dropped tangles are not surfaced, so they are correctly absent here.
	feats := map[string]tangleFeat{}
	views := map[string]tangleView{}
	for _, k := range tangles {
		if in.Me != "" && !partiesInclude(k.Parties, in.Me) {
			continue // agent surfaces only its own human's tangles, not a shared feed
		}
		v := toTangleView(k)
		if muted[v.Key] {
			out.Muted++ // handled/not-real — stop showing it, but stay honest about the count
			continue
		}
		v.Escalated = escalated[v.Key]
		v.Crux = s.resolve(ctx, k, atoms)
		feats[v.Key] = tangleFeat{Kind: k.Kind, Votes: k.Votes, Samples: k.Samples, Firm: k.Firm()}
		views[v.Key] = v
		if k.Firm() {
			out.Firm = append(out.Firm, v)
		} else {
			out.Soft = append(out.Soft, v)
		}
	}
	s.rememberSurfaced(feats, views)
	for _, k := range suppressed {
		if in.Me != "" && !partiesInclude(k.Parties, in.Me) {
			continue
		}
		out.HeldBack = append(out.HeldBack, toTangleView(k))
	}
	out.FloorHeld = floorHeld
	scope := "team"
	if in.Me != "" {
		scope = in.Me
	}
	return text(fmt.Sprintf("horizon (%s): %d firm, %d soft tangle(s) across %d participant(s)%s.",
		scope, len(out.Firm), len(out.Soft), len(parts), heldBackNote(len(out.HeldBack), floorHeld))), out, nil
}

// resolve pre-stages a contested tangle as the either/or a human picks between,
// which is the whole reason the crux seam exists: the mesh binds what is bindable
// and refuses to decide what isn't. Returns nil for every other tangle.
//
// A resolver failure is reported in the branch text rather than raised: a contested
// tangle whose crux is missing still has to reach the human, and failing the whole
// horizon over an unreachable gemot would hide every other tangle with it.
func (s *server) resolve(ctx context.Context, k ettlemesh.Tangle, atoms []ettlemesh.Atom) *cruxView {
	if !crux.Contested(k) {
		return nil
	}
	r := s.resolver
	if r == nil {
		r = crux.Inline{}
	}
	res, err := r.Resolve(ctx, k, atoms)
	if err != nil || res == nil {
		return &cruxView{
			Source: "unavailable", Crux: k.About,
			Branches: []string{fmt.Sprintf("resolver unavailable (%v) — the choice is still %s's to make", err, strings.Join(k.Parties, " and "))},
			Note:     "this is a values call, not a bindable one; present it and let the human decide",
		}
	}
	return &cruxView{
		Source: res.Source, Crux: res.Crux, Controversy: res.Controversy,
		Proposal: res.Proposal, Branches: res.Branches,
		Note: "this is a values call, not a bindable one; offer the branches and let the human pick — never choose for them",
	}
}

// heldBackNote renders the optional suppression tail on the horizon summary so a
// caller reading only the text line still learns candidates were held back — the
// coupling-check kills itemized in HeldBack, the floor drops as an aggregate count.
func heldBackNote(coupling, floor int) string {
	switch {
	case coupling > 0 && floor > 0:
		return fmt.Sprintf("; %d held back by the coupling check, %d below the floor", coupling, floor)
	case coupling > 0:
		return fmt.Sprintf("; %d held back by the coupling check", coupling)
	case floor > 0:
		return fmt.Sprintf("; %d held back below the confidence floor", floor)
	default:
		return ""
	}
}

// --- ettle_self_check (N=1) ---

type selfIn struct {
	Participant string `json:"participant" jsonschema:"the person whose notes these are"`
	Role        string `json:"role,omitempty" jsonschema:"the person's role (optional)"`
	Notes       string `json:"notes" jsonschema:"the person's own notes; checked for a stale self-assumption — a commitment that contradicts an assumption the same plan rests on. No teammate needed"`
}

type selfOut struct {
	Participant string       `json:"participant"`
	Atoms       []atomView   `json:"atoms"`
	Tangles     []tangleView `json:"tangles"`
}

// selfCheck is the N=1 on-ramp: distill one person's notes and run the self pass
// only (stale-self-assumption). It is stateless — it does NOT touch the shared
// horizon, so it is useful with no team present.
func (s *server) selfCheck(ctx context.Context, _ *mcp.CallToolRequest, in selfIn) (*mcp.CallToolResult, selfOut, error) {
	if strings.TrimSpace(in.Participant) == "" {
		return nil, selfOut{}, fmt.Errorf("participant is required")
	}
	if strings.TrimSpace(in.Notes) == "" {
		return nil, selfOut{}, fmt.Errorf("notes is required")
	}
	atoms, err := s.det.Distill(ctx, in.Participant, in.Role, in.Notes, nil)
	if err != nil {
		return nil, selfOut{}, err
	}
	tangles, err := s.det.ReconcileSelf(ctx, atoms)
	if err != nil {
		return nil, selfOut{}, err
	}
	out := selfOut{Participant: in.Participant, Atoms: atomViews(atoms), Tangles: []tangleView{}}
	for _, k := range tangles {
		out.Tangles = append(out.Tangles, toTangleView(k))
	}
	return text(fmt.Sprintf("%s: %d atom(s), %d self-tangle(s).", in.Participant, len(atoms), len(out.Tangles))), out, nil
}

// --- ettle_respond (stage 0c-2: capture the human verdict as the calibration label) ---

// Label is one human verdict on a surfaced cross-person tangle — the ground-truth
// signal stage 2's calibration loop will consume (docs/LEGIBILITY.md). It is captured
// now, before that loop exists, so the labeled data accrues from day one: a detector
// flag-rate is only calibratable against confirmations from people who saw the work.
type Label struct {
	Key     string `json:"key"`     // tangleKey: the coordination problem answered
	Verdict string `json:"verdict"` // real | not_real | handled
	By      string `json:"by"`      // the responder (their own tangle)
	Note    string `json:"note,omitempty"`
	TS      string `json:"ts"` // RFC3339, UTC
	// Kind/Votes/Samples/Firm are the surfaced tangle's features at capture time — the
	// recurrence signal (Votes of Samples) a future per-kind calibration loop would
	// threshold on, plus the kind and the firm/soft tier it was shown as. Populated
	// when ettle_respond runs against the server that surfaced the tangle (the common,
	// same-session path); a cross-session verdict carries Kind only (recovered from
	// the key) with zero recurrence. The loop itself is deliberately unbuilt — this
	// only stops the feature being discarded so the data is learnable if it accrues.
	// omitempty keeps pre-enrichment log lines (which lack these) parseable on read.
	Kind    string `json:"kind,omitempty"`
	Votes   int    `json:"votes,omitempty"`
	Samples int    `json:"samples,omitempty"`
	Firm    bool   `json:"firm,omitempty"`
}

// kindFromKey recovers the tangle Kind from a tangleKey ("kind|parties"). The Kind is
// always present even when the surfaced-horizon join misses (a verdict from a
// different session, or after a restart), so a label still carries its kind — just
// without the recurrence that only the surfacing server held.
func kindFromKey(key string) string {
	if i := strings.IndexByte(key, '|'); i >= 0 {
		return key[:i]
	}
	return ""
}

// labelSink persists verdicts. A file sink is the default; tests inject an in-memory
// one. Kept an interface so capture has no hard dependency on the filesystem.
type labelSink interface {
	record(Label) error
}

// fileLabelSink appends one JSON object per line — an append-only log the calibration
// loop (or any audit) can replay. Append+create, mutex-guarded for concurrent tools.
type fileLabelSink struct {
	mu   sync.Mutex
	path string
}

func newFileLabelSink(path string) *fileLabelSink { return &fileLabelSink{path: path} }

// LabelsPath is where verdicts accrue: an append-only JSONL file in the working dir,
// overridable by ETTLE_LABELS_PATH. Exported because the CLI writes the same log —
// a verdict recorded by `ettle mute` has to land in the same place as one recorded by
// ettle_respond, or the calibration data splits by which surface someone happened to
// use.
func LabelsPath() string {
	if p := strings.TrimSpace(os.Getenv("ETTLE_LABELS_PATH")); p != "" {
		return p
	}
	return "ettle-labels.jsonl"
}

// RecordLabel appends one verdict to the label log, filling the timestamp and
// recovering the tangle kind from the key. The recurrence features (votes, samples,
// firm) stay zero: only the server that surfaced the tangle held them, and inventing
// them would make an un-learnable row look learnable.
func RecordLabel(l Label) error {
	if strings.TrimSpace(l.TS) == "" {
		l.TS = time.Now().UTC().Format(time.RFC3339)
	}
	if l.Kind == "" {
		l.Kind = kindFromKey(l.Key)
	}
	return newFileLabelSink(LabelsPath()).record(l)
}

func (f *fileLabelSink) record(l Label) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	fh, err := os.OpenFile(f.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer fh.Close()
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	_, err = fh.Write(append(b, '\n'))
	return err
}

type respondIn struct {
	Me      string `json:"me" jsonschema:"the person responding — answer only your OWN tangles"`
	Tangle  string `json:"tangle" jsonschema:"the tangle's key field from ettle_horizon"`
	Verdict string `json:"verdict" jsonschema:"one of: real | not_real | handled | clear (clear un-mutes a tangle muted earlier)"`
	Note    string `json:"note,omitempty" jsonschema:"optional free-text context"`
}

type respondOut struct {
	Recorded bool   `json:"recorded"`
	Key      string `json:"key"`
	Verdict  string `json:"verdict"`
}

// respond records a human's verdict on a cross-person tangle. It does NOT mutate the
// horizon or bind anything — it only captures the label (humans stay the deciders;
// the loop that consumes these is stage 2, deliberately unbuilt).
func (s *server) respond(ctx context.Context, _ *mcp.CallToolRequest, in respondIn) (*mcp.CallToolResult, respondOut, error) {
	if s.labels == nil {
		return nil, respondOut{}, fmt.Errorf("label capture is not configured on this server")
	}
	me := strings.TrimSpace(in.Me)
	key := strings.TrimSpace(in.Tangle)
	if me == "" || key == "" {
		return nil, respondOut{}, fmt.Errorf("both `me` and `tangle` (the key from ettle_horizon) are required")
	}
	v := strings.ToLower(strings.TrimSpace(in.Verdict))
	switch v {
	case "real", "not_real", "handled", "clear":
	default:
		return nil, respondOut{}, fmt.Errorf("verdict must be one of real | not_real | handled | clear, got %q", in.Verdict)
	}
	// "clear" is the undo, not a verdict about the tangle — it un-mutes something
	// muted earlier and writes no label, because "I muted this by mistake" is not
	// ground truth about whether the detector was right.
	if v == "clear" {
		removed, err := tanglestate.Remove(tanglestate.Muted, s.stateKey, key)
		if err != nil {
			return nil, respondOut{}, fmt.Errorf("clear %s: %w", key, err)
		}
		msg := fmt.Sprintf("cleared the mute on %s — it can surface again.", key)
		if !removed {
			msg = fmt.Sprintf("%s was not muted; nothing to clear.", key)
		}
		return text(msg), respondOut{Recorded: removed, Key: key, Verdict: v}, nil
	}
	lbl := Label{Key: key, Verdict: v, By: me, Note: in.Note, TS: time.Now().UTC().Format(time.RFC3339)}
	// Enrich with the surfaced tangle's features so the verdict is learnable. Same
	// session (the common path: an agent answers the horizon it just read) → full
	// recurrence; otherwise recover the kind from the key and leave recurrence zero
	// rather than fabricate it.
	if feat, ok := s.surfacedFeat(key); ok {
		lbl.Kind, lbl.Votes, lbl.Samples, lbl.Firm = feat.Kind, feat.Votes, feat.Samples, feat.Firm
	} else if kind := kindFromKey(key); kind != "" {
		lbl.Kind = kind
	}
	if err := s.labels.record(lbl); err != nil {
		return nil, respondOut{}, fmt.Errorf("record label: %w", err)
	}
	// A "not_real" (false alarm) or "handled" (resolved) verdict MUTES the tangle so
	// horizon stops re-surfacing it and escalate won't post it — the label loop that
	// consumes verdicts is still unbuilt, but muting makes the verdict act now.
	// "real" leaves it surfaced (a genuine open conflict the human should keep seeing).
	muted := ""
	if v == "not_real" || v == "handled" {
		if err := tanglestate.Add(tanglestate.Muted, s.stateKey, key); err != nil {
			return nil, respondOut{}, fmt.Errorf("mute %s: %w", key, err)
		}
		muted = " (muted — it won't resurface)"
	}
	return text(fmt.Sprintf("recorded: %s judged %q on %s%s.", me, v, key, muted)),
		respondOut{Recorded: true, Key: key, Verdict: v}, nil
}

// --- ettle_escalate (surface a tangle to a non-adopter on Linear) ---

type escalateIn struct {
	Tangle string `json:"tangle" jsonschema:"the tangle key (from ettle_horizon) to escalate — post this ONE coordination tangle to the room's Linear coordination issue for a teammate who doesn't run ettle"`
}

type escalateOut struct {
	Escalated    bool   `json:"escalated"`
	Key          string `json:"key"`
	IssueCreated bool   `json:"issue_created,omitempty"`
}

// escalate posts one surfaced tangle as a native Linear agent elicitation on the
// room's single coordination issue, so a teammate who never installed ettle sees it
// and can reply. Deliberate and per-tangle: the agent offers, the human says yes, this
// runs. It renders the tangle the LAST ettle_horizon surfaced (call horizon first).
func (s *server) escalate(ctx context.Context, _ *mcp.CallToolRequest, in escalateIn) (*mcp.CallToolResult, escalateOut, error) {
	if s.esc == nil {
		return nil, escalateOut{}, fmt.Errorf("escalation is not configured on this server (needs LINEAR_AGENT_TOKEN and a Linear room)")
	}
	key := strings.TrimSpace(in.Tangle)
	if key == "" {
		return nil, escalateOut{}, fmt.Errorf("tangle (the key from ettle_horizon) is required")
	}
	view, ok := s.surfacedView(key)
	if !ok {
		return nil, escalateOut{}, fmt.Errorf("unknown tangle %q — call ettle_horizon first, then escalate a key it surfaced", key)
	}
	issueID, created, err := s.esc.EnsureCoordinationIssue(ctx, s.room, s.team)
	if err != nil {
		return nil, escalateOut{}, fmt.Errorf("ensure coordination issue: %w", err)
	}
	sid, err := s.esc.OpenSession(ctx, issueID)
	if err != nil {
		return nil, escalateOut{}, fmt.Errorf("open agent session: %w", err)
	}
	if _, err := s.esc.PostTangle(ctx, sid, escalateBody(view)); err != nil {
		return nil, escalateOut{}, fmt.Errorf("post tangle: %w", err)
	}
	if err := tanglestate.Add(tanglestate.Escalated, s.stateKey, key); err != nil {
		return nil, escalateOut{}, fmt.Errorf("record escalated: %w", err)
	}
	return text(fmt.Sprintf("escalated %s to room %q's coordination issue.", key, s.room)),
		escalateOut{Escalated: true, Key: key, IssueCreated: created}, nil
}

// escalateBody renders a surfaced tangle as the elicitation a teammate reads. Framed
// as a question to confirm — humans stay the deciders; the mesh never asserts a
// cross-person conflict as fact.
func escalateBody(v tangleView) string {
	parties := strings.Join(v.Parties, ", ")
	var b strings.Builder
	if strings.TrimSpace(v.About) != "" {
		fmt.Fprintf(&b, "**Possible %s — %s** (%s)\n\n", v.Kind, v.About, parties)
	} else {
		fmt.Fprintf(&b, "**Possible %s** (%s)\n\n", v.Kind, parties)
	}
	if strings.TrimSpace(v.Explanation) != "" {
		b.WriteString(v.Explanation)
		b.WriteString("\n\n")
	}
	b.WriteString("Reply here if this needs a decision — ettle brings your answer back into the team's coordination. If it's already handled, say so.")
	return strings.TrimSpace(b.String())
}

// newMCPServer builds the MCP server with the tools registered. Shared by
// Serve (stdio) and the in-memory round-trip test.
func newMCPServer(s *server, version string) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "ettle", Version: version}, nil)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "ettle_emit",
		Description: "Emit YOUR OWN human to the team coordination horizon, two ways. Pass `notes` and the server distills them through the privacy boundary into typed atoms (needs the server's API key; only the atoms are stored, raw notes are dropped). Or pass `atoms` you distilled yourself with the `ettle_distill` prompt — no API key, and the raw notes never leave this machine. Either way it returns exactly what crossed. Emit only your own person — never a teammate.",
	}, s.emit)

	// The key-free path made discoverable. An agent that already has its human's
	// notes and its own model has everything needed to distill locally; this prompt
	// is the rule set that makes its output the same shape the server would produce.
	srv.AddPrompt(&mcp.Prompt{
		Name:        "ettle_distill",
		Description: "The rules for distilling YOUR OWN human's notes into typed coordination atoms locally, so you can call ettle_emit with `atoms` instead of `notes` — no API key, and the raw notes never leave this machine.",
		Arguments: []*mcp.PromptArgument{
			{Name: "participant", Description: "the name of the person whose notes you are distilling — your own human", Required: true},
			{Name: "role", Description: "their role on the team (optional, e.g. 'backend')"},
			{Name: "private", Description: "comma-separated phrases the person marked private, which must never appear in an atom (optional)"},
		},
	}, distillPrompt)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "ettle_horizon",
		Description: "Reconcile the team's emitted atoms into coordination tangles — collisions, duplicated work, stale assumptions, decision-rights gaps — split into firm (worth a look) and soft (worth a question). Pass `me` to see only the tangles involving your own human.",
	}, s.horizon)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "ettle_self_check",
		Description: "Useful at N=1, no team needed: distill one person's notes and surface a stale self-assumption — a commitment that contradicts an assumption the same plan rests on. Stateless; does not touch the shared horizon.",
	}, s.selfCheck)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "ettle_respond",
		Description: "Record YOUR human's verdict on a cross-person tangle from ettle_horizon — real, not_real, handled, or clear. Pass the tangle's `key`. `not_real` (false alarm) and `handled` (resolved) MUTE the tangle so it stops re-surfacing and won't be escalated; `real` keeps it on the horizon; `clear` un-mutes one muted earlier and records no label. It captures the verdict as the calibration ground-truth; it does not bind or decide the work itself.",
	}, s.respond)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "ettle_mirror",
		Description: "Show YOUR human what the team's directed models (L2) currently believe ABOUT them, and which of those beliefs their later work has already made stale — the layer that drives how someone gets treated, made readable to that person. No API key and no model call. Attribution is withheld by default (the belief, not who holds it); pass `by_observer` to attribute, which surfaces a teammate's private model of your human.",
	}, s.mirror)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "ettle_drift",
		Description: "The emit side of the same layer: which changes since this session first read the bus would leave a teammate's model stale, and therefore who each one is routed to — rather than broadcast to everyone. Returns the routing savings. No API key and no model call. Pass `me` to see only what involves your own human.",
	}, s.drift)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "ettle_room_status",
		Description: "Who is on the room's bus and what each person is working on, depending on, committed to, and assuming — read straight off the bus with no tangle detection, no API key and no model call, so it is cheap to call often. Freshness is coarse on purpose (recently / today / yesterday / Nd ago): a per-minute age across a team is a working-patterns feed.",
	}, s.roomStatus)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "ettle_escalate",
		Description: "Surface ONE coordination tangle to a teammate who doesn't run ettle, by posting it as a native elicitation on the room's Linear coordination issue (never a feature ticket). Pass the tangle `key` from ettle_horizon — prefer tangles shown escalated:false. Deliberate: offer it to your human first, then call this on yes. The teammate's reply comes back via `ettle pull`.",
	}, s.escalate)

	return srv
}

// distillPrompt hands the caller's agent the same boundary rules the server-side
// distiller runs under, so client-side distillation is a relocation of the work
// and not a weakening of it. The instructions never include the note — the agent
// already holds it, which is the whole point.
func distillPrompt(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	args := req.Params.Arguments
	who := strings.TrimSpace(args["participant"])
	if who == "" {
		return nil, fmt.Errorf("participant is required")
	}
	var private []string
	for _, p := range strings.Split(args["private"], ",") {
		if p = strings.TrimSpace(p); p != "" {
			private = append(private, p)
		}
	}
	sys, instructions := ettlemesh.DistillGuide(who, args["role"], private)
	return &mcp.GetPromptResult{
		Description: "Distill " + who + "'s notes into typed atoms locally, then call ettle_emit with `atoms`.",
		Messages: []*mcp.PromptMessage{
			{Role: "user", Content: &mcp.TextContent{Text: sys + "\n\n---\n\n" + instructions}},
		},
	}, nil
}

// Serve registers the tools and runs the server over stdio until ctx is done.
// version is passed in because mcpserver cannot import package main (where
// buildVersion lives). Stdio discipline: stdout is the JSON-RPC channel, so
// callers must keep all logging on stderr.
//
// bus is the horizon's backing transport: pass transport.NewInProcess() for a
// local single-process server, or a room's bus so the agents driving this server
// share a horizon with teammates on other machines. Serve closes it on return.
// stateKey is the transport spec (e.g. "github://acme/widgets/crew"), which keys the
// per-room muted/escalated stores — every bus needs muting, so this is deliberately
// not the Linear room. linRoom IS the Linear room ("" if the bus is not Linear) and
// is only the escalate target: when it is set and LINEAR_AGENT_TOKEN is present,
// ettle_escalate is enabled (posts as the OAuth app actor); otherwise the tool
// reports it is not configured.
// resolver selects how contested tangles get staged: nil takes crux.Inline, which
// needs nothing running. Pass a crux.Gemot to deliberate them against a gemot
// endpoint instead.
func Serve(ctx context.Context, det reconciler, bus transport.Transport, version, stateKey, linRoom string, resolver crux.Resolver) error {
	// Label capture is local-first (see LabelsPath). The verdicts are the calibration
	// loop's future input (stage 2); writing them now means the data exists before the
	// loop does.
	// crux.Inline needs no service and no key, so a contested tangle arrives
	// pre-staged on every install rather than only where a gemot happens to be
	// running (see server.resolver).
	if resolver == nil {
		resolver = crux.Inline{}
	}
	s := &server{det: det, h: newHorizonOn(bus), labels: newFileLabelSink(LabelsPath()), stateKey: stateKey, room: linRoom, resolver: resolver}
	// Enable ettle_escalate only for a Linear room with an app-actor token present —
	// the member key can read agent activities but not post them.
	if tok := strings.TrimSpace(os.Getenv("LINEAR_AGENT_TOKEN")); tok != "" && strings.TrimSpace(linRoom) != "" {
		s.esc = transport.NewLinearAgentWriter(tok, version)
		s.team = strings.TrimSpace(os.Getenv("LINEAR_TEAM_ID"))
	}
	defer func() { _ = s.h.close() }()
	return newMCPServer(s, version).Run(ctx, &mcp.StdioTransport{})
}
