// Package gemotclient is a thin MCP client for a gemot deliberation service
// (github.com/justinstimatze/gemot). It drives the deliberation primitive
// ettle's collective layer needs: agents submit positions, gemot extracts
// cruxes with a controversy score, and proposes a binding compromise. (gemot
// also exposes EigenTrust reputation; ettle doesn't consume it yet.)
//
// Bring up a local gemot with the one-command dev stack in deploy/ (NATS + gemot
// in demo mode — in-memory, no Postgres, no auth) and run ettle against it with
// --insecure-local. See deploy/README.md. A persistent, authenticated gemot
// (Postgres + GEMOT_REQUIRE_AUTH=1 + a per-agent bearer token) is the real
// deployment shape — see SECURITY.md and gemot's docs/private-deployment.md.
package gemotclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/justinstimatze/ettle/internal/loopback"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Client wraps an MCP session to a local gemot server.
type Client struct {
	ctx context.Context
	s   *mcp.ClientSession
}

// Connect dials gemot's MCP endpoint. A bearer token is REQUIRED off localhost,
// and the endpoint must be https:// when a token is sent (TLS terminated at gemot
// or a proxy) — the crux is the most sensitive payload on the wire and a bearer
// over plaintext leaks. The token is sent as `Authorization: Bearer` on every
// call. The only tokenless path is explicit: insecureLocal=true AND a loopback
// endpoint, for dev against gemot's anonymous sandbox.
//
// "loopback" is resolved, not string-matched (see internal/loopback): a hostname
// that resolves off-box is rejected even if it's named to look local. After
// connecting with a token, the session is checked to be non-anonymous (gemot
// degrades a bad/expired token to an anonymous sandbox rather than rejecting it
// unless GEMOT_REQUIRE_AUTH=1) — a detected anonymous session is a loud error,
// not a silent plaintext-grade run.
func Connect(ctx context.Context, endpoint, token string, insecureLocal bool) (*Client, error) {
	local := loopback.IsURL(endpoint)
	if token == "" {
		if !local {
			return nil, fmt.Errorf("gemotclient: refusing to connect to %s without a bearer token (set ETTLE_GEMOT_TOKEN) — anonymous gemot access is sandbox-only", endpoint)
		}
		if !insecureLocal {
			return nil, fmt.Errorf("gemotclient: %s is local but no token given; pass --insecure-local to use gemot's anonymous sandbox (dev only), or set ETTLE_GEMOT_TOKEN", endpoint)
		}
	} else if u, _ := neturl.Parse(endpoint); u != nil && u.Scheme != "https" && !local {
		// A bearer token over plaintext off-box is a credential leak. Refuse it
		// rather than send the key in the clear.
		return nil, fmt.Errorf("gemotclient: refusing to send a bearer token to %s over %q — use https:// (TLS at gemot or a proxy)", endpoint, u.Scheme)
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
	cl := &Client{ctx: ctx, s: s}
	// With a token we expect an authenticated session. If gemot still reports an
	// anonymous/sandbox session, the token didn't take (wrong/expired, or
	// GEMOT_REQUIRE_AUTH is off) — fail loud rather than route cruxes into a
	// shared anonymous sandbox believing we're authenticated.
	if token != "" && cl.sessionIsAnonymous() {
		s.Close()
		return nil, fmt.Errorf("gemotclient: connected to %s but the session is anonymous — the bearer token was not accepted (check the key and that gemot runs with GEMOT_REQUIRE_AUTH=1)", endpoint)
	}
	return cl, nil
}

// sessionIsAnonymous best-effort detects a degraded (unauthenticated) session
// from gemot's MCP initialize result — its server instructions/info name the
// anonymous sandbox when no valid credential was presented. Absent any such
// marker it returns false (don't block a session we can't prove is anonymous —
// the off-box token+https guards above are the hard gate).
func (c *Client) sessionIsAnonymous() bool {
	init := c.s.InitializeResult()
	if init == nil {
		return false
	}
	hay := strings.ToLower(init.Instructions)
	if init.ServerInfo != nil {
		hay += " " + strings.ToLower(init.ServerInfo.Name)
	}
	return strings.Contains(hay, "anonymous") || strings.Contains(hay, "sandbox mode") || strings.Contains(hay, "unauthenticated")
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
