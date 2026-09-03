package transport

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// LinearAgentWriter is the EMIT half of the Linear agent path: it surfaces a
// coordination tangle onto Linear as a native agent elicitation, for a teammate who
// never installs ettle. It authenticates as the OAuth APP ACTOR (Bearer) — the
// member key that reads agent activities (LinearAgentReader) cannot POST them, so
// this is genuinely a different token, not a style choice. This is the opt-in
// ESCALATION (docs/SURFACES.md): it writes only to one dedicated coordination issue
// per room ("ettle coordination" in project ettle-<room>), never onto a feature
// ticket. Emit gated to firm tangles upstream keeps the write rare.
type LinearAgentWriter struct {
	gql *linearDocStore
}

// coordinationIssueTitle is the single per-room issue every escalation lands on, so
// tangles are quarantined to one dedicated place and never pollute work tickets.
const coordinationIssueTitle = "ettle coordination"

// NewLinearAgentWriter builds the writer over the live GraphQL backend using an
// OAuth app-actor token (Bearer auth). expect carries the same workspace expectation
// the bus uses: EnsureCoordinationIssue calls the same resolveProject, so without it
// escalation would be the one path that could still create a room in the wrong
// workspace.
func NewLinearAgentWriter(appToken, version string, expect Workspace) *LinearAgentWriter {
	return &LinearAgentWriter{gql: &linearDocStore{
		http:      &http.Client{Timeout: 30 * time.Second},
		apiKey:    strings.TrimSpace(appToken),
		endpoint:  linearEndpoint,
		ua:        "ettle/" + version + " (+https://github.com/justinstimatze/ettle)",
		bearer:    true,
		expectOrg: expect,
	}}
}

// EnsureCoordinationIssue finds (or creates) the room's one coordination issue,
// reusing the same room→project mapping the bus uses (ettle-<room>). teamID is
// needed only to create the project/issue the first time; once it exists it's
// ignored.
func (w *LinearAgentWriter) EnsureCoordinationIssue(ctx context.Context, room, teamID string) (issueID string, created bool, err error) {
	pid, err := w.gql.resolveProject(ctx, room, teamID)
	if err != nil {
		return "", false, err
	}
	var found struct {
		Issues struct {
			Nodes []struct {
				ID string `json:"id"`
			} `json:"nodes"`
		} `json:"issues"`
	}
	const findQ = `query($p:ID!,$t:String!){ issues(filter:{project:{id:{eq:$p}}, title:{eq:$t}}){ nodes{ id } } }`
	if err := w.gql.do(ctx, findQ, map[string]any{"p": pid, "t": coordinationIssueTitle}, &found); err != nil {
		return "", false, fmt.Errorf("find coordination issue: %w", err)
	}
	if len(found.Issues.Nodes) > 0 {
		return found.Issues.Nodes[0].ID, false, nil
	}
	if strings.TrimSpace(teamID) == "" {
		return "", false, fmt.Errorf("no coordination issue yet and no team id to create one (set LINEAR_TEAM_ID)")
	}
	var made struct {
		IssueCreate struct {
			Success bool `json:"success"`
			Issue   struct {
				ID string `json:"id"`
			} `json:"issue"`
		} `json:"issueCreate"`
	}
	const createQ = `mutation($t:String!,$team:String!,$p:String!,$d:String!){ issueCreate(input:{title:$t, teamId:$team, projectId:$p, description:$d}){ success issue{ id } } }`
	desc := "Cross-person coordination tangles ettle surfaces here for teammates who don't run it. Reply in a thread and `ettle pull` brings your answer back into the mesh. ettle only posts to this one issue — never onto your feature tickets."
	if err := w.gql.do(ctx, createQ, map[string]any{"t": coordinationIssueTitle, "team": teamID, "p": pid, "d": desc}, &made); err != nil {
		return "", false, fmt.Errorf("create coordination issue: %w", err)
	}
	if !made.IssueCreate.Success || made.IssueCreate.Issue.ID == "" {
		return "", false, fmt.Errorf("create coordination issue: linear reported no success")
	}
	return made.IssueCreate.Issue.ID, true, nil
}

// OpenSession opens an agent session on the coordination issue; tangles posted to it
// render as native agent activities. The app's Agent-session webhook must be
// enabled for this to be allowed (Linear does not validate the URL is reachable —
// it is a registration flag, not a running server).
func (w *LinearAgentWriter) OpenSession(ctx context.Context, issueID string) (string, error) {
	var out struct {
		AgentSessionCreateOnIssue struct {
			Success      bool `json:"success"`
			AgentSession struct {
				ID string `json:"id"`
			} `json:"agentSession"`
		} `json:"agentSessionCreateOnIssue"`
	}
	const q = `mutation($i:String!){ agentSessionCreateOnIssue(input:{issueId:$i}){ success agentSession{ id } } }`
	if err := w.gql.do(ctx, q, map[string]any{"i": issueID}, &out); err != nil {
		return "", fmt.Errorf("open agent session: %w", err)
	}
	if out.AgentSessionCreateOnIssue.AgentSession.ID == "" {
		return "", fmt.Errorf("open agent session: linear returned no session (is the app's Agent-session webhook enabled?)")
	}
	return out.AgentSessionCreateOnIssue.AgentSession.ID, nil
}

// PostTangle posts one coordination tangle as an elicitation activity — a prompt the
// teammate can reply to inline.
func (w *LinearAgentWriter) PostTangle(ctx context.Context, sessionID, body string) (string, error) {
	var out struct {
		AgentActivityCreate struct {
			Success       bool `json:"success"`
			AgentActivity struct {
				ID string `json:"id"`
			} `json:"agentActivity"`
		} `json:"agentActivityCreate"`
	}
	const q = `mutation($s:String!,$c:JSONObject!){ agentActivityCreate(input:{agentSessionId:$s, content:$c}){ success agentActivity{ id } } }`
	content := map[string]any{"type": "elicitation", "body": body}
	if err := w.gql.do(ctx, q, map[string]any{"s": sessionID, "c": content}, &out); err != nil {
		return "", fmt.Errorf("post tangle: %w", err)
	}
	if out.AgentActivityCreate.AgentActivity.ID == "" {
		return "", fmt.Errorf("post tangle: linear returned no activity")
	}
	return out.AgentActivityCreate.AgentActivity.ID, nil
}
