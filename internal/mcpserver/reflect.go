package mcpserver

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/transport"
)

// This file is the read half of the MCP surface: the L2 directed-model views
// (ettle_mirror, ettle_drift) and the presence view (ettle_room_status). The CLI
// reaches L2 by distilling two directories of notes; from a session there are no
// directories and no notes to re-distill, because the bus already carries atoms.
// So the two rounds come from the bus instead, and none of these tools makes a
// model call.
//
// What a "round" is here: the atoms the bus held when this session first read it
// (the baseline) versus what it holds now. Teammates' models of you were seeded
// from the former; ettle_mirror asks which of those beliefs your later emits have
// already made stale. That makes the mirror answerable mid-session, which is the
// only time it can change what you do.

// selfModels folds a bus snapshot into the shape the L2 layer takes — one
// self-model per participant, keyed by the participant's own display name.
func selfModels(envs []transport.Envelope) map[string][]ettlemesh.Atom {
	out := make(map[string][]ettlemesh.Atom, len(envs))
	for _, e := range envs {
		name := strings.TrimSpace(e.Participant)
		if name == "" || len(e.Atoms) == 0 {
			continue
		}
		out[name] = append(out[name], e.Atoms...)
	}
	return out
}

// rememberBaseline records the first bus read of this session as the previous
// round, once. It is never rewritten: a baseline that chased the current state
// would make every mirror empty, which is the failure mode that would read as
// "nothing anyone believes about you is stale."
func (h *horizon) rememberBaseline(envs []transport.Envelope) {
	h.baseMu.Lock()
	defer h.baseMu.Unlock()
	if h.baseSet {
		return
	}
	h.base = selfModels(envs)
	h.baseSet = true
}

// baseline returns the previous round, or nil if the bus has not been read yet.
func (h *horizon) baseline() map[string][]ettlemesh.Atom {
	h.baseMu.Lock()
	defer h.baseMu.Unlock()
	out := make(map[string][]ettlemesh.Atom, len(h.base))
	for k, v := range h.base {
		out[k] = append([]ettlemesh.Atom(nil), v...)
	}
	return out
}

// mesh builds the two-round L2 state from the bus: the session baseline seeds
// every directed model, the live snapshot supplies the surprise-gated deltas.
// Deterministic and key-free — the model calls the CLI makes are distillation,
// and the bus is already past that.
func (h *horizon) mesh(ctx context.Context) (state *ettlemesh.MeshState, curr map[string][]ettlemesh.Atom, deltas []ettlemesh.Emission, err error) {
	envs, err := h.snapshot(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	curr = selfModels(envs)
	state = ettlemesh.NewMeshState()
	state.Advance(h.baseline())
	deltas = state.Surprise(curr)
	return state, curr, deltas, nil
}

// --- ettle_mirror (what the team's models believe about you) ---

type mirrorIn struct {
	Me         string `json:"me" jsonschema:"the person the beliefs are ABOUT — your own human"`
	ByObserver bool   `json:"by_observer,omitempty" jsonschema:"attribute each belief to the teammate holding it; default false (coarsened — the belief, not who holds it)"`
}

// mirrorBelief is one thing the team currently believes about the subject.
type mirrorBelief struct {
	Type     string `json:"type"`
	Subject  string `json:"subject"`
	Content  string `json:"content"`
	Observer string `json:"observer,omitempty"`
	Stale    string `json:"stale,omitempty" jsonschema:"drifted | maybe_stale — absent when the belief still matches"`
	NowHold  string `json:"you_now_hold,omitempty"`
}

type mirrorOut struct {
	Me       string         `json:"me"`
	Beliefs  []mirrorBelief `json:"beliefs"`
	Stale    int            `json:"stale"`
	Coarse   bool           `json:"coarsened" jsonschema:"true when attribution was withheld"`
	Contest  string         `json:"contest"`
	Observed int            `json:"teammates_modeling_you"`
}

// mirror is the read side of the one-way mirror (docs/LEGIBILITY.md stage 1b),
// reachable from inside a session. Attribution is COARSENED by default for the
// same reason the CLI coarsens it: "alice believes X about you" surfaces alice's
// private model, so turning the mirror around must not point a new surveillance
// surface at the modelers.
func (s *server) mirror(ctx context.Context, _ *mcp.CallToolRequest, in mirrorIn) (*mcp.CallToolResult, mirrorOut, error) {
	me := strings.TrimSpace(in.Me)
	if me == "" {
		return nil, mirrorOut{}, fmt.Errorf("`me` is required — the mirror shows the beliefs held ABOUT one person")
	}
	state, curr, _, err := s.h.mesh(ctx)
	if err != nil {
		return nil, mirrorOut{}, err
	}

	observers := make([]string, 0, len(curr))
	for o := range curr {
		if !ettlemesh.SamePerson(o, me) {
			observers = append(observers, o)
		}
	}
	sort.Strings(observers)

	out := mirrorOut{Me: me, Coarse: !in.ByObserver, Beliefs: []mirrorBelief{}}
	seen := map[string]int{} // slot identity -> index in out.Beliefs, for the coarsened union
	for _, o := range observers {
		m, ok := state.ModelOf(o, me)
		if !ok || len(m.Beliefs) == 0 {
			continue
		}
		out.Observed++
		stale := map[string]ettlemesh.Drift{}
		for _, d := range ettlemesh.StaleBeliefs(m, curr[me]) {
			stale[beliefIdent(d.Believed)] = d
		}
		for _, b := range ettlemesh.Canonical(m.Beliefs) {
			mb := mirrorBelief{Type: string(b.Typ), Subject: b.Subject, Content: b.Content}
			if in.ByObserver {
				mb.Observer = o
			}
			if d, ok := stale[beliefIdent(b)]; ok {
				if d.Kind == ettlemesh.DriftDrifted && d.Actual != nil {
					mb.Stale, mb.NowHold = "drifted", d.Actual.Content
				} else {
					// Hedged on purpose: a re-distill reword is indistinguishable from
					// a real drop at the structural layer (ettlemesh/directed.go).
					mb.Stale = "maybe_stale"
				}
			}
			if in.ByObserver {
				out.Beliefs = append(out.Beliefs, mb)
				continue
			}
			// Coarsened: one entry per slot across the whole team, and a belief that
			// is stale for anyone is reported stale.
			id := beliefIdent(b)
			if i, dup := seen[id]; dup {
				if out.Beliefs[i].Stale == "" && mb.Stale != "" {
					out.Beliefs[i].Stale, out.Beliefs[i].NowHold = mb.Stale, mb.NowHold
				}
				continue
			}
			seen[id] = len(out.Beliefs)
			out.Beliefs = append(out.Beliefs, mb)
		}
	}
	for _, b := range out.Beliefs {
		if b.Stale != "" {
			out.Stale++
		}
	}
	// Said on every call because it is the honest limit of this whole layer, and a
	// reader who only sees the beliefs will assume they can be corrected.
	out.Contest = "reading is built; contesting is not — a wrong belief here is retired by re-emitting, not by disputing it (the calibration loop is unbuilt)"

	if out.Observed == 0 {
		return text(fmt.Sprintf("nobody holds a model of %s yet — the mirror needs a teammate on the bus and at least one emit since this session started reading it.", me)), out, nil
	}
	return text(fmt.Sprintf("%d belief(s) held about %s, %d stale. %s", len(out.Beliefs), me, out.Stale, out.Contest)), out, nil
}

func beliefIdent(a ettlemesh.Atom) string {
	return string(a.Typ) + "|" + a.Subject + "|" + a.Content
}

// --- ettle_drift (the team-wide directed view) ---

type driftIn struct {
	Me string `json:"me,omitempty" jsonschema:"show only the deltas routed to or from this person; empty = the whole team"`
}

type driftDelta struct {
	Subject   string   `json:"subject" jsonschema:"whose state changed"`
	Observers []string `json:"routed_to" jsonschema:"the teammates whose model of the subject went stale"`
	Atoms     []string `json:"atoms"`
}

type driftOut struct {
	Deltas   []driftDelta `json:"deltas"`
	Routed   int          `json:"routed"`
	Possible int          `json:"broadcast_would_be" jsonschema:"how many deliveries an unconditional broadcast would have made"`
	Note     string       `json:"note"`
}

// drift is the emit side of L2: which changes since this session's baseline would
// leave a teammate's model stale, and therefore who each one is routed to. The
// savings number is the point of the layer — it is what a broadcast would have
// cost minus what the gate actually sent.
func (s *server) drift(ctx context.Context, _ *mcp.CallToolRequest, in driftIn) (*mcp.CallToolResult, driftOut, error) {
	_, curr, deltas, err := s.h.mesh(ctx)
	if err != nil {
		return nil, driftOut{}, err
	}
	me := strings.TrimSpace(in.Me)

	bySubject := map[string]*driftDelta{}
	var order []string
	for _, e := range deltas {
		if me != "" && !ettlemesh.SamePerson(e.Subject, me) && !ettlemesh.SamePerson(e.Observer, me) {
			continue
		}
		d, ok := bySubject[e.Subject]
		if !ok {
			d = &driftDelta{Subject: e.Subject}
			bySubject[e.Subject] = d
			order = append(order, e.Subject)
		}
		d.Observers = append(d.Observers, e.Observer)
		for _, a := range e.Atoms {
			line := fmt.Sprintf("[%s] %s — %s", a.Typ, a.Subject, a.Content)
			if !contains(d.Atoms, line) {
				d.Atoms = append(d.Atoms, line)
			}
		}
	}
	sort.Strings(order)
	out := driftOut{Deltas: []driftDelta{}}
	for _, k := range order {
		d := bySubject[k]
		sort.Strings(d.Observers)
		out.Deltas = append(out.Deltas, *d)
		out.Routed += len(d.Observers)
	}
	// A broadcast sends every changed subject to every other participant.
	if n := len(curr); n > 1 {
		out.Possible = len(out.Deltas) * (n - 1)
	}
	out.Note = "a round here is this session: the baseline is what the bus held when this session first read it"
	return text(fmt.Sprintf("%d subject(s) changed, %d routed delivery(ies) (broadcast would be %d).", len(out.Deltas), out.Routed, out.Possible)), out, nil
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// --- ettle_room_status (presence off the bus) ---

type roomStatusIn struct{}

type roomMember struct {
	Participant string   `json:"participant"`
	Role        string   `json:"role,omitempty"`
	Freshness   string   `json:"freshness" jsonschema:"coarse on purpose — recently | today | yesterday | Nd ago"`
	WorkingOn   []string `json:"working_on,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
	CommittedTo []string `json:"committed_to,omitempty"`
	Assuming    []string `json:"assuming,omitempty"`
}

type roomStatusOut struct {
	Present int          `json:"present"`
	Members []roomMember `json:"members"`
}

// roomStatus is the presence view, read straight off the bus: who has published
// and what each is on. No tangle detection and no model call, so an agent can ask
// it as often as it likes.
//
// Freshness is deliberately coarse. A per-minute age across a whole team is a
// working-patterns feed — who starts at six, who was up at three — and presence
// is not worth that.
func (s *server) roomStatus(ctx context.Context, _ *mcp.CallToolRequest, _ roomStatusIn) (*mcp.CallToolResult, roomStatusOut, error) {
	envs, err := s.h.snapshot(ctx)
	if err != nil {
		return nil, roomStatusOut{}, err
	}
	now := time.Now().UTC()
	out := roomStatusOut{Members: []roomMember{}}
	for _, e := range envs {
		m := roomMember{Participant: e.Participant, Role: e.Role, Freshness: freshness(e.EmittedAt, now)}
		for _, a := range e.Atoms {
			line := a.Content
			if a.Subject != "" {
				line = a.Subject + " — " + a.Content
			}
			switch a.Typ {
			case ettlemesh.Intent:
				m.WorkingOn = append(m.WorkingOn, line)
			case ettlemesh.Dependency:
				m.DependsOn = append(m.DependsOn, line)
			case ettlemesh.Commitment:
				m.CommittedTo = append(m.CommittedTo, line)
			case ettlemesh.Assumption:
				m.Assuming = append(m.Assuming, line)
			}
		}
		out.Members = append(out.Members, m)
	}
	sort.Slice(out.Members, func(i, j int) bool { return out.Members[i].Participant < out.Members[j].Participant })
	out.Present = len(out.Members)
	if out.Present == 0 {
		return text("nobody has published to this room yet."), out, nil
	}
	return text(fmt.Sprintf("%d present on the bus.", out.Present)), out, nil
}

// freshness buckets an RFC3339 emit time. Coarse by design — see roomStatus.
func freshness(emittedAt string, now time.Time) string {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(emittedAt))
	if err != nil {
		return "unknown"
	}
	switch d := now.Sub(t); {
	case d < 2*time.Hour:
		return "recently"
	case d < 24*time.Hour:
		return "today"
	case d < 48*time.Hour:
		return "yesterday"
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	}
}
