// Package transport — Linear adapter (always compiled; net/http + encoding/json
// only, so unlike the NATS adapter it needs no build tag).
//
// Linear (linear.app) is a SaaS issue tracker whose GraphQL API exposes
// project-scoped Documents. ettle rides it as a coordination bus for the team
// that already lives in Linear + Claude Code: the room is a Linear project, each
// participant owns one Document, and that Document's content is their current
// envelope. Storage is REPLACE-CURRENT (documentUpdate overwrites the content in
// place), so the footprint is N documents for N people — the same bounded class
// as DirBus, and unlike leat there is no per-emit accumulation.
//
// Measured, not assumed (2026-08-04): API-driven documentUpdate does NOT accrue
// visible content-history snapshots — 12 rapid updates left documentContentHistory
// at 0 entries — so no doc-rotation is needed to keep Linear's side bounded.
// Linear's interactive editor-session history is a separate surface we never
// touch. This is an observation of today's API, not a documented guarantee; if
// Linear ever snapshots API writes, the mitigation is a periodic delete+recreate,
// deliberately not built ahead of that need.
//
// Mapping ettle <-> Linear:
//   - room        → a Linear project named "ettle-<room>" (created if absent)
//   - participant → a Document titled "ettle/<slug(participant)>", the AUTHORITATIVE
//     identity on Collect (an in-content participant whose slug disagrees is
//     overridden and warned — the same cheap anti-spoof as DirBus's filename
//     identity; a matching in-content name is kept so display casing survives)
//   - envelope    → the Document content (marshaled ettle Envelope JSON)
//   - Publish     → upsert this participant's document (create, or update in place)
//   - Collect     → list the project's documents, unmarshal each content
//
// Honest limits: a single room API token means every write's Linear actor is the
// token owner, NOT the participant — so identity rides the title/content and
// Linear cannot corroborate it (leat's git-author check is strictly stronger).
// And a participant whose document isn't in the project is invisible (Collect
// reports who is PRESENT, not an out-of-band roster) — same as DirBus.
//
// Politeness (we are a guest on their platform): every request carries a
// User-Agent identifying ettle, a 429 is surfaced as a distinct rate-limit error
// rather than a generic failure, and the write shape is deliberately light — one
// project, N documents, no per-emit litter. Publish is two calls (find + write);
// with the emit gate upstream keeping publishes rare, that stays well under any
// sane rate budget.
package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	linearEndpoint    = "https://api.linear.app/graphql"
	linearTitlePrefix = "ettle/"
)

// storedDoc is one document as the store hands it back: the ettle title and the
// raw content (a marshaled Envelope).
type storedDoc struct{ Title, Content string }

// docStore is the minimal document backend LinearBus needs, factored out so unit
// tests use an in-memory fake and never touch the network. The real
// implementation (linearDocStore) is GraphQL over HTTP.
type docStore interface {
	// upsert sets the document titled `title` to `content` within the room,
	// creating it if absent (replace-current).
	upsert(ctx context.Context, title, content string) error
	// list returns every document in the room.
	list(ctx context.Context) ([]storedDoc, error)
	close() error
}

// LinearBus is an atom transport over Linear documents: the room is a Linear
// project, each participant is one document, and its content is that person's
// latest envelope, so Collect returns the latest envelope per person.
type LinearBus struct {
	store docStore

	mu       sync.Mutex
	warnings []string
}

func newLinearBusOn(store docStore) *LinearBus { return &LinearBus{store: store} }

// NewLinearBus builds a Linear-backed transport: it resolves (or creates) the
// project for `room`, then returns a bus that writes one document per
// participant. teamID is required only when the project must be created (Linear
// needs a team to own a new project); if the project already exists it is
// ignored. version tags the User-Agent so Linear can see it is ettle calling.
func NewLinearBus(apiKey, room, teamID, version string) (*LinearBus, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("transport/linear: empty API key (set LINEAR_API_KEY)")
	}
	store := &linearDocStore{
		http:     &http.Client{Timeout: 30 * time.Second},
		apiKey:   strings.TrimSpace(apiKey),
		endpoint: linearEndpoint,
		ua:       "ettle/" + version + " (+https://github.com/justinstimatze/ettle)",
	}
	pid, err := store.resolveProject(context.Background(), room, strings.TrimSpace(teamID))
	if err != nil {
		return nil, err
	}
	store.projectID = pid
	return newLinearBusOn(store), nil
}

// Publish upserts this participant's document, replacing any prior content, so
// their latest atom set overwrites the earlier one.
func (b *LinearBus) Publish(ctx context.Context, env Envelope) error {
	if strings.TrimSpace(env.Participant) == "" {
		return fmt.Errorf("transport/linear: envelope has no participant")
	}
	// Stamp emit time if unset (display/staleness only, never used for ordering).
	if env.EmittedAt == "" {
		env.EmittedAt = time.Now().UTC().Format(time.RFC3339)
	}
	env.V = envelopeV
	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("transport/linear: marshal %s: %w", env.Participant, err)
	}
	title := linearTitlePrefix + slug(env.Participant)
	if err := b.store.upsert(ctx, title, string(body)); err != nil {
		return fmt.Errorf("transport/linear: upsert %s: %w", env.Participant, err)
	}
	return nil
}

// Collect returns the latest envelope per participant: it lists the project's
// documents, ignores any that are not ettle documents (a hand-authored doc in
// the same project), unmarshals each content, and treats the TITLE slug as the
// authoritative identity (an in-content claim that disagrees is overridden and
// warned; a matching one is kept so display casing survives).
func (b *LinearBus) Collect(ctx context.Context) ([]Envelope, error) {
	docs, err := b.store.list(ctx)
	if err != nil {
		return nil, fmt.Errorf("transport/linear: list: %w", err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.warnings = nil
	out := make([]Envelope, 0, len(docs))
	for _, d := range docs {
		want, ok := strings.CutPrefix(d.Title, linearTitlePrefix)
		if !ok {
			continue // not an ettle document; leave the team's own docs alone
		}
		var env Envelope
		if err := json.Unmarshal([]byte(d.Content), &env); err != nil {
			b.warnings = append(b.warnings, fmt.Sprintf("skipped %q: unparseable content", d.Title))
			continue
		}
		if env.Participant == "" {
			env.Participant = want
		} else if slug(env.Participant) != want {
			b.warnings = append(b.warnings, fmt.Sprintf(
				"%q claims participant %q; using title identity %q", d.Title, env.Participant, want))
			env.Participant = want
		}
		out = append(out, env)
	}
	return out, nil
}

func (b *LinearBus) Close() error { return b.store.close() }

// TeamScope is one team that owns the room's project, with Linear's visibility for
// it. Linear's "public" means visible to the whole WORKSPACE, not the internet —
// there is no internet-public Linear project — so this is a disclosure of audience,
// not a leak check. (The GitHub adapter's isPrivate refusal is the opposite case:
// there, public really does mean the world.)
type TeamScope struct{ Name, Key, Visibility string }

// audienceReporter is optional: an in-memory store used by tests does not implement
// it, and Audience then reports nothing rather than forcing every fake to grow a method.
type audienceReporter interface {
	audience(ctx context.Context) ([]TeamScope, error)
}

// Audience returns the teams that own the room's project, so a caller can tell the
// user who can read what they are about to publish.
func (b *LinearBus) Audience(ctx context.Context) ([]TeamScope, error) {
	ar, ok := b.store.(audienceReporter)
	if !ok {
		return nil, nil
	}
	return ar.audience(ctx)
}

// Warnings returns a copy of the non-fatal issues from the last Collect
// (unparseable documents, identity mismatches), matching DirBus/LeatBus so the
// driver can surface them the same way.
func (b *LinearBus) Warnings() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.warnings) == 0 {
		return nil
	}
	return append([]string(nil), b.warnings...)
}

// linearDocStore is the GraphQL-over-HTTP backend.
type linearDocStore struct {
	http      *http.Client
	apiKey    string
	endpoint  string
	ua        string
	projectID string
	// bearer selects the auth scheme: false (default) sends the key raw, as a Linear
	// MEMBER key wants (documents, agent-activity reads); true sends "Bearer <key>",
	// as an OAuth APP-ACTOR token wants (agent-activity writes — the escalation-emit
	// path posts as the app). The member key can read agent activities but cannot post
	// them, so the two auth modes are genuinely different tokens, not a style choice.
	bearer bool
}

// do executes one GraphQL request, decoding data into out (which may be nil).
// It maps a 429 to a distinct rate-limit error and surfaces GraphQL errors.
func (s *linearDocStore) do(ctx context.Context, query string, vars map[string]any, out any) error {
	reqBody, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	if s.bearer {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	} else {
		req.Header.Set("Authorization", s.apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", s.ua) // identify ourselves — a guest on their platform
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("linear rate-limited (429): %s", strings.TrimSpace(string(raw)))
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("linear http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if len(envelope.Errors) > 0 {
		return fmt.Errorf("linear graphql: %s", envelope.Errors[0].Message)
	}
	if out != nil {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return fmt.Errorf("decode data: %w", err)
		}
	}
	return nil
}

// audience reads the visibility of the teams owning the room's project.
func (s *linearDocStore) audience(ctx context.Context) ([]TeamScope, error) {
	var q struct {
		Project struct {
			Teams struct {
				Nodes []TeamScope `json:"nodes"`
			} `json:"teams"`
		} `json:"project"`
	}
	const query = `query($p:String!){ project(id:$p){ teams(first:10){ nodes{ name key visibility } } } }`
	if err := s.do(ctx, query, map[string]any{"p": s.projectID}, &q); err != nil {
		return nil, err
	}
	return q.Project.Teams.Nodes, nil
}

// resolveProject finds the project named "ettle-<room>", creating it under teamID
// if absent. teamID may be empty only when the project already exists.
func (s *linearDocStore) resolveProject(ctx context.Context, room, teamID string) (string, error) {
	name := "ettle-" + SanitizeID(room)
	var q struct {
		Projects struct {
			Nodes []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"nodes"`
		} `json:"projects"`
	}
	if err := s.do(ctx, `query{ projects(first: 250){ nodes{ id name } } }`, nil, &q); err != nil {
		return "", fmt.Errorf("transport/linear: list projects: %w", err)
	}
	for _, p := range q.Projects.Nodes {
		if p.Name == name {
			return p.ID, nil
		}
	}
	if teamID == "" {
		return "", fmt.Errorf("transport/linear: project %q not found and no LINEAR_TEAM_ID set to create it", name)
	}
	var m struct {
		ProjectCreate struct {
			Success bool `json:"success"`
			Project struct {
				ID string `json:"id"`
			} `json:"project"`
		} `json:"projectCreate"`
	}
	vars := map[string]any{"i": map[string]any{
		"name":        name,
		"teamIds":     []string{teamID},
		"description": "ettle coordination room — each document is one participant's current envelope",
	}}
	if err := s.do(ctx, `mutation($i:ProjectCreateInput!){ projectCreate(input:$i){ success project{ id } } }`, vars, &m); err != nil {
		return "", fmt.Errorf("transport/linear: create project %q: %w", name, err)
	}
	if !m.ProjectCreate.Success || m.ProjectCreate.Project.ID == "" {
		return "", fmt.Errorf("transport/linear: create project %q returned no id", name)
	}
	return m.ProjectCreate.Project.ID, nil
}

// rawDoc is a project document with its id, needed by upsert to decide
// create-vs-update.
type rawDoc struct{ id, title, content string }

func (s *linearDocStore) listRaw(ctx context.Context) ([]rawDoc, error) {
	var q struct {
		Project struct {
			Documents struct {
				Nodes []struct {
					ID      string `json:"id"`
					Title   string `json:"title"`
					Content string `json:"content"`
				} `json:"nodes"`
			} `json:"documents"`
		} `json:"project"`
	}
	vars := map[string]any{"id": s.projectID}
	if err := s.do(ctx, `query($id:String!){ project(id:$id){ documents{ nodes{ id title content } } } }`, vars, &q); err != nil {
		return nil, err
	}
	out := make([]rawDoc, 0, len(q.Project.Documents.Nodes))
	for _, n := range q.Project.Documents.Nodes {
		out = append(out, rawDoc{id: n.ID, title: n.Title, content: n.Content})
	}
	return out, nil
}

func (s *linearDocStore) list(ctx context.Context) ([]storedDoc, error) {
	raw, err := s.listRaw(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]storedDoc, 0, len(raw))
	for _, d := range raw {
		out = append(out, storedDoc{Title: d.title, Content: d.content})
	}
	return out, nil
}

// upsert finds this title's document and updates it in place, or creates it. The
// find is a project-scoped list (O(participants)); with publishes kept rare by
// the emit gate, the extra read is a fair price for correctness over a cached id
// that another writer could have invalidated.
func (s *linearDocStore) upsert(ctx context.Context, title, content string) error {
	docs, err := s.listRaw(ctx)
	if err != nil {
		return err
	}
	var id string
	for _, d := range docs {
		if d.title == title {
			id = d.id
			break
		}
	}
	if id == "" {
		var m struct {
			DocumentCreate struct {
				Success bool `json:"success"`
			} `json:"documentCreate"`
		}
		vars := map[string]any{"i": map[string]any{"title": title, "content": content, "projectId": s.projectID}}
		if err := s.do(ctx, `mutation($i:DocumentCreateInput!){ documentCreate(input:$i){ success } }`, vars, &m); err != nil {
			return err
		}
		if !m.DocumentCreate.Success {
			return fmt.Errorf("documentCreate %q returned success=false", title)
		}
		return nil
	}
	var m struct {
		DocumentUpdate struct {
			Success bool `json:"success"`
		} `json:"documentUpdate"`
	}
	vars := map[string]any{"id": id, "i": map[string]any{"content": content}}
	if err := s.do(ctx, `mutation($id:String!,$i:DocumentUpdateInput!){ documentUpdate(id:$id,input:$i){ success } }`, vars, &m); err != nil {
		return err
	}
	if !m.DocumentUpdate.Success {
		return fmt.Errorf("documentUpdate %q returned success=false", title)
	}
	return nil
}

func (s *linearDocStore) close() error { return nil }
