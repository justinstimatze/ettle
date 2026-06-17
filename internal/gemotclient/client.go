// Package gemotclient is a thin MCP client for a local gemot deliberation
// service (github.com/justinstimatze/gemot). It drives the deliberation
// primitive ettle's collective layer needs: agents submit positions, gemot
// extracts cruxes with a controversy score, and proposes a binding compromise.
// (gemot also exposes EigenTrust reputation; ettle doesn't consume it yet.)
//
// Bring up a local gemot first (in-memory docker Postgres + an Anthropic key):
//
//	docker run -d --name gemot-pg -e POSTGRES_USER=gemot -e POSTGRES_PASSWORD=gemot \
//	  -e POSTGRES_DB=gemot -p 127.0.0.1:5432:5432 --tmpfs /var/lib/postgresql/data postgres:17-alpine
//	DATABASE_URL=postgres://gemot:gemot@localhost:5432/gemot?sslmode=disable \
//	  ANTHROPIC_API_KEY=$(sed -n 's/^ANTHROPIC_API_KEY=//p' .env) gemot http --addr :8080
package gemotclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Client wraps an MCP session to a local gemot server.
type Client struct {
	ctx context.Context
	s   *mcp.ClientSession
}

// Connect dials gemot's MCP endpoint. A bearer token is REQUIRED, and the
// endpoint should be https:// (TLS terminated at gemot or a proxy) — the crux is
// the most sensitive payload on the wire, and gemot degrades unauthed requests
// to an anonymous sandbox rather than rejecting them. The token is sent as
// `Authorization: Bearer` on every call. The only tokenless path is explicit:
// insecureLocal=true AND a localhost endpoint, for dev against the local
// sandbox. Note isLocal keys on the URL hostname, so do NOT point a "localhost"
// URL at a tunnel that forwards off-box — that would skip the token check.
func Connect(ctx context.Context, endpoint, token string, insecureLocal bool) (*Client, error) {
	if token == "" {
		if !isLocal(endpoint) {
			return nil, fmt.Errorf("gemotclient: refusing to connect to %s without a bearer token (set ETTLE_GEMOT_TOKEN) — anonymous gemot access is sandbox-only", endpoint)
		}
		if !insecureLocal {
			return nil, fmt.Errorf("gemotclient: %s is localhost but no token given; pass --insecure-local to use gemot's anonymous sandbox (dev only), or set ETTLE_GEMOT_TOKEN", endpoint)
		}
	}
	httpClient := http.DefaultClient
	if token != "" {
		httpClient = &http.Client{Transport: &bearerRoundTripper{token: token, base: http.DefaultTransport}}
	}
	c := mcp.NewClient(&mcp.Implementation{Name: "ettle", Version: "0.1"}, nil)
	s, err := c.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: httpClient}, nil)
	if err != nil {
		return nil, err
	}
	return &Client{ctx: ctx, s: s}, nil
}

// bearerRoundTripper attaches the gemot API key to every request.
type bearerRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (b *bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(req)
}

func isLocal(endpoint string) bool {
	u, err := neturl.Parse(endpoint)
	if err != nil {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

func (c *Client) Close() {
	if c.s != nil {
		c.s.Close()
	}
}

// call returns the concatenated text content, erroring on transport or tool error.
func (c *Client) call(name string, args map[string]any) (string, error) {
	res, err := c.s.CallTool(c.ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, ct := range res.Content {
		if t, ok := ct.(*mcp.TextContent); ok {
			b.WriteString(t.Text)
		}
	}
	if res.IsError {
		return b.String(), fmt.Errorf("gemot %s: %s", name, strings.TrimSpace(b.String()))
	}
	return b.String(), nil
}

// callSoft never errors on tool-level failure — returns (text, isError) so
// callers can poll past transient "not ready" states.
func (c *Client) callSoft(name string, args map[string]any) (string, bool) {
	res, err := c.s.CallTool(c.ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return err.Error(), true
	}
	var b strings.Builder
	for _, ct := range res.Content {
		if t, ok := ct.(*mcp.TextContent); ok {
			b.WriteString(t.Text)
		}
	}
	return b.String(), res.IsError
}

// Create makes a deliberation and returns its id.
func (c *Client) Create(topic, description string) (string, error) {
	out, err := c.call("deliberation", map[string]any{"action": "create", "topic": topic, "description": description})
	if err != nil {
		return "", err
	}
	var r struct {
		DeliberationID string `json:"deliberation_id"`
		ID             string `json:"id"`
	}
	firstJSON(out, &r)
	if r.DeliberationID != "" {
		return r.DeliberationID, nil
	}
	if r.ID != "" {
		return r.ID, nil
	}
	return "", fmt.Errorf("gemot: no deliberation_id in: %s", out)
}

// SubmitPosition adds one agent's position. Only the typed position crosses —
// never the agent's raw private reasoning.
func (c *Client) SubmitPosition(delibID, agentID, content, interests, reservation string) error {
	_, err := c.call("participate", map[string]any{
		"action": "submit_position", "deliberation_id": delibID,
		"agent_id": agentID, "content": content,
		"interests": interests, "reservation": reservation,
	})
	return err
}

// RunAnalysis triggers gemot's (async) crux extraction.
func (c *Client) RunAnalysis(delibID string) error {
	_, err := c.call("analyze", map[string]any{"action": "run", "deliberation_id": delibID})
	return err
}

// PollResult waits for the analysis result (raw JSON), tolerating "not ready" errors.
func (c *Client) PollResult(delibID string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	last := ""
	for {
		out, isErr := c.callSoft("analyze", map[string]any{"action": "get_result", "deliberation_id": delibID})
		last = out
		if !isErr && (strings.Contains(out, "crux") || strings.Contains(out, "cluster") || strings.Contains(out, "consensus")) {
			return out, nil
		}
		if time.Now().After(deadline) {
			return last, fmt.Errorf("analysis not ready after %s", timeout)
		}
		time.Sleep(5 * time.Second)
	}
}

// ProposeCompromise asks gemot for a binding compromise (raw JSON).
func (c *Client) ProposeCompromise(delibID string) string {
	out, _ := c.callSoft("analyze", map[string]any{"action": "propose_compromise", "deliberation_id": delibID})
	return out
}

// Crux is one extracted point of contention. Controversy in [0,1]. (gemot
// returns more fields — explanation, type, agree/disagree rosters — which ettle
// doesn't consume yet; parse them when a caller needs them.)
type Crux struct {
	Claim       string
	Controversy float64
}

// Cruxes parses the analysis result JSON.
func Cruxes(raw string) []Crux {
	var top map[string]any
	if firstJSON(raw, &top) != nil {
		return nil
	}
	var out []Crux
	for _, it := range findArray(top, "cruxes") {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, Crux{
			Claim:       firstStr(m, "crux_claim", "claim", "crux", "statement"),
			Controversy: firstFloat(m, "controversy_score", "controversy", "score"),
		})
	}
	return out
}

// Compromise is a proposed binding resolution.
type Compromise struct {
	Crux      string
	Proposal  string
	Rationale string
}

// Compromises parses propose_compromise output (array form or single-object form).
func Compromises(raw string) []Compromise {
	var top map[string]any
	if firstJSON(raw, &top) != nil {
		return nil
	}
	var out []Compromise
	for _, it := range findArray(top, "compromises") {
		if m, ok := it.(map[string]any); ok {
			out = append(out, Compromise{
				Crux:      firstStr(m, "crux", "claim"),
				Proposal:  firstStr(m, "proposal", "statement", "compromise"),
				Rationale: firstStr(m, "rationale", "why"),
			})
		}
	}
	if len(out) == 0 {
		if p := firstStr(top, "compromise_proposal", "proposal", "statement", "compromise"); p != "" {
			out = append(out, Compromise{
				Crux:      firstStr(top, "crux_claim", "crux", "topic"),
				Proposal:  p,
				Rationale: firstStr(top, "rationale", "why", "reasoning"),
			})
		}
	}
	return out
}

// --- helpers ---

func firstJSON(s string, v any) error {
	return json.NewDecoder(strings.NewReader(s)).Decode(v)
}

func findArray(top map[string]any, key string) []any {
	if a, ok := top[key].([]any); ok {
		return a
	}
	for _, nest := range []string{"result", "analysis", "latest", "data"} {
		if sub, ok := top[nest].(map[string]any); ok {
			if a, ok := sub[key].([]any); ok {
				return a
			}
		}
	}
	return nil
}

func firstStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func firstFloat(m map[string]any, keys ...string) float64 {
	for _, k := range keys {
		if f, ok := m[k].(float64); ok {
			return f
		}
	}
	return 0
}
