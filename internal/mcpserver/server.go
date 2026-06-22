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
	"fmt"
	"sort"
	"strings"
	"sync"

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
	det reconciler
	h   *horizon
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
}

func toKnotView(k ettlemesh.Knot) knotView {
	return knotView{
		Kind: k.Kind, Parties: k.Parties, About: k.About,
		Explanation: k.Explanation, Confidence: k.Confidence,
		Votes: k.Votes, Samples: k.Samples,
		Question: crossPerson(k.Parties),
	}
}

func partiesInclude(parties []string, me string) bool {
	for _, p := range parties {
		if ettlemesh.SamePerson(p, me) {
			return true
		}
	}
	return false
}

// crossPerson reports whether a knot names at least two DISTINCT people — the knots
// presented as questions rather than assertions (stage 0c).
func crossPerson(parties []string) bool {
	if len(parties) < 2 {
		return false
	}
	for _, p := range parties[1:] {
		if !ettlemesh.SamePerson(p, parties[0]) {
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
	Notes       string `json:"notes" jsonschema:"the person's raw working notes or reasoning-in-progress; distilled server-side into typed atoms — only the typed atoms are stored, the raw text is dropped"`
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
	if strings.TrimSpace(in.Notes) == "" {
		return nil, emitOut{}, fmt.Errorf("notes is required")
	}
	// Distill applies the privacy boundary (contextual-integrity prompt + the
	// deterministic secret scrub + structural caps). Only the typed atoms are
	// kept; the raw notes are never stored.
	atoms, err := s.det.Distill(ctx, in.Participant, in.Role, in.Notes, nil)
	if err != nil {
		return nil, emitOut{}, err
	}
	s.h.upsert(transport.Envelope{Participant: in.Participant, Role: in.Role, Atoms: atoms})
	out := emitOut{Participant: in.Participant, Count: len(atoms), Atoms: atomViews(atoms)}
	return text(fmt.Sprintf("%s emitted %d atom(s) to the horizon (raw notes dropped).", in.Participant, len(atoms))), out, nil
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
	for _, k := range knots {
		if in.Me != "" && !partiesInclude(k.Parties, in.Me) {
			continue // agent surfaces only its own human's knots, not a shared feed
		}
		v := toKnotView(k)
		if k.Firm() {
			out.Firm = append(out.Firm, v)
		} else {
			out.Soft = append(out.Soft, v)
		}
	}
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

// newMCPServer builds the MCP server with the three tools registered. Shared by
// Serve (stdio) and the in-memory round-trip test.
func newMCPServer(s *server, version string) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "ettle", Version: version}, nil)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "ettle_emit",
		Description: "Emit YOUR OWN human's working notes to the team coordination horizon. The server distills them through the privacy boundary into typed atoms (only the atoms are stored; raw notes are dropped) and returns exactly what crossed. Emit only your own person — never a teammate.",
	}, s.emit)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "ettle_horizon",
		Description: "Reconcile the team's emitted atoms into coordination knots — collisions, duplicated work, stale assumptions, decision-rights gaps — split into firm (worth a look) and soft (worth a question). Pass `me` to see only the knots involving your own human.",
	}, s.horizon)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "ettle_self_check",
		Description: "Useful at N=1, no team needed: distill one person's notes and surface a stale self-assumption — a commitment that contradicts an assumption the same plan rests on. Stateless; does not touch the shared horizon.",
	}, s.selfCheck)

	return srv
}

// Serve registers the tools and runs the server over stdio until ctx is done.
// version is passed in because mcpserver cannot import package main (where
// buildVersion lives). Stdio discipline: stdout is the JSON-RPC channel, so
// callers must keep all logging on stderr.
func Serve(ctx context.Context, det reconciler, version string) error {
	s := &server{det: det, h: newHorizon()}
	return newMCPServer(s, version).Run(ctx, &mcp.StdioTransport{})
}
