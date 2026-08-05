package transport

// Linear agent-activity reader — the RECEIVE half of the Linear agent path.
//
// The doc bus (linear.go) lets ettle publish typed atoms as documents. But a
// teammate who never installs ettle contributes a different way: they reply in
// Linear's native agent UI (an agent session on an issue), and their prose lands
// as an AgentActivity of kind "prompt". This reader pulls those human replies so
// a local session can distill each one — locally, under that teammate's identity —
// and publish the resulting atoms to the room bus. Raw prose never touches the
// bus; distillation happens on the machine that pulls (docs/CONCEPT.md boundary).
//
// Auth: this reads with a plain member API key (LINEAR_API_KEY) — the same
// credential the doc bus uses. No OAuth app-actor token is needed to READ agent
// activities (proven live against IWS-33, 2026-08-04). The EMIT half (surfacing
// ettle's knots as elicitations) is a separate, app-token path and is not here.
//
// Incremental: callers keep a cursor (the createdAt of the newest activity they
// have seen) and pass it as `since`; the query filters createdAt > since server
// side, so a steady-state pull is one cheap request that returns nothing new.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// agentPullPage is the single-request cap (Linear's max page). More unseen
// activities than this in one gap is surfaced via Pull's `more`, never dropped
// silently — the next pull (cursor advanced) continues.
const agentPullPage = 250

// promptContentType is the AgentActivityContent __typename for a human's reply —
// as opposed to ettle's own emits (Elicitation) or thoughts/actions.
const promptContentType = "AgentActivityPromptContent"

// Reply is one teammate reply pulled off a Linear agent session: the raw prose a
// human posted, tagged with who and when, before any distillation.
type Reply struct {
	Author    string // the human's Linear display name — the participant to attribute atoms to
	Body      string // the reply prose (distilled locally; never parked on the bus)
	IssueID   string // the issue the session hangs on (e.g. "IWS-33"), for context/filtering
	SessionID string // the agent session id
	CreatedAt string // RFC3339 UTC; the incremental cursor
}

// rawActivity is one agent activity as the source hands it back, before ettle
// decides whether it is a human reply worth keeping.
type rawActivity struct {
	Typename  string
	Body      string
	Author    string
	IssueID   string
	SessionID string
	CreatedAt string
}

// replySource is the minimal backend the reader needs, factored so tests inject a
// fake and never touch the network (mirrors docStore in linear.go).
type replySource interface {
	// fetch returns activities newer than `since` (empty = all), and `more` true
	// if the page filled (possibly more beyond this request).
	fetch(ctx context.Context, since string) (acts []rawActivity, more bool, err error)
}

// LinearAgentReader pulls human replies off Linear's agent sessions.
type LinearAgentReader struct {
	src replySource
}

// NewLinearAgentReader builds a reader over the live GraphQL backend using a
// member API key. version tags the User-Agent so Linear sees ettle calling.
func NewLinearAgentReader(apiKey, version string) *LinearAgentReader {
	// Reuse linearDocStore purely for its do() GraphQL executor — projectID is
	// unused by the agentActivities query, so nothing else here needs a project.
	return &LinearAgentReader{src: &linearAgentSource{gql: &linearDocStore{
		http:     &http.Client{Timeout: 30 * time.Second},
		apiKey:   strings.TrimSpace(apiKey),
		endpoint: linearEndpoint,
		ua:       "ettle/" + version + " (+https://github.com/justinstimatze/ettle)",
	}}}
}

// newLinearAgentReaderOn wraps an injected source (tests).
func newLinearAgentReaderOn(src replySource) *LinearAgentReader { return &LinearAgentReader{src: src} }

// Pull returns human replies newer than `since` (empty = all) plus the new cursor
// and `more` (the page filled — re-pull after advancing to get the rest).
//
// The cursor advances over EVERY fetched activity (prompt or not), so ettle's own
// elicitations aren't re-fetched forever; only the prompt activities are returned
// as replies. The caller distills each reply and persists the returned cursor
// only after the atoms are published, so a crash mid-run re-pulls rather than
// loses replies.
func (r *LinearAgentReader) Pull(ctx context.Context, since string) (replies []Reply, cursor string, more bool, err error) {
	acts, more, err := r.src.fetch(ctx, since)
	if err != nil {
		return nil, since, false, err
	}
	cursor = since
	for _, a := range acts {
		if newerRFC3339(a.CreatedAt, cursor) {
			cursor = a.CreatedAt
		}
		if a.Typename != promptContentType {
			continue // ettle's own emit, a thought, an action — not a human reply
		}
		body := strings.TrimSpace(a.Body)
		if body == "" {
			continue
		}
		replies = append(replies, Reply{
			Author:    a.Author,
			Body:      body,
			IssueID:   a.IssueID,
			SessionID: a.SessionID,
			CreatedAt: a.CreatedAt,
		})
	}
	return replies, cursor, more, nil
}

// newerRFC3339 reports whether a is a strictly later timestamp than b. b may be
// empty (first run), in which case any non-empty a is newer. Falls back to a
// lexical compare only if a timestamp won't parse (same-format API strings sort
// lexically anyway).
func newerRFC3339(a, b string) bool {
	if b == "" {
		return a != ""
	}
	ta, ea := time.Parse(time.RFC3339, a)
	tb, eb := time.Parse(time.RFC3339, b)
	if ea == nil && eb == nil {
		return ta.After(tb)
	}
	return a > b
}

// linearAgentSource is the live GraphQL backend. It reuses linearDocStore.do (a
// generic GraphQL-over-HTTP executor); projectID stays empty and unused.
type linearAgentSource struct{ gql *linearDocStore }

func (s *linearAgentSource) fetch(ctx context.Context, since string) ([]rawActivity, bool, error) {
	vars := map[string]any{"n": agentPullPage}
	if strings.TrimSpace(since) != "" {
		// createdAt > since: only activities we haven't seen. gt is confirmed on
		// AgentActivityFilter.createdAt (DateComparator).
		vars["f"] = map[string]any{"createdAt": map[string]any{"gt": since}}
	}
	var q struct {
		AgentActivities struct {
			Nodes []struct {
				ID        string `json:"id"`
				CreatedAt string `json:"createdAt"`
				User      struct {
					Name string `json:"name"`
				} `json:"user"`
				AgentSession struct {
					ID    string `json:"id"`
					Issue struct {
						Identifier string `json:"identifier"`
					} `json:"issue"`
				} `json:"agentSession"`
				Content struct {
					Typename string `json:"__typename"`
					Body     string `json:"body"`
				} `json:"content"`
			} `json:"nodes"`
		} `json:"agentActivities"`
	}
	// Only the prompt fragment carries body; other content kinds return just
	// __typename (Pull drops them) — so this yields human replies, not ettle's own
	// elicitations/thoughts.
	const query = `query($f:AgentActivityFilter,$n:Int){ agentActivities(first:$n, filter:$f, orderBy:createdAt){ nodes{ id createdAt user{ name } agentSession{ id issue{ identifier } } content{ __typename ... on AgentActivityPromptContent{ body } } } } }`
	if err := s.gql.do(ctx, query, vars, &q); err != nil {
		return nil, false, fmt.Errorf("transport/linear: pull agent activities: %w", err)
	}
	out := make([]rawActivity, 0, len(q.AgentActivities.Nodes))
	for _, n := range q.AgentActivities.Nodes {
		out = append(out, rawActivity{
			Typename:  n.Content.Typename,
			Body:      n.Content.Body,
			Author:    n.User.Name,
			IssueID:   n.AgentSession.Issue.Identifier,
			SessionID: n.AgentSession.ID,
			CreatedAt: n.CreatedAt,
		})
	}
	more := len(q.AgentActivities.Nodes) >= agentPullPage
	return out, more, nil
}
