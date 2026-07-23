// Package mcpserver exposes ettle's coordination engine over the Model Context
// Protocol. Each participant's OWN agent emits that person's notes; the server
// distills them through the privacy boundary into typed atoms, reconciles the
// team's atoms into coordination knots, and surfaces them per-person.
//
// Why MCP and not a Slack/meeting bot: docs/ADOPTION.md disqualifies the
// viral-harvest pattern (a bot enrolls a participant list nobody consented to).
// An MCP tool is invoked by a participant's own agent — no non-participant is
// ever modeled, contacted, or harvested. The tool surface IS the consent
// boundary. The differentiated thing it leads with is the KNOT (cross-person
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

	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/transport"
)

// reconciler is the narrow seam over *ettlemesh.Detector that the server needs.
// Depending on an interface rather than the concrete Detector keeps the handlers
// testable: the Detector's model seam (the fake `messager`) is unexported and
// in-package ettlemesh, so an external test cannot build a key-free Detector —
// but it can supply its own fake reconciler.
type reconciler interface {
	Distill(ctx context.Context, from, role, text string, private []string) ([]ettlemesh.Atom, error)
	ReconcileVoted(ctx context.Context, atoms []ettlemesh.Atom, samples int) (knots []ettlemesh.Knot, floorDropped int, err error)
	ReconcileSelf(ctx context.Context, atoms []ettlemesh.Atom) ([]ettlemesh.Knot, error)
	GroundKnots(ctx context.Context, knots []ettlemesh.Knot, atoms []ettlemesh.Atom) (kept, suppressed []ettlemesh.Knot, err error)
}

// defaultSamples matches the CLI default (voting on); 1 disables voting.
const defaultSamples = 5

// horizon is the in-memory shared coordination state for ONE team/process: each
// participant's distilled atoms, keyed by a folded (lowercased, trimmed) name.
type horizon struct {
	mu   sync.Mutex
	envs map[string]transport.Envelope
}

func newHorizon() *horizon { return &horizon{envs: map[string]transport.Envelope{}} }

func foldName(p string) string { return strings.ToLower(strings.TrimSpace(p)) }

// upsert replaces this participant's atoms. Re-emit overwrites (the emit-delta
// refinement — re-emit only what changed — is a later step).
func (h *horizon) upsert(env transport.Envelope) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.envs[foldName(env.Participant)] = env
}

// snapshot returns a copy of every participant's envelope, taken under the lock.
// The (model-calling) reconcile runs on the copy OUTSIDE the lock — the mutex is
// never held across an API call.
func (h *horizon) snapshot() []transport.Envelope {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]transport.Envelope, 0, len(h.envs))
	for _, e := range h.envs {
		out = append(out, e)
	}
	return out
}

type server struct {
	det    reconciler
	h      *horizon
	labels labelSink // where ettle_respond writes verdicts; nil disables the tool

	// lastSurfaced remembers the features of the knots shown by the most recent
	// horizon() call, keyed by knotKey, so a later ettle_respond can join a verdict
	// to the knot's recurrence/tier (label enrichment). Last horizon wins; guarded
	// by mu because horizon and respond can be called concurrently.
	mu           sync.Mutex
	lastSurfaced map[string]knotFeat
}

// knotFeat is the calibration-relevant slice of a surfaced knot: its kind, the
// recurrence (Votes of Samples) from voting, and whether it was shown firm or soft.
type knotFeat struct {
	Kind    string
	Votes   int
	Samples int
	Firm    bool
}

// rememberSurfaced records one horizon call's surfaced-knot features, replacing the
// previous set (only knots actually shown are labelable, so this mirrors exactly
// what crossed to the agent).
func (s *server) rememberSurfaced(feats map[string]knotFeat) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastSurfaced = feats
}

// surfacedFeat returns the remembered features for a knot key from the most recent
// horizon, if it was shown there.
func (s *server) surfacedFeat(key string) (knotFeat, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.lastSurfaced[key]
	return f, ok
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

type knotView struct {
	Kind        string   `json:"kind"`
	Parties     []string `json:"parties"`
	About       string   `json:"about"`
	Explanation string   `json:"explanation"`
	Confidence  float64  `json:"confidence"`
	Votes       int      `json:"votes,omitempty"`
	Samples     int      `json:"samples,omitempty"`
	// Question marks a cross-person knot the agent must present as a QUESTION to its
	// human, not an assertion — the detector cannot certify a cross-person conflict
	// (docs/LEGIBILITY.md stage 0c). Self knots (own drift) are assertable and omit it.
	Question bool `json:"question,omitempty"`
	// Key identifies the coordination problem (kind + sorted parties, wording-
	// independent) so a human can answer it via ettle_respond — the label-capture
	// channel (stage 0c-2). Same key across horizon calls = the same knot recurring.
	Key string `json:"key"`
}

func toKnotView(k ettlemesh.Knot) knotView {
	return knotView{
		Kind: k.Kind, Parties: k.Parties, About: k.About,
		Explanation: k.Explanation, Confidence: k.Confidence,
		Votes: k.Votes, Samples: k.Samples,
		Question: ettlemesh.MultiPerson(k.Parties),
		Key:      knotKey(k.Kind, k.Parties),
	}
}

// knotKey is the wording-independent identity of a coordination problem: kind + its
// distinct parties (lowercased, trimmed, sorted), joined cleanly for use as a tool
// argument. Mirrors eval.KnotKey's semantics but with a readable separator (no NUL),
// since this key is passed back through ettle_respond by an agent.
func knotKey(kind string, parties []string) string {
	ps := make([]string, 0, len(parties))
	seen := map[string]bool{}
	for _, p := range parties {
		n := strings.ToLower(strings.TrimSpace(p))
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		ps = append(ps, n)
	}
	sort.Strings(ps)
	return kind + "|" + strings.Join(ps, "+")
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
// structured knots while a human-facing client sees the summary line.
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
	s.h.upsert(transport.Envelope{Participant: in.Participant, Role: in.Role, Atoms: atoms})
	out := emitOut{Participant: in.Participant, Count: len(atoms), Atoms: atomViews(atoms)}
	return text(fmt.Sprintf("%s emitted %d atom(s) to the horizon (%s).", in.Participant, len(atoms), how)), out, nil
}

// --- ettle_horizon ---

type horizonIn struct {
	Me      string `json:"me,omitempty" jsonschema:"surface only knots involving this participant (their agent's view); empty = the whole team's horizon"`
	Samples int    `json:"samples,omitempty" jsonschema:"independent reconcile samples to vote across; recurrence ranks knots firm vs soft. Default 5; 1 disables voting"`
}

type horizonOut struct {
	Participants []string   `json:"participants"`
	Firm         []knotView `json:"firm"`
	Soft         []knotView `json:"soft"`
	// HeldBack: knots the coupling check judged not-a-real-conflict, surfaced off the
	// agenda so the lead surface can show what was suppressed (legible abstention;
	// docs/LEGIBILITY.md). Omitted when empty.
	HeldBack []knotView `json:"held_back,omitempty"`
	// FloorHeld: how many low-recurrence candidates the abstention floor dropped —
	// a count, not a list (they're noise by design), so a clear horizon stays honest.
	FloorHeld int `json:"floor_held,omitempty"`
}

func (s *server) horizon(ctx context.Context, _ *mcp.CallToolRequest, in horizonIn) (*mcp.CallToolResult, horizonOut, error) {
	envs := s.h.snapshot()
	parts := make([]string, 0, len(envs))
	for _, e := range envs {
		parts = append(parts, e.Participant)
	}
	sort.Strings(parts)

	out := horizonOut{Participants: parts, Firm: []knotView{}, Soft: []knotView{}}

	atoms := transport.Atoms(envs)
	if len(atoms) == 0 {
		// Empty-horizon guard: nothing emitted → no model call.
		return text("the horizon is empty — no atoms emitted yet (call ettle_emit first)."), out, nil
	}

	samples := in.Samples
	if samples == 0 {
		samples = defaultSamples
	}
	knots, floorHeld, err := s.det.ReconcileVoted(ctx, atoms, samples)
	if err != nil {
		return nil, horizonOut{}, err
	}
	// Cross-person coupling check: drop collision/duplication/teamwide knots that
	// bridge people on a shared topic word across independent scopes (no-op if the
	// detector has Ground off). suppressed = what it held back, surfaced off the
	// agenda so the lead surface stays honest (legible abstention; docs/LEGIBILITY.md).
	knots, suppressed, err := s.det.GroundKnots(ctx, knots, atoms)
	if err != nil {
		return nil, horizonOut{}, err
	}
	// Remember exactly what we surface (firm AND soft are both shown, so both are
	// labelable) so a later ettle_respond can join its verdict to the knot's
	// recurrence. The coupling-suppressed and floor-dropped knots are not surfaced,
	// so they are correctly absent here.
	feats := map[string]knotFeat{}
	for _, k := range knots {
		if in.Me != "" && !partiesInclude(k.Parties, in.Me) {
			continue // agent surfaces only its own human's knots, not a shared feed
		}
		v := toKnotView(k)
		feats[v.Key] = knotFeat{Kind: k.Kind, Votes: k.Votes, Samples: k.Samples, Firm: k.Firm()}
		if k.Firm() {
			out.Firm = append(out.Firm, v)
		} else {
			out.Soft = append(out.Soft, v)
		}
	}
	s.rememberSurfaced(feats)
	for _, k := range suppressed {
		if in.Me != "" && !partiesInclude(k.Parties, in.Me) {
			continue
		}
		out.HeldBack = append(out.HeldBack, toKnotView(k))
	}
	out.FloorHeld = floorHeld
	scope := "team"
	if in.Me != "" {
		scope = in.Me
	}
	return text(fmt.Sprintf("horizon (%s): %d firm, %d soft knot(s) across %d participant(s)%s.",
		scope, len(out.Firm), len(out.Soft), len(parts), heldBackNote(len(out.HeldBack), floorHeld))), out, nil
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
	Participant string     `json:"participant"`
	Atoms       []atomView `json:"atoms"`
	Knots       []knotView `json:"knots"`
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
	knots, err := s.det.ReconcileSelf(ctx, atoms)
	if err != nil {
		return nil, selfOut{}, err
	}
	out := selfOut{Participant: in.Participant, Atoms: atomViews(atoms), Knots: []knotView{}}
	for _, k := range knots {
		out.Knots = append(out.Knots, toKnotView(k))
	}
	return text(fmt.Sprintf("%s: %d atom(s), %d self-knot(s).", in.Participant, len(atoms), len(out.Knots))), out, nil
}

// --- ettle_respond (stage 0c-2: capture the human verdict as the calibration label) ---

// Label is one human verdict on a surfaced cross-person knot — the ground-truth
// signal stage 2's calibration loop will consume (docs/LEGIBILITY.md). It is captured
// now, before that loop exists, so the labeled data accrues from day one: a detector
// flag-rate is only calibratable against confirmations from people who saw the work.
type Label struct {
	Key     string `json:"key"`     // knotKey: the coordination problem answered
	Verdict string `json:"verdict"` // real | not_real | handled
	By      string `json:"by"`      // the responder (their own knot)
	Note    string `json:"note,omitempty"`
	TS      string `json:"ts"` // RFC3339, UTC
	// Kind/Votes/Samples/Firm are the surfaced knot's features at capture time — the
	// recurrence signal (Votes of Samples) a future per-kind calibration loop would
	// threshold on, plus the kind and the firm/soft tier it was shown as. Populated
	// when ettle_respond runs against the server that surfaced the knot (the common,
	// same-session path); a cross-session verdict carries Kind only (recovered from
	// the key) with zero recurrence. The loop itself is deliberately unbuilt — this
	// only stops the feature being discarded so the data is learnable if it accrues.
	// omitempty keeps pre-enrichment log lines (which lack these) parseable on read.
	Kind    string `json:"kind,omitempty"`
	Votes   int    `json:"votes,omitempty"`
	Samples int    `json:"samples,omitempty"`
	Firm    bool   `json:"firm,omitempty"`
}

// kindFromKey recovers the knot Kind from a knotKey ("kind|parties"). The Kind is
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
	Me      string `json:"me" jsonschema:"the person responding — answer only your OWN knots"`
	Knot    string `json:"knot" jsonschema:"the knot's key field from ettle_horizon"`
	Verdict string `json:"verdict" jsonschema:"one of: real | not_real | handled"`
	Note    string `json:"note,omitempty" jsonschema:"optional free-text context"`
}

type respondOut struct {
	Recorded bool   `json:"recorded"`
	Key      string `json:"key"`
	Verdict  string `json:"verdict"`
}

// respond records a human's verdict on a cross-person knot. It does NOT mutate the
// horizon or bind anything — it only captures the label (humans stay the deciders;
// the loop that consumes these is stage 2, deliberately unbuilt).
func (s *server) respond(ctx context.Context, _ *mcp.CallToolRequest, in respondIn) (*mcp.CallToolResult, respondOut, error) {
	if s.labels == nil {
		return nil, respondOut{}, fmt.Errorf("label capture is not configured on this server")
	}
	me := strings.TrimSpace(in.Me)
	key := strings.TrimSpace(in.Knot)
	if me == "" || key == "" {
		return nil, respondOut{}, fmt.Errorf("both `me` and `knot` (the key from ettle_horizon) are required")
	}
	v := strings.ToLower(strings.TrimSpace(in.Verdict))
	switch v {
	case "real", "not_real", "handled":
	default:
		return nil, respondOut{}, fmt.Errorf("verdict must be one of real | not_real | handled, got %q", in.Verdict)
	}
	lbl := Label{Key: key, Verdict: v, By: me, Note: in.Note, TS: time.Now().UTC().Format(time.RFC3339)}
	// Enrich with the surfaced knot's features so the verdict is learnable. Same
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
	return text(fmt.Sprintf("recorded: %s judged %q on %s.", me, v, key)),
		respondOut{Recorded: true, Key: key, Verdict: v}, nil
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
		Description: "Reconcile the team's emitted atoms into coordination knots — collisions, duplicated work, stale assumptions, decision-rights gaps — split into firm (worth a look) and soft (worth a question). Pass `me` to see only the knots involving your own human.",
	}, s.horizon)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "ettle_self_check",
		Description: "Useful at N=1, no team needed: distill one person's notes and surface a stale self-assumption — a commitment that contradicts an assumption the same plan rests on. Stateless; does not touch the shared horizon.",
	}, s.selfCheck)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "ettle_respond",
		Description: "Record YOUR human's verdict on a cross-person knot from ettle_horizon (one marked question:true) — real, not_real, or handled. This is the ground-truth signal the system will calibrate against; answer only your own knots, and pass the knot's `key`. It records the label only — it does not bind or decide anything.",
	}, s.respond)

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
func Serve(ctx context.Context, det reconciler, version string) error {
	// Label capture is local-first: an append-only JSONL file in the working dir,
	// overridable by ETTLE_LABELS_PATH. The verdicts are the calibration loop's future
	// input (stage 2); writing them now means the data exists before the loop does.
	path := os.Getenv("ETTLE_LABELS_PATH")
	if path == "" {
		path = "ettle-labels.jsonl"
	}
	s := &server{det: det, h: newHorizon(), labels: newFileLabelSink(path)}
	return newMCPServer(s, version).Run(ctx, &mcp.StdioTransport{})
}
