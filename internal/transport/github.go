// Package transport — GitHub adapter (always compiled; net/http + encoding/json
// only, so like the Linear adapter it needs no build tag).
//
// This is the Linear docs-as-bus shape for a team that lives in GitHub instead:
// the room is a repository **Discussion**, each participant owns one comment on
// it, and that comment's body carries their current envelope. Storage is
// REPLACE-CURRENT (updateDiscussionComment overwrites in place), so the footprint
// is N comments for N people — the same bounded class as LinearBus and DirBus,
// and unlike leat there is no per-emit accumulation.
//
// Why not just use the git bus (leat) for GitHub teams? Because leat needs a
// SEPARATE private repo created, cloned, and seeded before anything works, while
// a Discussion lives in the repository the team is already standing in and rides
// credentials every developer machine already has. That setup delta is the whole
// reason this adapter exists.
//
// Mapping ettle <-> GitHub:
//   - room        → a Discussion titled "ettle/<room>" (created if absent)
//   - participant → one comment, identified by a leading "<!-- ettle:<slug> -->"
//     marker, which is the AUTHORITATIVE identity on Collect
//   - envelope    → the marshaled ettle Envelope, in a fenced json block so the
//     Discussion still renders readably to a human who opens it
//   - Publish     → upsert this participant's comment (add, or update in place)
//   - Collect     → list the discussion's comments, unmarshal each body
//
// PRIVATE REPOSITORIES ONLY, enforced, not advised. A public repo's Discussions
// are readable by anyone on the internet, and the bus carries every participant's
// intents, commitments, and assumptions — internal working state. A Linear project
// is workspace-scoped and a private repo's Discussion is collaborator-scoped, which
// are comparable audiences; a public repo is a categorically different one. So
// NewGitHubBus refuses a public repository outright rather than warning, and there
// is deliberately no override flag: the contextual-privacy boundary is a design
// invariant (docs/CONCEPT.md), and a flag to switch it off is a flag that gets
// switched off. The check runs at construction, so a repo flipped to public later
// fails the next publish loudly instead of leaking quietly.
//
// Residual risk this canNOT close: a repo that is private today can be made public
// tomorrow, and the Discussion history goes with it. Nothing in the API prevents
// that, so it is named here rather than papered over.
//
// Honest limits, inherited from the same place LinearBus has them: the comment
// AUTHOR is whoever's token wrote it, not necessarily the participant — `ettle
// pull` publishes a non-adopter's atoms under their identity using the puller's
// token — so identity rides the marker and GitHub cannot corroborate it (leat's
// git-author check is strictly stronger). And a participant with no comment is
// invisible: Collect reports who is PRESENT, not an out-of-band roster.
//
// Politeness (we are a guest on their platform): every request carries a
// User-Agent identifying ettle, a 429 is surfaced as a distinct rate-limit error,
// and the write shape is light — one discussion, N comments, no per-emit litter.
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
	githubEndpoint    = "https://api.github.com/graphql"
	githubTitlePrefix = "ettle/"
	// githubMarkerFmt is the per-comment identity line. An HTML comment, so it is
	// invisible in GitHub's rendered view but survives a round-trip through the API.
	githubMarkerFmt = "<!-- ettle:%s -->"
)

// storedComment is one comment as the store hands it back: the participant slug
// parsed from its marker, the raw envelope JSON, and the node id used to update it.
type storedComment struct{ Participant, Content, ID string }

// commentStore is the minimal backend GitHubBus needs, factored out so unit tests
// use an in-memory fake and never touch the network.
type commentStore interface {
	// upsert sets the comment owned by `participant` to `content`, adding it if absent.
	upsert(ctx context.Context, participant, content string) error
	// list returns every ettle comment on the room's discussion.
	list(ctx context.Context) ([]storedComment, error)
	close() error
}

// GitHubBus is an atom transport over a repository Discussion: the room is the
// discussion, each participant is one comment, and its body is that person's
// latest envelope, so Collect returns the latest envelope per person.
type GitHubBus struct {
	store commentStore

	mu       sync.Mutex
	warnings []string
}

func newGitHubBusOn(store commentStore) *GitHubBus { return &GitHubBus{store: store} }

// NewGitHubBus builds a Discussion-backed transport. It refuses a public
// repository (see the package comment), then resolves or creates the room's
// discussion. token is a GitHub token with `repo` scope — the same one `gh auth`
// already holds on a developer machine.
func NewGitHubBus(token, owner, repo, room, version string) (*GitHubBus, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("transport/github: empty token (set GITHUB_TOKEN, or use `gh auth token`)")
	}
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(repo) == "" {
		return nil, fmt.Errorf("transport/github: need owner and repo (github://<owner>/<repo>[/<room>])")
	}
	store := &githubCommentStore{
		http:     &http.Client{Timeout: 30 * time.Second},
		token:    strings.TrimSpace(token),
		endpoint: githubEndpoint,
		ua:       "ettle/" + version + " (+https://github.com/justinstimatze/ettle)",
		owner:    strings.TrimSpace(owner),
		repo:     strings.TrimSpace(repo),
	}
	if err := store.resolveDiscussion(context.Background(), room); err != nil {
		return nil, err
	}
	return newGitHubBusOn(store), nil
}

// Publish upserts this participant's comment, replacing any prior body, so their
// latest atom set overwrites the earlier one.
func (b *GitHubBus) Publish(ctx context.Context, env Envelope) error {
	if strings.TrimSpace(env.Participant) == "" {
		return fmt.Errorf("transport/github: envelope has no participant")
	}
	if env.EmittedAt == "" {
		env.EmittedAt = time.Now().UTC().Format(time.RFC3339)
	}
	env.V = envelopeV
	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("transport/github: marshal %s: %w", env.Participant, err)
	}
	if err := b.store.upsert(ctx, slug(env.Participant), string(body)); err != nil {
		return fmt.Errorf("transport/github: upsert %s: %w", env.Participant, err)
	}
	return nil
}

// Collect returns the latest envelope per participant: it lists the discussion's
// comments, ignores any without an ettle marker (a human replying in the thread),
// and treats the MARKER slug as the authoritative identity — an in-content claim
// that disagrees is overridden and warned, a matching one kept so display casing
// survives. Same rule as LinearBus's title identity, for the same reason.
func (b *GitHubBus) Collect(ctx context.Context) ([]Envelope, error) {
	comments, err := b.store.list(ctx)
	if err != nil {
		return nil, fmt.Errorf("transport/github: list: %w", err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.warnings = nil
	out := make([]Envelope, 0, len(comments))
	for _, c := range comments {
		var env Envelope
		if err := json.Unmarshal([]byte(c.Content), &env); err != nil {
			b.warnings = append(b.warnings, fmt.Sprintf("skipped comment for %q: unparseable body", c.Participant))
			continue
		}
		if env.Participant == "" {
			env.Participant = c.Participant
		} else if slug(env.Participant) != c.Participant {
			b.warnings = append(b.warnings, fmt.Sprintf(
				"comment marked %q claims participant %q; using marker identity %q", c.Participant, env.Participant, c.Participant))
			env.Participant = c.Participant
		}
		out = append(out, env)
	}
	return out, nil
}

func (b *GitHubBus) Close() error { return b.store.close() }

// Warnings returns a copy of the non-fatal issues from the last Collect, matching
// DirBus/LeatBus/LinearBus so the driver can surface them the same way.
func (b *GitHubBus) Warnings() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.warnings) == 0 {
		return nil
	}
	return append([]string(nil), b.warnings...)
}

// renderCommentBody wraps an envelope in its identity marker and a fenced json
// block, so the Discussion stays readable to a human who opens it and the body
// still round-trips exactly.
func renderCommentBody(participant, content string) string {
	return fmt.Sprintf(githubMarkerFmt, participant) + "\n```json\n" + content + "\n```"
}

// parseCommentBody is renderCommentBody's inverse: it returns the participant slug
// and the envelope JSON, ok=false for any comment that is not an ettle comment (a
// teammate replying in the thread, which we leave strictly alone).
func parseCommentBody(body string) (participant, content string, ok bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(body), "<!-- ettle:")
	if !ok {
		return "", "", false
	}
	participant, rest, ok = strings.Cut(rest, "-->")
	if !ok {
		return "", "", false
	}
	participant = strings.TrimSpace(participant)
	if participant == "" {
		return "", "", false
	}
	rest = strings.TrimSpace(rest)
	rest = strings.TrimPrefix(rest, "```json")
	rest = strings.TrimSuffix(strings.TrimSpace(rest), "```")
	return participant, strings.TrimSpace(rest), true
}

// githubCommentStore is the GraphQL-over-HTTP backend.
type githubCommentStore struct {
	http         *http.Client
	token        string
	endpoint     string
	ua           string
	owner, repo  string
	discussionID string
}

// do executes one GraphQL request, decoding data into out (which may be nil). It
// maps a 429 to a distinct rate-limit error and surfaces GraphQL errors.
func (s *githubCommentStore) do(ctx context.Context, query string, vars map[string]any, out any) error {
	reqBody, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", s.ua)
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden {
		// GitHub signals both secondary rate limits and plain permission failures with
		// 403, so say which it looks like rather than guessing wrong in the message.
		return fmt.Errorf("github http %d (rate limit or insufficient token scope — `repo` is required): %s",
			resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
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
		return fmt.Errorf("github graphql: %s", envelope.Errors[0].Message)
	}
	if out != nil {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return fmt.Errorf("decode data: %w", err)
		}
	}
	return nil
}

// resolveDiscussion refuses a public repository, then finds the room's discussion
// and creates it if absent. The visibility check is FIRST and fatal: everything
// after it writes internal working state into the repo.
func (s *githubCommentStore) resolveDiscussion(ctx context.Context, room string) error {
	title := githubTitlePrefix + SanitizeID(room)
	var q struct {
		Repository *struct {
			ID                    string `json:"id"`
			IsPrivate             bool   `json:"isPrivate"`
			HasDiscussionsEnabled bool   `json:"hasDiscussionsEnabled"`
			DiscussionCategories  struct {
				Nodes []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
					Slug string `json:"slug"`
				} `json:"nodes"`
			} `json:"discussionCategories"`
			Discussions struct {
				Nodes []struct {
					ID    string `json:"id"`
					Title string `json:"title"`
				} `json:"nodes"`
			} `json:"discussions"`
		} `json:"repository"`
	}
	const query = `query($o:String!,$n:String!){ repository(owner:$o,name:$n){
	  id isPrivate hasDiscussionsEnabled
	  discussionCategories(first:25){ nodes{ id name slug } }
	  discussions(first:100){ nodes{ id title } } } }`
	if err := s.do(ctx, query, map[string]any{"o": s.owner, "n": s.repo}, &q); err != nil {
		return fmt.Errorf("transport/github: look up %s/%s: %w", s.owner, s.repo, err)
	}
	if q.Repository == nil {
		return fmt.Errorf("transport/github: repository %s/%s not found (or the token cannot see it)", s.owner, s.repo)
	}
	if !q.Repository.IsPrivate {
		return fmt.Errorf("transport/github: %s/%s is PUBLIC — refusing. A public repo's Discussions are readable by anyone on the internet, and the bus carries every participant's intents, commitments, and assumptions. Use a private repo (or the Linear bus, or `ettle room init` over a private git repo)", s.owner, s.repo)
	}
	if !q.Repository.HasDiscussionsEnabled {
		return fmt.Errorf("transport/github: Discussions are not enabled on %s/%s — turn them on in Settings → General → Features, or run: gh api -X PATCH repos/%s/%s -F has_discussions=true", s.owner, s.repo, s.owner, s.repo)
	}
	for _, d := range q.Repository.Discussions.Nodes {
		if d.Title == title {
			s.discussionID = d.ID
			return nil
		}
	}

	// Absent: create it. Prefer the "General" category, else whatever exists first —
	// an answerable category (Q&A) would be a strange home but is better than failing.
	cats := q.Repository.DiscussionCategories.Nodes
	if len(cats) == 0 {
		return fmt.Errorf("transport/github: %s/%s has Discussions enabled but no categories to create %q in", s.owner, s.repo, title)
	}
	catID := cats[0].ID
	for _, c := range cats {
		if c.Slug == "general" {
			catID = c.ID
			break
		}
	}
	var m struct {
		CreateDiscussion struct {
			Discussion struct {
				ID string `json:"id"`
			} `json:"discussion"`
		} `json:"createDiscussion"`
	}
	const create = `mutation($r:ID!,$c:ID!,$t:String!,$b:String!){
	  createDiscussion(input:{repositoryId:$r, categoryId:$c, title:$t, body:$b}){ discussion{ id } } }`
	vars := map[string]any{"r": q.Repository.ID, "c": catID, "t": title,
		"b": "ettle atom bus — one comment per participant, each holding that person's typed atoms. Managed by ettle (https://github.com/justinstimatze/ettle); editing a comment by hand will be overwritten on their next publish."}
	if err := s.do(ctx, create, vars, &m); err != nil {
		return fmt.Errorf("transport/github: create discussion %q: %w", title, err)
	}
	s.discussionID = m.CreateDiscussion.Discussion.ID
	return nil
}

// list returns the ettle comments on the room's discussion, skipping anything a
// human wrote in the thread.
func (s *githubCommentStore) list(ctx context.Context) ([]storedComment, error) {
	var q struct {
		Node struct {
			Comments struct {
				Nodes []struct {
					ID   string `json:"id"`
					Body string `json:"body"`
				} `json:"nodes"`
			} `json:"comments"`
		} `json:"node"`
	}
	const query = `query($d:ID!){ node(id:$d){ ... on Discussion {
	  comments(first:100){ nodes{ id body } } } } }`
	if err := s.do(ctx, query, map[string]any{"d": s.discussionID}, &q); err != nil {
		return nil, err
	}
	out := make([]storedComment, 0, len(q.Node.Comments.Nodes))
	for _, c := range q.Node.Comments.Nodes {
		who, content, ok := parseCommentBody(c.Body)
		if !ok {
			continue
		}
		out = append(out, storedComment{Participant: who, Content: content, ID: c.ID})
	}
	return out, nil
}

// upsert writes this participant's comment: update in place if they already have
// one, add it otherwise. Two calls (find + write), which the emit gate upstream
// keeps rare enough to stay well under any sane rate budget.
func (s *githubCommentStore) upsert(ctx context.Context, participant, content string) error {
	existing, err := s.list(ctx)
	if err != nil {
		return err
	}
	body := renderCommentBody(participant, content)
	for _, c := range existing {
		if c.Participant == participant {
			const update = `mutation($c:ID!,$b:String!){ updateDiscussionComment(input:{commentId:$c, body:$b}){ comment{ id } } }`
			return s.do(ctx, update, map[string]any{"c": c.ID, "b": body}, nil)
		}
	}
	const add = `mutation($d:ID!,$b:String!){ addDiscussionComment(input:{discussionId:$d, body:$b}){ comment{ id } } }`
	return s.do(ctx, add, map[string]any{"d": s.discussionID, "b": body}, nil)
}

func (s *githubCommentStore) close() error { return nil }

// ParseGitHubSpec splits "owner/repo" or "owner/repo/room" (the tail of a
// github:// transport string) into its parts. An omitted room is "default", so
// the common one-room-per-repo case needs no extra path segment.
func ParseGitHubSpec(spec string) (owner, repo, room string, err error) {
	parts := strings.Split(strings.Trim(strings.TrimSpace(spec), "/"), "/")
	switch len(parts) {
	case 2:
		return parts[0], parts[1], "default", nil
	case 3:
		if strings.TrimSpace(parts[2]) == "" {
			return parts[0], parts[1], "default", nil
		}
		return parts[0], parts[1], parts[2], nil
	default:
		return "", "", "", fmt.Errorf("transport/github: bad spec %q — want github://<owner>/<repo>[/<room>]", spec)
	}
}
