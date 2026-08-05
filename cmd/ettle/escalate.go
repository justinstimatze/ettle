package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/transport"
)

// `ettle escalate` is the EMIT half of the Linear agent path and the one path that
// writes ONTO Linear — the opt-in escalation to reach a teammate who won't install
// ettle. It reconciles the room's atoms, takes the FIRM cross-person knots (firm IS
// the calibration gate; a knot below the recurrence bar never posts), and surfaces
// each NEW one as a native agent elicitation on the room's single coordination issue
// — never a feature ticket. Idempotent: a knot already posted is skipped. Whisper-
// first means this is deliberate, not automatic; the default install never posts
// (see docs/SURFACES.md). Reply flow closes via `ettle pull`.

// knotPoster is the slice of LinearAgentWriter escalate needs, factored so the
// posting orchestration is tested with a fake and no network / app token.
type knotPoster interface {
	EnsureCoordinationIssue(ctx context.Context, room, teamID string) (issueID string, created bool, err error)
	OpenSession(ctx context.Context, issueID string) (sessionID string, err error)
	PostKnot(ctx context.Context, sessionID, body string) (activityID string, err error)
}

// escalateKey is the wording-independent identity of a coordination knot (kind +
// sorted distinct parties), so re-running escalate skips knots already posted even
// as their explanation wording drifts. Mirrors the MCP tangleKey semantics.
func escalateKey(k ettlemesh.Tangle) string {
	ps := make([]string, 0, len(k.Parties))
	seen := map[string]bool{}
	for _, p := range k.Parties {
		n := strings.ToLower(strings.TrimSpace(p))
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		ps = append(ps, n)
	}
	sort.Strings(ps)
	return k.Kind + "|" + strings.Join(ps, "+")
}

// escalatableKnots picks what to post: FIRM (the calibration gate) and cross-person
// (an escalation is inherently between people — a self-tangle is your own drift, not
// something to raise with a teammate) and NOT already emitted. Pure, so it's the
// unit-tested core of the decision.
func escalatableKnots(res horizonResult, already map[string]bool) []ettlemesh.Tangle {
	var out []ettlemesh.Tangle
	for _, k := range res.firm {
		if !ettlemesh.MultiPerson(k.Parties) {
			continue
		}
		if already[escalateKey(k)] {
			continue
		}
		out = append(out, k)
	}
	return out
}

// renderKnotBody is the elicitation text a teammate sees: what the knot is, who it's
// between, and how to respond. Framed as a question to confirm — humans stay the
// deciders; the mesh never asserts a cross-person conflict as fact.
func renderKnotBody(k ettlemesh.Tangle) string {
	parties := strings.Join(k.Parties, ", ")
	var b strings.Builder
	about := strings.TrimSpace(k.About)
	if about != "" {
		fmt.Fprintf(&b, "**Possible %s — %s** (%s)\n\n", k.Kind, about, parties)
	} else {
		fmt.Fprintf(&b, "**Possible %s** (%s)\n\n", k.Kind, parties)
	}
	if expl := strings.TrimSpace(k.Explanation); expl != "" {
		b.WriteString(expl)
		b.WriteString("\n\n")
	}
	b.WriteString("Reply here if this needs a decision — ettle brings your answer back into the team's coordination. If it's already handled, say so.")
	return strings.TrimSpace(b.String())
}

// postKnots runs the write orchestration against a knotPoster: ensure the one
// coordination issue, open a session, post each knot. Returns the keys actually
// posted (in order) so the caller records only those as emitted — a mid-run failure
// still persists the progress, so a retry doesn't repost.
func postKnots(ctx context.Context, w knotPoster, room, teamID string, knots []ettlemesh.Tangle) (issueID string, created bool, postedKeys []string, err error) {
	issueID, created, err = w.EnsureCoordinationIssue(ctx, room, teamID)
	if err != nil {
		return "", false, nil, err
	}
	sid, err := w.OpenSession(ctx, issueID)
	if err != nil {
		return issueID, created, nil, err
	}
	for _, k := range knots {
		if _, perr := w.PostKnot(ctx, sid, renderKnotBody(k)); perr != nil {
			return issueID, created, postedKeys, fmt.Errorf("post %q: %w", escalateKey(k), perr)
		}
		postedKeys = append(postedKeys, escalateKey(k))
	}
	return issueID, created, postedKeys, nil
}

// --- emitted-key store (idempotency, per room) ---------------------------

func emittedPath(room string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	return filepath.Join(dir, "ettle", "emit", transport.SanitizeID(room)+".json"), nil
}

func loadEmitted(room string) (map[string]bool, error) {
	path, err := emittedPath(room)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, err
	}
	var keys []string
	if err := json.Unmarshal(data, &keys); err != nil {
		return nil, fmt.Errorf("emitted store %s: corrupt: %w", path, err)
	}
	set := make(map[string]bool, len(keys))
	for _, k := range keys {
		set[k] = true
	}
	return set, nil
}

func saveEmitted(room string, set map[string]bool) error {
	path, err := emittedPath(room)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	data, _ := json.Marshal(keys)
	return os.WriteFile(path, data, 0o644)
}

// runEscalate is `ettle escalate --room <room>`.
func runEscalate(args []string) error {
	fs := flag.NewFlagSet("escalate", flag.ContinueOnError)
	room := fs.String("room", "", "the Linear room whose firm knots to escalate (maps to project ettle-<room>)")
	model := fs.String("model", "claude-haiku-4-5", "model id for the reconcile")
	samples := fs.Int("samples", 5, "reconcile samples to vote across; only firm knots (majority recurrence) are escalated")
	team := fs.String("team", strings.TrimSpace(os.Getenv("LINEAR_TEAM_ID")), "Linear team id, to create the coordination issue the first time (default LINEAR_TEAM_ID)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *room == "" {
		return fmt.Errorf("usage: ettle escalate --room <room>   (posts the room's firm cross-person knots to its one coordination issue, for teammates who don't run ettle)")
	}
	key := apiKey()
	if key == "" {
		return fmt.Errorf("no ANTHROPIC_API_KEY (set it in the environment or a .env file) — escalate reconciles locally")
	}
	lkey := strings.TrimSpace(os.Getenv("LINEAR_API_KEY"))
	if lkey == "" {
		return fmt.Errorf("escalate needs LINEAR_API_KEY (a member key) to read the room's atoms")
	}
	appTok := strings.TrimSpace(os.Getenv("LINEAR_AGENT_TOKEN"))
	if appTok == "" {
		return fmt.Errorf("escalate needs LINEAR_AGENT_TOKEN (the OAuth app-actor token) to post as the agent — the member key can read agent activities but not post them")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client := anthropic.NewClient(option.WithAPIKey(key), option.WithMaxRetries(4))
	det := ettlemesh.NewDetector(&client, *model)
	det.Ground = true // cross-person coupling check ON, so we escalate real conflicts
	bus, err := linearBusFor(*room)
	if err != nil {
		return err
	}
	defer bus.Close()

	res, err := reconcileHorizon(ctx, det, bus, "", *samples) // whole-team firm knots
	if err != nil {
		return err
	}
	already, err := loadEmitted(*room)
	if err != nil {
		return err
	}
	knots := escalatableKnots(res, already)
	if len(knots) == 0 {
		fmt.Println("ettle: no new firm cross-person knots to escalate.")
		return nil
	}

	writer := transport.NewLinearAgentWriter(appTok, buildVersion())
	_, created, postedKeys, postErr := postKnots(ctx, writer, *room, *team, knots)

	// Record what actually posted before returning any error, so a partial run
	// doesn't repost on retry.
	for _, k := range postedKeys {
		already[k] = true
	}
	if serr := saveEmitted(*room, already); serr != nil && postErr == nil {
		postErr = serr
	}
	if len(postedKeys) > 0 {
		note := ""
		if created {
			note = " (created the coordination issue)"
		}
		fmt.Printf("ettle: escalated %s to room %q's coordination issue%s.\n",
			plural(len(postedKeys), "knot", "knots"), *room, note)
	}
	return postErr
}
