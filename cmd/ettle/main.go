// Command ettle is the slim multiplayer coordination PoC: point it at each
// teammate's working notes and it surfaces the coordination knots — collisions,
// duplicated work, stale assumptions, decision-rights gaps — before any of it
// ships. No meeting. The agent surfaces only what is relevant to its own human.
//
//	go run ./cmd/ettle standup --me alice testdata/standup/*.md
//
// The loop:
//  1. distill each person's notes into typed atoms (+ infer the operative
//     assumptions they didn't state);
//  2. publish atoms over the transport seam (in-process by default; NATS bus
//     with -tags nats) and collect the whole team's;
//  3. reconcile pairwise + team-wide into knots, routed FIRM (worth a look) vs
//     SOFT (worth a question);
//  4. route contested knots to a resolver (gemot, or an inline either/or);
//  5. surface to --me only what involves them.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"golang.org/x/sync/errgroup"

	"github.com/justinstimatze/ettle/internal/capture"
	"github.com/justinstimatze/ettle/internal/crux"
	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/eval"
	"github.com/justinstimatze/ettle/internal/mcpserver"
	"github.com/justinstimatze/ettle/internal/transport"
)

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "version", "-version", "--version", "-v":
			fmt.Println("ettle", buildVersion())
			return
		case "capture":
			if err := runCapture(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "ettle:", err)
				os.Exit(1)
			}
			return
		case "eval":
			if err := runEval(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "ettle:", err)
				os.Exit(1)
			}
			return
		case "drift":
			if err := runDrift(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "ettle:", err)
				os.Exit(1)
			}
			return
		case "mirror":
			if err := runMirror(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "ettle:", err)
				os.Exit(1)
			}
			return
		case "mcp":
			if err := runMCP(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "ettle:", err)
				os.Exit(1)
			}
			return
		case "room":
			if err := runRoom(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "ettle:", err)
				os.Exit(1)
			}
			return
		}
	}
	if len(os.Args) < 2 || os.Args[1] != "standup" {
		fmt.Fprintln(os.Stderr, "usage: ettle standup [flags] <input>...")
		fmt.Fprintln(os.Stderr, "  each input is one participant: a note file, or a Claude Code")
		fmt.Fprintln(os.Stderr, "  session transcript (.jsonl) — the live-reasoning L1 source.")
		fmt.Fprintln(os.Stderr, "  ettle capture <transcript.jsonl>   # preview what a session distills to")
		fmt.Fprintln(os.Stderr, "  ettle drift <prev-dir> <curr-dir>  # L2: directed models + surprise-gated deltas across two rounds")
		fmt.Fprintln(os.Stderr, "  ettle mcp                          # serve the coordination engine over MCP (stdio): ettle_emit / ettle_horizon / ettle_self_check")
		fmt.Fprintln(os.Stderr, "  cost: ~2N+3 model calls per sample for N participants; voting defaults to --samples 5 (set --samples 1 to disable)")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("standup", flag.ExitOnError)
	me := fs.String("me", "", "surface knots relevant to this participant (their agent's view); empty = full team view")
	model := fs.String("model", "claude-haiku-4-5", "model id")
	gemotURL := fs.String("gemot", "", "gemot MCP endpoint for contested knots (e.g. https://gemot.example/mcp); empty = inline either/or")
	transportName := fs.String("transport", "inproc", "atom transport: inproc | file://<shared-folder> (zero-infra, each agent writes its own file) | nats (needs -tags nats)")
	insecureLocal := fs.Bool("insecure-local", false, "dev only: allow plaintext/tokenless connections to localhost gemot + NATS (e.g. local docker)")
	gemotTimeout := fs.Duration("gemot-timeout", 180*time.Second, "how long to wait for a gemot deliberation's analysis")
	samples := fs.Int("samples", 5, "run the reconcile passes N times; recurrence frequency ranks knots firm (assert) vs soft (ask) — knots recurring at or above a per-kind bar are asserted, flickery ones become questions (not dropped). N=1 disables voting and falls back to confidence. Costs N× the reconcile calls.")
	showAtoms := fs.Bool("show-atoms", false, "print exactly what crosses the boundary (each person's typed atoms) before surfacing knots")
	noGround := fs.Bool("no-ground", false, "disable the cross-person coupling check (ON by default): it drops collision/duplication/teamwide knots that bridge people on a shared topic word across independent scopes (producer/consumer, different deliverables, an unscheduled task swept into a deadline). Measured to cut fabrication toward 0 at full real-knot recall (see ground.go).")
	groundModel := fs.String("ground-model", "", "verify cross-person knots with this (stronger) model instead of --model; empty = same as --model")
	shareInferred := fs.Bool("share-inferred", false, "let INFERRED atoms (your agent's de-novo guesses about a person) cross to the team. OFF by default: an inference is a claim the person never stated, and the pass measurably fabricates sensitive ones, so it is held back and surfaced to its subject first (docs/LEGIBILITY.md stage 0b)")
	room := fs.String("room", "", "use a configured leat room (created by `ettle room init|join`) as the transport — resolves that room's repo, agent, and remote; overrides --transport")
	_ = fs.Parse(os.Args[2:])

	cfg := runConfig{me: *me, model: *model, gemotURL: *gemotURL, transport: *transportName, room: *room, insecureLocal: *insecureLocal, gemotTimeout: *gemotTimeout, samples: *samples, showAtoms: *showAtoms, ground: !*noGround, groundModel: *groundModel, shareInferred: *shareInferred, paths: fs.Args()}
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "ettle:", err)
		os.Exit(1)
	}
}

type participant struct {
	Name, Role, Notes string
	Private           []string // phrases the note marked `private:` — never cross the boundary
}

type runConfig struct {
	me, model, gemotURL, transport, room string
	insecureLocal                        bool
	showAtoms                            bool
	ground                               bool
	groundModel                          string
	shareInferred                        bool
	gemotTimeout                         time.Duration
	samples                              int
	paths                                []string
}

func run(cfg runConfig) error {
	if len(cfg.paths) == 0 {
		return fmt.Errorf("no note files given")
	}
	key := apiKey()
	if key == "" {
		return fmt.Errorf("no ANTHROPIC_API_KEY (set it in the environment or a .env file)")
	}
	people, err := loadParticipants(cfg.paths)
	if err != nil {
		return err
	}
	// Validate --me against the roster: a typo'd name silently filters every
	// knot away and prints "the horizon is clear" — the most dangerous false
	// all-clear a coordination tool can give. Refuse it.
	if cfg.me != "" && !hasParticipant(people, cfg.me) {
		return fmt.Errorf("--me %q matches none of the loaded participants (%s)", cfg.me, strings.Join(names(people), ", "))
	}

	// Generous overall budget: a contested knot can spend minutes in gemot.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// WithMaxRetries: the SDK retries 429/5xx with backoff + Retry-After natively;
	// bump it above the default 2 so a transient rate-limit doesn't abort a whole
	// multi-person run (and re-bill every prior call).
	client := anthropic.NewClient(option.WithAPIKey(key), option.WithMaxRetries(4))
	det := ettlemesh.NewDetector(&client, cfg.model)
	det.Ground = cfg.ground
	det.GroundModel = cfg.groundModel

	bus, err := selectBus(cfg)
	if err != nil {
		return err
	}
	defer bus.Close()

	// 1: distill every person in parallel (independent calls — latency is the
	// "no meeting" competitor, so don't serialize N people).
	results, err := distillAll(ctx, det, people)
	if err != nil {
		return err
	}
	if cfg.showAtoms {
		printAtoms(results, cfg.shareInferred)
	}
	// 2: publish each person's atoms over the seam.
	var allQuestions []authoredQuestion
	var inferredAboutMe []ettlemesh.Atom
	heldInferred := 0 // de-novo claims held back from the team this run (legible abstention)
	for _, r := range results {
		for _, q := range r.questions {
			allQuestions = append(allQuestions, authoredQuestion{who: r.name, text: q})
		}
		// Subject-gating (docs/LEGIBILITY.md stage 0b): an inferred atom is a de-novo
		// claim ABOUT r.name that they did not state — and the inference pass measurably
		// fabricates sensitive ones (1a-1). So by default it does NOT cross the transport
		// to the team; it is surfaced to its own subject for review. --share-inferred
		// opts back into the old behavior (inferred atoms flow to the team reconcile).
		crossing := r.atoms
		if cfg.shareInferred {
			crossing = append(append([]ettlemesh.Atom{}, r.atoms...), r.inferred...)
		} else {
			heldInferred += len(r.inferred)
			if cfg.me != "" && ettlemesh.SamePerson(r.name, cfg.me) {
				inferredAboutMe = r.inferred
			}
		}
		if err := bus.Publish(ctx, transport.Envelope{Participant: r.name, Role: r.role, Atoms: crossing}); err != nil {
			return fmt.Errorf("publish %s: %w", r.name, err)
		}
	}

	// 3: collect the whole team and reconcile.
	envs, err := bus.Collect(ctx)
	if err != nil {
		return fmt.Errorf("collect: %w", err)
	}
	// Assert we got everyone we published. A short NATS gather window can drop a
	// slow peer; reconciling a subset and printing "clear" would be a false
	// all-clear. Make the gap loud, not silent.
	if got := distinctParticipants(envs); got < len(people) {
		fmt.Fprintf(os.Stderr, "ettle: WARNING collected %d of %d participants — results are PARTIAL (a peer may have missed the window); do not read this as 'all clear'.\n", got, len(people))
	}
	// File transport: a shared synced folder is eventually-consistent, so a peer's
	// file may not have arrived yet. Surface the roster + per-member staleness so a
	// stale/partial horizon is visible and "clear" is never read as bare. (Honest
	// limit: a teammate whose file never reached this folder can't be seen here.)
	reportCoverage(bus)
	atoms := transport.Atoms(envs)
	// Cross-person detection (pairwise + team-wide). With --samples>1, runs the
	// passes N times and keeps only knots that recur across a majority — the
	// stochastic detector's noise becomes a confidence signal.
	knots, floorHeld, err := det.ReconcileVoted(ctx, atoms, cfg.samples)
	if err != nil {
		return fmt.Errorf("reconcile: %w", err)
	}
	// Single-party self-assumption pass (the N=1 signal: a person whose own later
	// atoms drifted from an earlier assumption). Deduped against the cross-person
	// knots so a team-wide divergence isn't also reported as a private one.
	self, err := det.ReconcileSelf(ctx, atoms)
	if err != nil {
		return fmt.Errorf("reconcile self: %w", err)
	}
	knots = append(knots, ettlemesh.DedupeSelf(self, knots)...)
	// Cross-person coupling check: drop collision/duplication/teamwide knots that
	// bridge people on a shared topic word across independent scopes (ON by default;
	// disable with --no-ground). Suppressed = what it held back, surfaced quietly so a
	// human can overrule a wrong call (legible abstention; docs/LEGIBILITY.md).
	knots, suppressed, err := det.GroundKnots(ctx, knots, atoms)
	if err != nil {
		return fmt.Errorf("ground: %w", err)
	}

	// 4+5: resolve contested knots, then surface to --me.
	var resolver crux.Resolver = crux.Inline{}
	if cfg.gemotURL != "" {
		resolver = crux.Gemot{URL: cfg.gemotURL, Token: os.Getenv("ETTLE_GEMOT_TOKEN"), InsecureLocal: cfg.insecureLocal, Timeout: cfg.gemotTimeout}
	}
	surface(ctx, cfg.me, knots, suppressed, floorHeld, atoms, allQuestions, inferredAboutMe, heldInferred, resolver)
	return nil
}

// surface prints the knots relevant to `me` (or all, in team view), routed FIRM
// vs SOFT, with contested knots resolved. This is the agent → its own human.
func surface(ctx context.Context, me string, knots, suppressed []ettlemesh.Knot, floorHeld int, atoms []ettlemesh.Atom, questions []authoredQuestion, inferredAboutMe []ettlemesh.Atom, heldInferred int, resolver crux.Resolver) {
	// Act/ask routing (docs/LEGIBILITY.md stage 0c). The detector has no ground truth
	// for a cross-person conflict, and recurrence is test-retest STABILITY, not
	// validity — so a cross-person knot is never ASSERTED, only posed as a question to
	// the parties (mixed-initiative: ask when a wrong claim's cost is social and the
	// confidence is unearned). Only SELF knots — a person's own drift, which they can
	// verify directly — are asserted. The Firm-and-bindable "act" lane for cross-person
	// knots opens later, earned per kind against the calibration label (stage 2). The
	// ask lane is ordered firm-first so the most-recurring float to the top.
	var act, ask []ettlemesh.Knot
	for _, k := range knots {
		if me != "" && !partyOf(k, me) {
			continue
		}
		if ettlemesh.MultiPerson(k.Parties) {
			ask = append(ask, k)
		} else {
			act = append(act, k)
		}
	}
	sort.SliceStable(ask, func(i, j int) bool {
		if ask[i].Firm() != ask[j].Firm() {
			return ask[i].Firm() // firm-first
		}
		return ask[i].Votes > ask[j].Votes
	})

	who := "the team"
	if me != "" {
		who = me
	}
	fmt.Printf("\n  ettle — coordination horizon for %s\n", who)
	fmt.Printf("  %s across %s; %s surfaced\n",
		plural(len(atoms), "atom", "atoms"),
		plural(countPeople(atoms), "person", "people"),
		plural(len(act)+len(ask), "knot", "knots"))

	if len(act) == 0 && len(ask) == 0 {
		section("horizon")
		fmt.Println("    — nothing; the horizon is clear.")
	}
	if len(act) > 0 {
		section("worth a look (your own assumptions to revisit)")
		for _, k := range act {
			printKnot(k)
		}
	}
	if len(ask) > 0 {
		section("worth checking together (a question, not a claim — confirm or wave off)")
		for _, k := range ask {
			printAsk(k)
			if crux.Contested(k) {
				r, err := resolver.Resolve(ctx, k, atoms)
				switch {
				case err != nil:
					// Don't swallow it — a contested knot with no crux must explain why.
					fmt.Printf("      → (resolver unavailable: %v)\n", err)
				case r != nil:
					printResolution(r)
				}
			}
		}
	}

	// Questions the inference step couldn't answer confidently — surfaced only
	// to the person they're about (friction in the right spot).
	var mine []string
	for _, q := range questions {
		if me == "" || ettlemesh.SamePerson(q.who, me) {
			mine = append(mine, q.text)
		}
	}
	if len(mine) > 0 {
		section("open questions (low-confidence — ask, don't assume)")
		for _, q := range mine {
			fmt.Printf("    ? %s\n", q)
		}
	}

	// Legible abstention: what the coupling check held back, surfaced quietly OFF the
	// agenda so a clear horizon never hides a silently-dropped call the human would
	// have overruled (docs/LEGIBILITY.md, stage 0a). Filtered to me, like the rest.
	var heldBack []ettlemesh.Knot
	for _, k := range suppressed {
		if me == "" || partyOf(k, me) {
			heldBack = append(heldBack, k)
		}
	}
	if len(heldBack) > 0 {
		section("held back (the coupling check judged these not a real conflict — shown in case that's wrong)")
		for _, k := range heldBack {
			printKnot(k)
		}
	}
	// Subject-gated inference (docs/LEGIBILITY.md stage 0b): your agent's de-novo
	// guesses ABOUT you, held back from the team. The inference pass measurably
	// fabricates sensitive claims and asserts them (1a-1), so these do NOT cross until
	// you confirm — shown here for you to keep or kill. Only your own; never a teammate's.
	switch {
	case len(inferredAboutMe) > 0:
		// --me: show the subject their own inferred atoms, in full, to keep or kill.
		section("inferred about you — held back from the team (your agent's guess, not something you stated; confirm before it travels)")
		for _, a := range inferredAboutMe {
			fmt.Printf("    ? [%s] %s — %s  (inferred, confidence %.1f)\n", a.Typ, a.Subject, a.Content, a.Confidence)
		}
		fmt.Printf("      (these did NOT cross to the team; pass --share-inferred to let inferred atoms flow.)\n")
	case me == "" && heldInferred > 0:
		// Team view has no single subject, so the inferred atoms can't be shown to one
		// — but a clear horizon must not silently hide that they were held back (the
		// same no-silent-drop discipline as legible abstention, stage 0a). A count only,
		// never whose or what: that would leak the very de-novo claims we're gating.
		section("inference held back")
		fmt.Printf("    %s held back from the team (your agents' de-novo guesses about people, not stated facts).\n",
			plural(heldInferred, "inferred atom", "inferred atoms"))
		fmt.Printf("      (run with --me <name> to review one person's own, or --share-inferred to let them flow.)\n")
	}

	// The abstention floor's drops are noise by design (recurred in too few samples),
	// so they are NOT listed — but a single quiet count keeps a clear horizon honest:
	// the human knows candidates were suppressed, without the notice becoming noise.
	if floorHeld > 0 {
		fmt.Printf("\n    (+ %s below the confidence floor, not shown)\n",
			plural(floorHeld, "low-recurrence candidate", "low-recurrence candidates"))
	}
	fmt.Println()
}

// voteSuffix is the shared recurrence tail (" · seen in N/M samples") on a knot line —
// one definition so printKnot (assert) and printAsk (question) can't drift on it.
func voteSuffix(k ettlemesh.Knot) string {
	if k.Samples > 0 {
		return fmt.Sprintf(" · seen in %d/%d samples", k.Votes, k.Samples)
	}
	return ""
}

func printKnot(k ettlemesh.Knot) {
	fmt.Printf("    • [%s] %s\n      %s\n      parties: %s · confidence %.1f%s\n",
		k.Kind, k.About, k.Explanation, strings.Join(k.Parties, ", "), k.Confidence, voteSuffix(k))
}

// printAsk renders a cross-person knot as a QUESTION addressed to its parties, not an
// assertion (docs/LEGIBILITY.md stage 0c) — the detector cannot certify a cross-person
// conflict, so it poses it for the humans to confirm or wave off.
func printAsk(k ettlemesh.Knot) {
	fmt.Printf("    ? [possible %s] %s\n      %s\n      Real, or already handled?  parties: %s · confidence %.1f%s\n",
		k.Kind, k.About, k.Explanation, strings.Join(k.Parties, ", "), k.Confidence, voteSuffix(k))
}

func printResolution(r *Resolution) {
	fmt.Printf("      → crux (%s): %s\n", r.Source, r.Crux)
	if r.Controversy > 0 {
		fmt.Printf("        controversy %.2f\n", r.Controversy)
	}
	if r.Proposal != "" {
		fmt.Printf("        proposed: %s\n", r.Proposal)
	}
	for _, b := range r.Branches {
		fmt.Printf("        ↳ %s\n", b)
	}
}

// Resolution is aliased so the printer reads cleanly.
type Resolution = crux.Resolution

func section(title string) { fmt.Printf("\n  %s\n", title) }

// plural renders "1 person" / "3 people" — small thing, but "1 people" in the
// first line a stranger sees reads as unfinished.
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func partyOf(k ettlemesh.Knot, who string) bool {
	for _, p := range k.Parties {
		if ettlemesh.SamePerson(p, who) {
			return true
		}
	}
	return false
}

func countPeople(atoms []ettlemesh.Atom) int {
	seen := map[string]bool{}
	for _, a := range atoms {
		seen[strings.ToLower(strings.TrimSpace(a.From))] = true
	}
	return len(seen)
}

// hasParticipant reports whether `who` names one of the loaded participants.
func hasParticipant(people []participant, who string) bool {
	for _, p := range people {
		if ettlemesh.SamePerson(p.Name, who) {
			return true
		}
	}
	return false
}

// names lists participant names for error messages.
func names(people []participant) []string {
	out := make([]string, len(people))
	for i, p := range people {
		out[i] = p.Name
	}
	return out
}

// distinctParticipants counts the unique senders actually collected off the bus.
func distinctParticipants(envs []transport.Envelope) int {
	seen := map[string]bool{}
	for _, e := range envs {
		seen[strings.ToLower(strings.TrimSpace(e.Participant))] = true
	}
	return len(seen)
}

// reportCoverage prints the shared-folder roster + per-member staleness when the
// bus is the file transport, so a partially-synced or stale horizon is never read
// as a clean all-clear. A no-op for transports without coverage (inproc/NATS),
// whose freshness is covered by the partial-collection guard above.
func reportCoverage(bus transport.Transport) {
	cov, ok := bus.(interface {
		Coverage() []transport.MemberStatus
	})
	if !ok {
		return
	}
	for _, m := range cov.Coverage() {
		age := "fresh"
		if m.EmittedAt == "" {
			age = "age unknown"
		} else if m.Staleness > 0 {
			age = fmt.Sprintf("emitted %s ago", m.Staleness.Round(time.Second))
		}
		fmt.Fprintf(os.Stderr, "ettle: horizon member %q — %s\n", m.Participant, age)
	}
	if w, ok := bus.(interface{ Warnings() []string }); ok {
		for _, msg := range w.Warnings() {
			fmt.Fprintf(os.Stderr, "ettle: WARNING %s\n", msg)
		}
	}
}

// personResult is one participant's distilled atoms + the questions the inference
// step couldn't answer confidently. atoms are STATED (the person wrote them); inferred
// are de-novo claims the agent inferred ABOUT the person — kept separate so the
// standup can hold them back from the team until the subject sees them (the inference
// pass measurably fabricates sensitive claims — docs/LEGIBILITY.md stage 0b/1a-1).
type personResult struct {
	name, role string
	atoms      []ettlemesh.Atom // stated
	inferred   []ettlemesh.Atom // inferred about this person (subject-gated)
	questions  []string
}

// authoredQuestion is a low-confidence question tagged with the participant it
// concerns, so --me filtering routes on identity (SamePerson) rather than
// parsing a "[name]" substring out of the question text.
type authoredQuestion struct {
	who, text string
}

// distillAll distills every participant in parallel (Distill + InferImplicit are
// independent per person). Results are index-aligned to `people`, so there's no
// shared-append race. Concurrency is capped so a big team doesn't fan out into a
// rate-limit wall.
func distillAll(ctx context.Context, det *ettlemesh.Detector, people []participant) ([]personResult, error) {
	results := make([]personResult, len(people))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(4)
	for i, p := range people {
		i, p := i, p
		g.Go(func() error {
			a, err := det.Distill(gctx, p.Name, p.Role, p.Notes, p.Private)
			if err != nil {
				return fmt.Errorf("distill %s: %w", p.Name, err)
			}
			inf, qs, err := det.InferImplicit(gctx, p.Name, p.Role, p.Notes, p.Private)
			if err != nil {
				return fmt.Errorf("infer %s: %w", p.Name, err)
			}
			results[i] = personResult{name: p.Name, role: p.Role, atoms: a, inferred: inf, questions: qs}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return results, nil
}

// printAtoms shows exactly what crosses the boundary — the privacy surface. This
// is the honest answer to "what leaves my machine": only these typed atoms, not
// the raw note or session.
func printAtoms(results []personResult, shareInferred bool) {
	fmt.Printf("\n  atoms crossing the boundary (this is what leaves each machine):\n")
	// The inferred tag must tell the truth about whether the atom actually crosses:
	// held back by default (stage 0b), but --share-inferred lets it flow to the team.
	infTag := "inferred %.1f — held back from the team (does NOT cross)"
	if shareInferred {
		infTag = "inferred %.1f — crosses to the team (--share-inferred)"
	}
	for _, r := range results {
		fmt.Printf("    %s:\n", r.name)
		if len(r.atoms) == 0 && len(r.inferred) == 0 {
			fmt.Println("      (none)")
		}
		for _, a := range r.atoms {
			fmt.Printf("      • [%s] %s — %s  (stated)\n", a.Typ, a.Subject, a.Content)
		}
		for _, a := range r.inferred {
			fmt.Printf("      • [%s] %s — %s  ("+infTag+")\n", a.Typ, a.Subject, a.Content, a.Confidence)
		}
	}
}

// detectFor runs the detector over participants and returns FIRM + soft knots.
// samples>1 routes the cross-person passes through majority voting. Self-knots
// are deduped against the cross-person set. (No transport — eval reconciles
// directly, the bus is only for the distributed standup.)
func detectFor(ctx context.Context, det *ettlemesh.Detector, people []participant, samples int) (firm, soft []ettlemesh.Knot, err error) {
	results, err := distillAll(ctx, det, people)
	if err != nil {
		return nil, nil, err
	}
	// The eval measures DETECTION, not the privacy surface — so it reconciles over
	// stated + inferred together (the standup's subject-gating of inferred atoms is a
	// user-surface policy, not a detector change).
	var atoms []ettlemesh.Atom
	for _, r := range results {
		atoms = append(atoms, r.atoms...)
		atoms = append(atoms, r.inferred...)
	}
	knots, _, err := det.ReconcileVoted(ctx, atoms, samples)
	if err != nil {
		return nil, nil, err
	}
	self, err := det.ReconcileSelf(ctx, atoms)
	if err != nil {
		return nil, nil, err
	}
	knots = append(knots, ettlemesh.DedupeSelf(self, knots)...)
	// Cross-person coupling check: drop collision/duplication/teamwide knots that
	// bridge people on a shared topic word across independent scopes (ON by default;
	// disable with --no-ground). The eval scores kept knots; suppressed is for the
	// human-facing surface, not the precision/recall harness.
	knots, _, err = det.GroundKnots(ctx, knots, atoms)
	if err != nil {
		return nil, nil, err
	}
	for _, k := range knots {
		if k.Firm() {
			firm = append(firm, k)
		} else {
			soft = append(soft, k)
		}
	}
	return firm, soft, nil
}

// runEval scores the detector against committed synthetic corpora: precision /
// recall over FIRM knots vs the curated ground truth, and (with --ab) the honest
// single-shot-vs-voted comparison with a McNemar significance test.
func runEval(args []string) error {
	fs := flag.NewFlagSet("eval", flag.ExitOnError)
	model := fs.String("model", "claude-haiku-4-5", "model id")
	samples := fs.Int("samples", 3, "voting samples for the --ab voted condition")
	ab := fs.Bool("ab", false, "also run the voted condition and compare with McNemar")
	leak := fs.Bool("leak", false, "privacy-boundary mode: distill a leak corpus and measure the secret leak rate (not knot detection)")
	leakInference := fs.Bool("leak-inference", false, "inference-channel mode: run the INFERENCE pass on a trap corpus and measure whether it manufactures a sensitive de-novo claim the source never stated (opt-in: adds one inference call per case — see docs/LEGIBILITY.md stage 1a)")
	stability := fs.Bool("stability", false, "determinism mode: run each corpus K times and report run-to-run knot-set agreement (Jaccard)")
	runs := fs.Int("runs", 5, "number of repeated runs for --stability")
	superposition := fs.Bool("superposition", false, "locality mode: check f(A∪B)=f(A)∪f(B) for independent groups — flags fabricated cross-group knots")
	separability := fs.Bool("separability", false, "diagnostic: over K joint runs, contrast fabricated vs real knots on recurrence-frequency and confidence (picks the fork: voting/threshold vs upstream grounding)")
	noGround := fs.Bool("no-ground", false, "disable the cross-person coupling check (ON by default — see ground.go); pass to measure the pre-grounding baseline")
	groundModel := fs.String("ground-model", "", "verify cross-person knots with this (stronger) model instead of --model; empty = same as --model")
	_ = fs.Parse(args)
	if len(fs.Args()) == 0 {
		return fmt.Errorf("usage: ettle eval [--ab] [--samples K] <corpus.json>...\n       ettle eval --leak <leak-corpus.json>...\n       ettle eval --stability [--runs K] <corpus.json>...\n       ettle eval --superposition <super-corpus.json>...")
	}
	key := apiKey()
	if key == "" {
		return fmt.Errorf("no ANTHROPIC_API_KEY (set it in the environment or a .env file)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	client := anthropic.NewClient(option.WithAPIKey(key), option.WithMaxRetries(4))
	det := ettlemesh.NewDetector(&client, *model)
	det.Ground = !*noGround
	det.GroundModel = *groundModel

	if *leak {
		return runLeakEval(ctx, det, fs.Args())
	}
	if *leakInference {
		return runLeakInferenceEval(ctx, det, fs.Args())
	}
	if *stability {
		return runStabilityEval(ctx, det, fs.Args(), *runs)
	}
	if *superposition {
		return runSuperpositionEval(ctx, det, fs.Args(), *runs, *samples)
	}
	if *separability {
		return runSeparabilityEval(ctx, det, fs.Args(), *runs)
	}

	// A/B discordant pairs are pooled ACROSS corpora: per-corpus, b+c is bounded
	// by the handful of real labels in one scenario and can never reach the N
	// where McNemar is reliable, so a per-corpus test would be structurally
	// incapable of ever finding significance. Pooling the discordant pairs is the
	// only honest way to get enough N to test at all.
	var poolB, poolC, poolFPSingle, poolFPVoted int
	for _, path := range fs.Args() {
		c, err := eval.LoadCorpus(path)
		if err != nil {
			return fmt.Errorf("load corpus %s: %w", path, err)
		}
		people, err := loadParticipants(c.Inputs)
		if err != nil {
			return fmt.Errorf("corpus %s inputs: %w", c.Name, err)
		}
		fmt.Printf("\n  ══ %s ══ (%d inputs, %d curated knots)\n", c.Name, len(c.Inputs), len(c.Expected))

		firm, soft, err := detectFor(ctx, det, people, 1)
		if err != nil {
			// One corpus's model hiccup (e.g. a weak model garbling the tool schema)
			// must not abort corpora that would pass — an eval harness that dies on
			// the first bad call can't report the rest. Note it loud, score the others.
			fmt.Printf("    ⚠ CORPUS FAILED (skipped): %v\n", err)
			continue
		}
		s := eval.Adjudicate(firm, soft, c.Expected)
		printScore("single-shot", s)
		// Specificity (simulation #1): when a corpus has NO real knots (RecallTotal
		// counts the real labels), the correct horizon is empty. Recall/precision
		// are degenerate here, so the meaningful headline is how much got surfaced
		// anyway — target 0. A firm knot is a false alarm; a soft one is at least
		// hedged as a question. Any trap hit already prints on the single-shot line.
		if s.RecallTotal == 0 {
			fmt.Printf("    %-22s %d firm + %d soft surfaced on independent work (target 0)\n",
				"specificity", s.TP+s.FP, s.WouldAsk)
		}
		printCalibration(append(append([]ettlemesh.Knot{}, firm...), soft...), c.Expected)

		if *ab {
			fv, sv, err := detectFor(ctx, det, people, *samples)
			if err != nil {
				fmt.Printf("    ⚠ voted condition failed (A/B skipped for this corpus): %v\n", err)
				continue
			}
			sV := eval.Adjudicate(fv, sv, c.Expected)
			printScore(fmt.Sprintf("voted (%d samples)", *samples), sV)
			// Accumulate this corpus's discordant recall pairs into the pool; the
			// test itself runs once, after all corpora, on the pooled N.
			var b, cc int
			for _, l := range c.Expected {
				if !l.Real {
					continue
				}
				if s.Recovered[l.ID] && !sV.Recovered[l.ID] {
					b++
				}
				if !s.Recovered[l.ID] && sV.Recovered[l.ID] {
					cc++
				}
			}
			poolB, poolC = poolB+b, poolC+cc
			poolFPSingle, poolFPVoted = poolFPSingle+s.FP, poolFPVoted+sV.FP
			fmt.Printf("    A/B (recall): discordant single-only=%d voted-only=%d (pooled below)\n", b, cc)
		}
	}

	if *ab {
		p := eval.McNemarTwoTailed(poolB, poolC)
		fmt.Printf("\n  ══ A/B pooled across %d corpora ══\n", len(fs.Args()))
		fmt.Printf("    recall discordance: single-only=%d voted-only=%d → McNemar p=%.3f %s\n",
			poolB, poolC, p, abVerdict(p, poolB, poolC))
		fmt.Printf("    precision: single FP=%d, voted FP=%d (descriptive — not a paired test)\n", poolFPSingle, poolFPVoted)
	}
	return nil
}

// runStabilityEval is the determinism harness (simulation #5). It runs each
// corpus K times and measures how much the SET of surfaced knots agrees run to
// run — the identity being (kind, parties), not wording, so a reworded
// explanation does not count as a different knot. A tool people check every
// morning must surface the same horizon from the same input; flicker is a trust
// failure independent of any single run's correctness. Cost: K detection cycles
// per corpus (one Distill per person per run); the scoring is pure and
// unit-tested. Default model is haiku to keep the repeated runs cheap.
func runStabilityEval(ctx context.Context, det *ettlemesh.Detector, paths []string, runs int) error {
	if runs < 2 {
		return fmt.Errorf("--stability needs --runs >= 2 (got %d): with one run there are no pairs to compare", runs)
	}
	for _, path := range paths {
		c, err := eval.LoadCorpus(path)
		if err != nil {
			return fmt.Errorf("load corpus %s: %w", path, err)
		}
		people, err := loadParticipants(c.Inputs)
		if err != nil {
			return fmt.Errorf("corpus %s inputs: %w", c.Name, err)
		}
		fmt.Printf("\n  ══ %s ══ (%d inputs, %d runs)\n", c.Name, len(c.Inputs), runs)

		var runKeys []map[string]bool
		failed := false
		for i := 0; i < runs; i++ {
			firm, soft, err := detectFor(ctx, det, people, 1)
			if err != nil {
				fmt.Printf("    ⚠ run %d failed (corpus skipped): %v\n", i+1, err)
				failed = true
				break
			}
			keys := eval.RunKeys(firm, soft)
			runKeys = append(runKeys, keys)
			fmt.Printf("    run %d: %d distinct knots\n", i+1, len(keys))
		}
		if failed {
			continue
		}

		res := eval.ComputeStability(runKeys)
		fmt.Printf("    %-22s mean pairwise Jaccard %.2f · worst pair %.2f (1.00 = identical horizon every run)\n",
			"stability", res.MeanJaccard, res.MinJaccard)
		if flick := res.Flickering(); len(flick) > 0 {
			fmt.Printf("    %-22s %d knot(s) appeared in some runs but not all:\n", "FLICKER", len(flick))
			for _, k := range flick {
				fmt.Printf("      %s  (in %d/%d runs)\n", prettyKey(k), res.Frequency[k], runs)
			}
		} else {
			fmt.Printf("    %-22s every surfaced knot appeared in all %d runs\n", "stable", runs)
		}
	}
	return nil
}

// prettyKey turns a stability key ("kind\x00a+b") back into readable form.
func prettyKey(key string) string {
	parts := strings.SplitN(key, "\x00", 2)
	if len(parts) != 2 {
		return key
	}
	return fmt.Sprintf("[%s] %s", parts[0], strings.ReplaceAll(parts[1], "+", ", "))
}

// runSuperpositionEval is the locality harness (the metamorphic test). For two
// independent groups it checks the law f(A∪B)=f(A)∪f(B). The headline is the
// CROSS-BOUNDARY FABRICATION: a knot linking A and B can never appear in a solo
// run, so it is provably invented — but WHETHER it appears varies run to run, so
// a single joint run is misleading (one clean run looks like a pass). We run the
// join K times. The PRIMARY signal is the mean cross-boundary knots PER RUN with
// a 95% CI (lower variance than a binary rate, so the same K buys a tighter
// estimate); a coarser binary rate (% of runs with >=1) with a Wilson interval is
// reported alongside. Both carry bands so an A/B is honest about whether two
// conditions actually differ — overlapping bands = underpowered. Every distinct
// fabricated link is listed. A and B are each detected once as the intra-group
// baseline (those secondary metrics are flicker-confounded; the cross-boundary
// signal is not). Cost: (K+2) cycles.
func runSuperpositionEval(ctx context.Context, det *ettlemesh.Detector, paths []string, runs, samples int) error {
	if runs < 1 {
		runs = 1
	}
	if samples < 1 {
		samples = 1
	}
	for _, path := range paths {
		c, err := eval.LoadSuperCorpus(path)
		if err != nil {
			return fmt.Errorf("load super-corpus %s: %w", path, err)
		}
		peopleA, err := loadParticipants(c.GroupA)
		if err != nil {
			return fmt.Errorf("corpus %s groupA: %w", c.Name, err)
		}
		peopleB, err := loadParticipants(c.GroupB)
		if err != nil {
			return fmt.Errorf("corpus %s groupB: %w", c.Name, err)
		}
		groupA, groupB := nameSet(peopleA), nameSet(peopleB)
		joint := append(append([]participant{}, peopleA...), peopleB...)
		fmt.Printf("\n  ══ %s ══ (A: %s · B: %s · %d joint runs × %d samples)\n", c.Name,
			strings.Join(sortedKeys(groupA), ","), strings.Join(sortedKeys(groupB), ","), runs, samples)

		// Baselines run VOTED at the same samples as the product ships, so f(A)/f(B)
		// reflect the abstention gate + firm bar, not raw single-shot output. Only the
		// pooled (firm+soft) key set is needed for the locality law.
		keysA, _, err := detectKeysVoted(ctx, det, peopleA, samples)
		if err != nil {
			fmt.Printf("    ⚠ group A detection failed (skipped): %v\n", err)
			continue
		}
		keysB, _, err := detectKeysVoted(ctx, det, peopleB, samples)
		if err != nil {
			fmt.Printf("    ⚠ group B detection failed (skipped): %v\n", err)
			continue
		}
		fmt.Printf("    f(A)=%d knots · f(B)=%d knots (intra-group baseline, voted)\n", len(keysA), len(keysB))

		fabFirm := map[string]bool{} // distinct ASSERTED cross-boundary links (the harm)
		fabAll := map[string]bool{}  // distinct cross-boundary links incl. soft (asked)
		var perRunFirm, perRunAll []int
		var localitySum float64
		var dropped, spurious, orphan int
		ok := 0
		for i := 0; i < runs; i++ {
			allKeysAB, firmKeysAB, err := detectKeysVoted(ctx, det, joint, samples)
			if err != nil {
				fmt.Printf("    ⚠ joint run %d failed: %v\n", i+1, err)
				continue
			}
			ok++
			// Locality is computed over the pooled set (a soft cross-group knot still
			// violates locality); the FIRM split below is what separates an asserted
			// fabrication (stop-ship) from a merely asked one.
			r := eval.ComputeSuperposition(keysA, keysB, allKeysAB, groupA, groupB)
			localitySum += r.LocalityScore()
			dropped += len(r.Dropped)
			spurious += len(r.SpuriousIntra)
			orphan += len(r.Orphan)
			firmCount := 0
			for _, k := range r.CrossBoundary {
				fabAll[k] = true
				if firmKeysAB[k] {
					firmCount++
					fabFirm[k] = true
				}
			}
			perRunAll = append(perRunAll, len(r.CrossBoundary))
			perRunFirm = append(perRunFirm, firmCount)
		}
		if ok == 0 {
			continue
		}

		// FIRM is the primary, stop-ship signal: an ASSERTED cross-group knot links
		// two people who were never in the same run, presented as a claim. Target 0.
		stFirm := eval.ComputeSuperStats(perRunFirm)
		stAll := eval.ComputeSuperStats(perRunAll)
		fmt.Printf("    %-22s %.2f knots/run  [95%% CI %.2f–%.2f]  — ASSERTED FABRICATION (stop-ship; target 0)\n",
			"FIRM CROSS-BOUNDARY", stFirm.MeanPerRun, stFirm.MeanCILow, stFirm.MeanCIHigh)
		fmt.Printf("    %-22s %.0f%% of %d runs asserted ≥1 A↔B knot  [95%% CI %.0f–%.0f%%]\n",
			"  (firm rate)", stFirm.Rate*100, ok, stFirm.RateCILow*100, stFirm.RateCIHigh*100)
		for _, k := range sortedKeys(fabFirm) {
			fmt.Printf("      FIRM  %s\n", prettyKey(k))
		}
		// ALL (firm+soft) is the secondary signal: the total locality leakage,
		// including knots only ASKED as questions (lower harm under "lighter agenda").
		fmt.Printf("    %-22s %.2f knots/run  [95%% CI %.2f–%.2f]  (incl. soft / asked-not-asserted)\n",
			"all cross-boundary", stAll.MeanPerRun, stAll.MeanCILow, stAll.MeanCIHigh)
		for _, k := range sortedKeys(fabAll) {
			if fabFirm[k] {
				continue // already listed as FIRM
			}
			fmt.Printf("      soft  %s\n", prettyKey(k))
		}
		fmt.Printf("    %-22s %.2f mean (1.00 = law holds; dominated by intra-group flicker, see #5)\n",
			"locality", localitySum/float64(ok))
		fmt.Printf("    %-22s %d dropped, %d spurious-intra across runs (flicker-confounded)\n",
			"intra-group churn", dropped, spurious)
		if orphan > 0 {
			fmt.Printf("    %-22s %d knot(s) about a non-participant — roster/identity bug\n", "ORPHAN", orphan)
		}
	}
	return nil
}

// detectKeysVoted runs one VOTED detection cycle (samples reconcile passes, through
// the abstention gate + firm bar) and returns two key sets: allKeys (firm+soft
// pooled — what the run "said") and firmKeys (asserted only). The superposition
// headline splits on firmKeys so an ASSERTED cross-group fabrication (the real harm)
// is reported apart from one merely asked as a question.
func detectKeysVoted(ctx context.Context, det *ettlemesh.Detector, people []participant, samples int) (allKeys, firmKeys map[string]bool, err error) {
	firm, soft, err := detectFor(ctx, det, people, samples)
	if err != nil {
		return nil, nil, err
	}
	return eval.RunKeys(firm, soft), eval.RunKeys(firm, nil), nil
}

// detectKnots runs one detection cycle and returns every knot it surfaced
// (firm+soft pooled), preserving confidence — the separability diagnostic needs
// the per-knot confidence the stability-key set throws away.
func detectKnots(ctx context.Context, det *ettlemesh.Detector, people []participant) ([]ettlemesh.Knot, error) {
	firm, soft, err := detectFor(ctx, det, people, 1)
	if err != nil {
		return nil, err
	}
	return append(append([]ettlemesh.Knot{}, firm...), soft...), nil
}

// runSeparabilityEval is the fork-picking diagnostic. Over K single-shot joint
// runs of a superposition corpus it contrasts FABRICATED (cross-group) knots
// against REAL (intra-group) ones on two signals — recurrence frequency (which
// simulates majority voting offline, free) and model confidence. If the fabricated
// knots are low-frequency and/or low-confidence relative to the real ones, the
// cheap fixes apply (voting we already have, or an abstention threshold); if the
// distributions overlap, grounding must move upstream to distill time. One batch,
// no extra calls beyond the K joint runs. Cost: K cycles.
func runSeparabilityEval(ctx context.Context, det *ettlemesh.Detector, paths []string, runs int) error {
	if runs < 2 {
		return fmt.Errorf("--separability needs --runs >= 2 (got %d): frequency needs repeated runs", runs)
	}
	for _, path := range paths {
		c, err := eval.LoadSuperCorpus(path)
		if err != nil {
			return fmt.Errorf("load super-corpus %s: %w", path, err)
		}
		peopleA, err := loadParticipants(c.GroupA)
		if err != nil {
			return fmt.Errorf("corpus %s groupA: %w", c.Name, err)
		}
		peopleB, err := loadParticipants(c.GroupB)
		if err != nil {
			return fmt.Errorf("corpus %s groupB: %w", c.Name, err)
		}
		groupA, groupB := nameSet(peopleA), nameSet(peopleB)
		joint := append(append([]participant{}, peopleA...), peopleB...)
		fmt.Printf("\n  ══ %s ══ (A: %s · B: %s · %d joint runs)\n", c.Name,
			strings.Join(sortedKeys(groupA), ","), strings.Join(sortedKeys(groupB), ","), runs)

		var jointRuns [][]ettlemesh.Knot
		for i := 0; i < runs; i++ {
			ks, err := detectKnots(ctx, det, joint)
			if err != nil {
				fmt.Printf("    ⚠ joint run %d failed: %v\n", i+1, err)
				continue
			}
			jointRuns = append(jointRuns, ks)
		}
		if len(jointRuns) < 2 {
			fmt.Printf("    too few successful runs (%d) to diagnose\n", len(jointRuns))
			continue
		}

		rep := eval.ComputeSeparability(jointRuns, groupA, groupB)
		k := rep.Runs
		majority := k/2 + 1

		fmt.Printf("    FABRICATED (cross-group) — %d distinct:\n", len(rep.Fabricated))
		for _, s := range rep.Fabricated {
			fmt.Printf("      %-44s seen %2d/%d · mean-conf %.2f%s\n",
				prettyKey(s.Key), s.Seen, k, s.MeanConf, votedNote(s.Seen, majority))
		}
		fmt.Printf("    REAL (intra-group) — %d distinct:\n", len(rep.Real))
		for _, s := range rep.Real {
			fmt.Printf("      %-44s seen %2d/%d · mean-conf %.2f%s\n",
				prettyKey(s.Key), s.Seen, k, s.MeanConf, votedNote(s.Seen, majority))
		}
		if len(rep.Orphan) > 0 {
			fmt.Printf("    ORPHAN (party in neither group — roster bug) — %d distinct\n", len(rep.Orphan))
		}

		fmt.Printf("    %-26s real %.0f/%d  vs  fabricated %.0f/%d  (would a samples>=%d majority vote split them?)\n",
			"frequency median", rep.RealFreqMedian, k, rep.FabFreqMedian, k, majority)
		fmt.Printf("    %-26s real %.2f  vs  fabricated %.2f  (does a confidence threshold split them?)\n",
			"confidence mean", rep.RealConfMean, rep.FabConfMean)

		// Project, offline from these same K runs, what voting would fabricate at
		// each samples size — the tuning curve, free of new calls. Seed fixed for a
		// reproducible estimate.
		curve := eval.ProjectVotingCurve(jointRuns, groupA, groupB, []int{1, 3, 5, 7, 9}, 4000, 1)
		fmt.Printf("    projected cross-group fabrication per voted detection (offline from %d runs):\n", k)
		for _, p := range curve {
			fmt.Printf("      samples=%d (maj %d)   %.3f knots/detection  [95%% CI %.2f–%.2f]\n",
				p.Samples, p.Majority, p.FabMean, p.FabCILow, p.FabCIHigh)
		}
	}
	return nil
}

// votedNote flags whether a knot's recurrence clears a strict-majority vote — the
// offline simulation of what ReconcileVoted(samples=K) would keep.
func votedNote(seen, majority int) string {
	if seen >= majority {
		return "  ✓ survives vote"
	}
	return "  ✗ voted out"
}

// nameSet is the lowercased participant-name set for a group (matches how KnotKey
// folds party names, so membership tests line up).
func nameSet(people []participant) map[string]bool {
	set := map[string]bool{}
	for _, p := range people {
		set[strings.ToLower(strings.TrimSpace(p.Name))] = true
	}
	return set
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// runLeakEval is the privacy-boundary harness: for each case it distills the
// person's note and checks whether any planted secret crossed (a leak) and
// whether the legitimate coordination survived (must-cross). It spends one Distill
// call per case — the only billable part; the scoring (eval.DetectLeaks) is pure
// and unit-tested. Leak rate is the headline; a non-zero rate is printed loud.
func runLeakEval(ctx context.Context, det *ettlemesh.Detector, paths []string) error {
	var agg eval.LeakResult
	for _, path := range paths {
		lc, err := eval.LoadLeakCorpus(path)
		if err != nil {
			return fmt.Errorf("load leak corpus %s: %w", path, err)
		}
		fmt.Printf("\n  ══ %s ══ (%d cases · privacy-boundary leak test)\n", lc.Name, len(lc.Cases))
		for _, c := range lc.Cases {
			atoms, err := det.Distill(ctx, c.Person, c.Role, c.Note, c.Private)
			if err != nil {
				return fmt.Errorf("distill %s/%s: %w", lc.Name, c.ID, err)
			}
			leaks := eval.DetectLeaks(c, atoms)
			crossed := eval.CrossedMustCross(c, atoms)

			agg.Cases++
			agg.Secrets += len(c.Secrets)
			agg.Leaks = append(agg.Leaks, leaks...)
			if len(c.MustCross) > 0 {
				agg.MustCrossReq++
				if crossed {
					agg.MustCrossMet++
				}
			}

			util := "coordination survived"
			if len(c.MustCross) > 0 && !crossed {
				util = "COORDINATION DROPPED (over-redacted)"
			}
			fmt.Printf("    %-16s %d atoms crossed · %d/%d secrets held · %s\n",
				c.ID, len(atoms), len(c.Secrets)-len(leaks), len(c.Secrets), util)
			for _, l := range leaks {
				fmt.Printf("      ⚠ LEAK [%s] %s — marker %q crossed in: %s\n", l.Secret, l.Desc, l.Marker, l.InAtom)
			}
		}
	}
	fmt.Printf("\n  ══ leak rate ══\n")
	fmt.Printf("    %d/%d secrets leaked = %.0f%% leak rate · utility %d/%d must-cross kept = %.0f%%\n",
		len(agg.Leaks), agg.Secrets, 100*agg.LeakRate(), agg.MustCrossMet, agg.MustCrossReq, 100*agg.UtilityRate())
	fmt.Printf("    (synthetic corpus · liberal substring matcher: over-counts a leak before it under-counts — eyeball any non-zero)\n")
	return nil
}

// runLeakInferenceEval is the inference-CHANNEL privacy harness (docs/LEGIBILITY.md
// stage 1a): it runs the INFERENCE pass (not Distill) on a trap corpus and measures
// whether the agent manufactures a sensitive de-novo claim — a conclusion the source
// never stated. The standard --leak harness is blind to this: it scans for markers the
// person WROTE, and an inference is by definition something they did not write. Opt-in
// because it adds one inference call per case; the inference pass is designed to resist
// the bait (it infers operative coordination assumptions, not personal conclusions), so
// a near-zero rate here is the EXPECTED-but-now-MEASURED result, not an assumption.
func runLeakInferenceEval(ctx context.Context, det *ettlemesh.Detector, paths []string) error {
	var traps, leaked, inferredTotal, asQuestions int
	var hits []eval.Leak
	for _, path := range paths {
		lc, err := eval.LoadLeakCorpus(path)
		if err != nil {
			return fmt.Errorf("load leak corpus %s: %w", path, err)
		}
		fmt.Printf("\n  ══ %s ══ (%d cases · inference-channel trap test)\n", lc.Name, len(lc.Cases))
		for _, c := range lc.Cases {
			inferred, questions, err := det.InferImplicit(ctx, c.Person, c.Role, c.Note, c.Private)
			if err != nil {
				return fmt.Errorf("infer %s/%s: %w", lc.Name, c.ID, err)
			}
			leaks := eval.InferenceLeaks(c, inferred)
			traps += len(c.Secrets)
			leaked += len(leaks)
			inferredTotal += len(inferred)
			asQuestions += len(questions)
			hits = append(hits, leaks...)

			verdict := "no sensitive inference"
			if len(leaks) > 0 {
				verdict = "TRAP TRIPPED"
			}
			fmt.Printf("    %-18s %d inferred atom(s), %d demoted to question(s) · %s\n",
				c.ID, len(inferred), len(questions), verdict)
			for _, a := range inferred {
				fmt.Printf("      • inferred: [%s] %s — %s  (conf %.1f)\n", a.Typ, a.Subject, a.Content, a.Confidence)
			}
			for _, l := range leaks {
				fmt.Printf("      ⚠ INFERENCE LEAK [%s] %s — marker %q appeared in an INFERRED atom: %s\n", l.Secret, l.Desc, l.Marker, l.InAtom)
			}
		}
	}
	fmt.Printf("\n  ══ inference-channel rate ══\n")
	rate := 0.0
	if traps > 0 {
		rate = 100 * float64(leaked) / float64(traps)
	}
	fmt.Printf("    %d/%d traps tripped = %.0f%% inference-leak rate · %d inferred atoms crossed, %d demoted to questions (the safe path)\n",
		leaked, traps, rate, inferredTotal, asQuestions)
	fmt.Printf("    (synthetic traps · the inference pass is steered toward operative assumptions, so it should mostly resist — this MEASURES that rather than assuming it; eyeball any inferred atom by hand)\n")
	return nil
}

func printScore(label string, s eval.Score) {
	fmt.Printf("    %-22s precision %d/%d=%.2f · recall %d/%d=%.2f · would-ask %d",
		label, s.TP, s.TP+s.FP, s.Precision(), s.RecallHits, s.RecallTotal, s.Recall(), s.WouldAsk)
	if len(s.Missed) > 0 {
		fmt.Printf(" · missed %s", strings.Join(s.Missed, ","))
	}
	if len(s.TrapHits) > 0 {
		fmt.Printf(" · FELL FOR TRAP %s", strings.Join(s.TrapHits, ","))
	}
	fmt.Println()
}

// printCalibration prints the confidence-vs-accuracy bins and the ECE for one
// corpus's knots. No extra model call — it reuses the knots already detected. On a
// tiny corpus this is descriptive (few knots per bin), so it is labeled as such.
func printCalibration(knots []ettlemesh.Knot, labels []eval.Label) {
	bins, ece := eval.Calibration(knots, labels, 5)
	populated := 0
	for _, b := range bins {
		if b.N > 0 {
			populated++
		}
	}
	if populated == 0 {
		return
	}
	fmt.Printf("    %-22s ECE=%.2f (descriptive — sparse bins on a small corpus)\n", "calibration", ece)
	for _, b := range bins {
		if b.N == 0 {
			continue
		}
		fmt.Printf("      conf [%.1f-%.1f]  n=%-2d  mean-conf %.2f  vs  accuracy %.2f\n",
			b.Lo, b.Hi, b.N, b.MeanConf, b.Accuracy)
	}
}

func abVerdict(p float64, b, c int) string {
	if b+c < 6 {
		return "(too few discordant to test — no claim)"
	}
	if p < 0.05 {
		return "(significant)"
	}
	return "(not significant)"
}

// runDrift demonstrates L2 — the directed dyadic models — across two rounds. It
// distills each person's notes in a PREVIOUS snapshot and a CURRENT one, seeds the
// directed mesh from the previous round (every observer's model of every teammate),
// then computes the CURRENT round's surprise: the deltas that — and only those that
// — would leave a teammate's model of someone stale. This is the principled emit
// rule and the surprise = L2-vs-L1 divergence definition, made runnable; the payoff
// is that round 2 re-sends a changed belief to exactly the teammates it affects, not
// a whole-team rebroadcast. Same cost shape as a standup, one Distill per person per
// round; the L2 diff itself is deterministic and free.
// loadAndDetect loads both round dirs, warns on a roster mismatch, and builds the
// detector — the shared front of `drift` and `mirror` (one pipeline, two views; kept
// single so the two commands can't drift apart).
func loadAndDetect(prevDir, currDir, model string) (prev, curr []participant, det *ettlemesh.Detector, err error) {
	if prev, err = loadDir(prevDir); err != nil {
		return nil, nil, nil, err
	}
	if curr, err = loadDir(currDir); err != nil {
		return nil, nil, nil, err
	}
	if cfg, cur := names(prev), names(curr); strings.Join(cfg, ",") != strings.Join(cur, ",") {
		// Same roster both rounds: drift is per-person across time, so a name present
		// in only one round has no counterpart to diff. Make the mismatch loud.
		fmt.Fprintf(os.Stderr, "ettle: WARNING round rosters differ (prev: %s; curr: %s) — a person missing from one round can't be diffed.\n",
			strings.Join(cfg, ", "), strings.Join(cur, ", "))
	}
	key := apiKey()
	if key == "" {
		return nil, nil, nil, fmt.Errorf("no ANTHROPIC_API_KEY (set it in the environment or a .env file)")
	}
	client := anthropic.NewClient(option.WithAPIKey(key), option.WithMaxRetries(4))
	return prev, curr, ettlemesh.NewDetector(&client, model), nil
}

// buildMesh runs the two-round L2 pipeline shared by `drift` and `mirror`: distill
// both rounds (reusing an unchanged note's prior atoms — the emit-gate discipline one
// layer down, so only a truly changed note re-distills and only real deltas cross),
// seed the mesh from round 1, and compute round 2's surprise-gated deltas. Pure of any
// rendering — drift and mirror present this same state two ways. The only model calls
// are the distill; the L2 projection adds none.
func buildMesh(ctx context.Context, det *ettlemesh.Detector, prev, curr []participant) (state *ettlemesh.MeshState, currSelf map[string][]ettlemesh.Atom, seed, deltas []ettlemesh.Emission, reused []string, err error) {
	prevSelf, err := distillRound(ctx, det, prev)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	currSelf, reused, err = distillCurrent(ctx, det, curr, prev, prevSelf)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	state = ettlemesh.NewMeshState()
	seed = state.Advance(prevSelf)    // round 1: seed every directed model
	deltas = state.Surprise(currSelf) // round 2: the surprise-gated deltas (read-only)
	return state, currSelf, seed, deltas, reused, nil
}

func runDrift(args []string) error {
	fs := flag.NewFlagSet("drift", flag.ExitOnError)
	me := fs.String("me", "", "show the directed view for this participant; empty = whole team")
	model := fs.String("model", "claude-haiku-4-5", "model id")
	_ = fs.Parse(args)
	rest := fs.Args()
	if len(rest) != 2 {
		return fmt.Errorf("usage: ettle drift [--me name] <prev-round-dir> <curr-round-dir>\n" +
			"  each dir holds one note per participant; same filename = same person across rounds")
	}
	prev, curr, det, err := loadAndDetect(rest[0], rest[1], *model)
	if err != nil {
		return err
	}
	if *me != "" && !hasParticipant(curr, *me) {
		return fmt.Errorf("--me %q matches none of the current-round participants (%s)", *me, strings.Join(names(curr), ", "))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	state, currSelf, seed, deltas, reused, err := buildMesh(ctx, det, prev, curr)
	if err != nil {
		return err
	}
	printDrift(*me, state, seed, deltas, currSelf, reused)
	return nil
}

// runMirror is the read side of the one-way mirror (docs/LEGIBILITY.md stage 1b): it
// shows one person what the team's directed models (L2) currently believe ABOUT them,
// and which of those beliefs are stale. Same pipeline as drift, subject-centric view.
func runMirror(args []string) error {
	fs := flag.NewFlagSet("mirror", flag.ExitOnError)
	me := fs.String("me", "", "whose mirror — the person the beliefs are ABOUT (required)")
	model := fs.String("model", "claude-haiku-4-5", "model id")
	byObserver := fs.Bool("by-observer", false, "attribute each belief to the teammate holding it (default: coarsened — the belief, not who holds it; see docs/LEGIBILITY.md)")
	_ = fs.Parse(args)
	rest := fs.Args()
	if len(rest) != 2 {
		return fmt.Errorf("usage: ettle mirror --me name [--by-observer] <prev-round-dir> <curr-round-dir>\n" +
			"  shows what the team's directed models (L2) currently believe ABOUT you, and which beliefs are stale")
	}
	if *me == "" {
		return fmt.Errorf("--me is required for mirror (it shows the beliefs held ABOUT one person)")
	}
	prev, curr, det, err := loadAndDetect(rest[0], rest[1], *model)
	if err != nil {
		return err
	}
	if !hasParticipant(curr, *me) {
		return fmt.Errorf("--me %q matches none of the current-round participants (%s)", *me, strings.Join(names(curr), ", "))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	state, currSelf, _, _, _, err := buildMesh(ctx, det, prev, curr)
	if err != nil {
		return err
	}
	printMirror(*me, *byObserver, state, currSelf)
	return nil
}

// printDrift renders the two-round directed view: how the mesh seeded, what changed
// this round (the surprise-gated deltas, grouped by who changed and listing the
// teammates whose model went stale), the routing savings, and — with --me — the
// caller's own stale beliefs about each teammate.
func printDrift(me string, state *ettlemesh.MeshState, seed, deltas []ettlemesh.Emission, currSelf map[string][]ettlemesh.Atom, reused []string) {
	seedBeliefs := 0
	for _, e := range seed {
		seedBeliefs += len(e.Atoms)
	}
	sentTotal := 0
	for _, e := range deltas {
		sentTotal += len(e.Atoms)
	}

	who := "the team"
	if me != "" {
		who = me
	}
	fmt.Printf("\n  ettle — directed drift (L2) for %s\n", who)
	fmt.Printf("  %s · round 1 seeded %s across %d directed models\n",
		plural(len(currSelf), "person", "people"),
		plural(seedBeliefs, "belief", "beliefs"), len(state.Snapshot()))

	// N=1 (or 0): L2 is a CROSS-person layer, so a single participant has no directed
	// signal by construction. Say so explicitly — never print the vacuous "every model
	// is current," which reads as "drift was checked and found clean" when no pair
	// could have been checked at all.
	if len(currSelf) < 2 {
		fmt.Println("\n  single participant — L2 has no cross-person signal by construction (it needs at least two people).")
		fmt.Println()
		return
	}

	// Round-1 roster (everyone who had a seeded model) vs round-2 roster, so a NEW
	// arrival (seeding) isn't mislabeled "changed", and a DEPARTED person (submitted
	// nothing this round) is surfaced rather than silently skipped.
	prevRoster := map[string]bool{}
	for _, e := range seed {
		prevRoster[strings.ToLower(strings.TrimSpace(e.Subject))] = true
	}
	isNewArrival := func(subj string) bool { return !prevRoster[strings.ToLower(strings.TrimSpace(subj))] }

	// Group this round's deltas by the subject; the atom set is the subject-side delta
	// (identical across that subject's observers), so show it once and list receivers.
	bySubject := map[string][]ettlemesh.Emission{}
	var subjects []string
	for _, e := range deltas {
		if _, ok := bySubject[e.Subject]; !ok {
			subjects = append(subjects, e.Subject)
		}
		bySubject[e.Subject] = append(bySubject[e.Subject], e)
	}
	sort.Strings(subjects)

	section("what moved this round (surprise-gated deltas, routed to whoever's model went stale)")
	shown := 0
	for _, subj := range subjects {
		ems := bySubject[subj]
		var receivers []string
		for _, e := range ems {
			receivers = append(receivers, e.Observer)
		}
		// --me view: show it if it's mine to broadcast OR it reaches me.
		if me != "" && !ettlemesh.SamePerson(subj, me) && !containsPerson(receivers, me) {
			continue
		}
		shown++
		atoms := ems[0].Atoms
		if isNewArrival(subj) {
			// First time seen: everyone learns them. NOT a change/staleness — labeling
			// a new arrival's whole self-model as "changed" would be a false drift.
			fmt.Printf("    %s joined this round — seeding %s (the team had no prior model):\n", subj, plural(len(atoms), "belief", "beliefs"))
		} else {
			fmt.Printf("    %s moved %s:\n", subj, plural(len(atoms), "belief", "beliefs"))
		}
		for _, a := range atoms {
			tag := "stated"
			if a.Inferred {
				tag = fmt.Sprintf("inferred %.1f", a.Confidence)
			}
			fmt.Printf("      • [%s] %s — %s  (%s)\n", a.Typ, a.Subject, a.Content, tag)
		}
		if isNewArrival(subj) {
			fmt.Printf("      → reaches %s (initial model)\n", strings.Join(receivers, ", "))
		} else {
			fmt.Printf("      → reaches %s — their model of %s had gone stale\n", strings.Join(receivers, ", "), subj)
		}
	}
	if shown == 0 {
		fmt.Println("    — nothing; every model is current.")
	}

	// Departed: in round 1's roster, absent from round 2. No new state for them, so no
	// delta to route — but say so, rather than let teammates' now-aging models of them
	// pass as fresh.
	currNames := make([]string, 0, len(currSelf))
	for n := range currSelf {
		currNames = append(currNames, n)
	}
	var departed []string
	for _, e := range seed {
		if !containsPerson(currNames, e.Subject) && !containsPerson(departed, e.Subject) {
			departed = append(departed, e.Subject)
		}
	}
	if len(departed) > 0 {
		sort.Strings(departed)
		fmt.Printf("    (no submission this round from %s — teammates' models of them are aging, not confirmed)\n", strings.Join(departed, ", "))
	}

	// The L2 payoff, stated as a number: a broadcast would have re-sent everything;
	// the surprise gate sent only the deltas.
	fmt.Printf("\n  routed: round 1 broadcast %d beliefs; round 2 sent %d — the emit gate withheld the rest (no machine-speed rebroadcast).\n",
		seedBeliefs, sentTotal)
	if len(reused) > 0 {
		sort.Strings(reused)
		fmt.Printf("  unchanged this round (note byte-identical, self-model reused, nothing re-distilled): %s\n", strings.Join(reused, ", "))
	}

	// --me: the caller's own stale beliefs about each teammate (the other direction —
	// what the caller now has WRONG until the delta lands).
	if me != "" {
		var mine []ettlemesh.Drift
		for subj := range currSelf {
			if ettlemesh.SamePerson(subj, me) {
				continue
			}
			if model, ok := state.ModelOf(me, subj); ok {
				mine = append(mine, ettlemesh.StaleBeliefs(model, currSelf[subj])...)
			}
		}
		if len(mine) > 0 {
			section(fmt.Sprintf("what %s's model now has stale (until the delta lands)", me))
			sort.Slice(mine, func(i, j int) bool { return mine[i].Subject < mine[j].Subject })
			for _, d := range mine {
				switch d.Kind {
				case ettlemesh.DriftDrifted:
					// Reliable: the same slot is present with different content.
					fmt.Printf("    • you believe %s: %q — they now hold: %q\n", d.Subject, d.Believed.Content, d.Actual.Content)
				case ettlemesh.DriftDropped:
					// HEDGED, not asserted: the slot has no exact match in their current
					// model. That is EITHER a real drop OR a re-distill that reworded the
					// subject — the structural (exact-key) diff can't tell which (known
					// limit; see ettlemesh/directed.go beliefKey). So flag it for a look,
					// don't claim they abandoned it.
					fmt.Printf("    • you believe %s: %q — no matching current belief (they dropped it, or a re-distill reworded it — worth a check)\n", d.Subject, d.Believed.Content)
				}
			}
		}
	}
	fmt.Println()
}

// atomIdent is a display-only identity for matching a specific atom value back to a
// Drift (full value, not the engine's type+subject slot) — used only to mark which
// mirror beliefs are stale, never to define slot semantics.
func atomIdent(a ettlemesh.Atom) string {
	return string(a.Typ) + "|" + a.Subject + "|" + a.Content
}

// printMirror is the read side of the one-way mirror (docs/LEGIBILITY.md stage 1b):
// it shows `me` what the team's directed models (L2) currently believe ABOUT them and
// flags the beliefs that have gone stale (me has drifted from what others still hold)
// — surfacing them first, since a stale belief about you is the one most worth fixing.
// The layer that drives how a person is treated, made readable to that person. Pure.
//
// Attribution is COARSENED by default — the belief, not which teammate holds it —
// because "alice believes X about you" surfaces alice's private model, a flow that
// touches her; turning the mirror around must not become a surveillance surface
// pointed at the modelers. --by-observer (byObserver=true) opts into attribution.
func printMirror(me string, byObserver bool, state *ettlemesh.MeshState, currSelf map[string][]ettlemesh.Atom) {
	fmt.Printf("\n  ettle — mirror: what the team's models believe about %s\n", me)

	observers := make([]string, 0, len(currSelf))
	for o := range currSelf {
		if !ettlemesh.SamePerson(o, me) {
			observers = append(observers, o)
		}
	}
	sort.Strings(observers)

	// Collect every observer's model of me (me as SUBJECT).
	type modelOfMe struct {
		observer string
		beliefs  []ettlemesh.Atom
	}
	var models []modelOfMe
	for _, o := range observers {
		if m, ok := state.ModelOf(o, me); ok && len(m.Beliefs) > 0 {
			models = append(models, modelOfMe{observer: o, beliefs: m.Beliefs})
		}
	}
	if len(models) == 0 {
		fmt.Println("\n  no teammate holds a model of you yet — the mirror needs at least two people across two rounds.")
		fmt.Println()
		return
	}

	staleLine := func(d ettlemesh.Drift) string {
		if d.Kind == ettlemesh.DriftDrifted {
			return fmt.Sprintf("      ⚠ stale — they still hold %q; you now hold %q", d.Believed.Content, d.Actual.Content)
		}
		// Dropped is HEDGED (a re-distill reword looks the same as a real drop — the
		// structural diff can't tell; see ettlemesh/directed.go beliefKey).
		return fmt.Sprintf("      ⚠ maybe stale — they still hold %q; no matching current belief of yours (you dropped it, or a re-distill reworded it)", d.Believed.Content)
	}

	if byObserver {
		// Attributed view (opt-in): each teammate's model of you, stale beliefs flagged.
		for _, m := range models {
			section(fmt.Sprintf("%s's model of you", m.observer))
			staleByIdent := map[string]ettlemesh.Drift{}
			for _, d := range ettlemesh.StaleBeliefs(ettlemesh.DirectedModel{Observer: m.observer, Subject: me, Beliefs: m.beliefs}, currSelf[me]) {
				staleByIdent[atomIdent(d.Believed)] = d
			}
			for _, b := range ettlemesh.Canonical(m.beliefs) {
				fmt.Printf("    • [%s] %s — %s\n", b.Typ, b.Subject, b.Content)
				if d, ok := staleByIdent[atomIdent(b)]; ok {
					fmt.Println(staleLine(d))
				}
			}
		}
		fmt.Println()
		return
	}

	// Coarsened view (default): the union of beliefs held about you across all
	// teammates, deduped on the engine's slot identity — the belief, not who holds it.
	var union []ettlemesh.Atom
	for _, m := range models {
		union = append(union, m.beliefs...)
	}
	union = ettlemesh.Canonical(union)
	staleByIdent := map[string]ettlemesh.Drift{}
	for _, d := range ettlemesh.StaleBeliefs(ettlemesh.DirectedModel{Subject: me, Beliefs: union}, currSelf[me]) {
		staleByIdent[atomIdent(d.Believed)] = d
	}
	section("the team's model of you (which beliefs are current, which have gone stale)")
	for _, b := range union {
		fmt.Printf("    • [%s] %s — %s\n", b.Typ, b.Subject, b.Content)
		if d, ok := staleByIdent[atomIdent(b)]; ok {
			fmt.Println(staleLine(d))
		}
	}
	fmt.Printf("\n  (attribution coarsened — run with --by-observer to see which teammate holds each.)\n")
	fmt.Println()
}

// containsPerson reports whether who is in the list (SamePerson identity).
func containsPerson(list []string, who string) bool {
	for _, p := range list {
		if ettlemesh.SamePerson(p, who) {
			return true
		}
	}
	return false
}

// distillRound distills every participant's notes into their self-model (stated
// atoms only — drift is about how the stated state changes between rounds, so the
// inference pass is skipped here to keep the diff clean and the cost one call per
// person). Keyed by participant name; runs in parallel, concurrency-capped.
func distillRound(ctx context.Context, det *ettlemesh.Detector, people []participant) (map[string][]ettlemesh.Atom, error) {
	out := make(map[string][]ettlemesh.Atom, len(people))
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(4)
	for _, p := range people {
		p := p
		g.Go(func() error {
			a, err := det.Distill(gctx, p.Name, p.Role, p.Notes, p.Private)
			if err != nil {
				return fmt.Errorf("distill %s: %w", p.Name, err)
			}
			mu.Lock()
			out[p.Name] = a
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return out, nil
}

// distillCurrent builds the current round's self-models, reusing the prior round's
// atoms verbatim for any participant whose note is UNCHANGED from last round (same
// self-model → no point re-rolling the stochastic distiller) and distilling only the
// rest. Returns the current self-models keyed by name and the names reused unchanged.
// This gates the L1 model call on note-change, so the L2 surprise gate downstream
// sees real deltas, not distillation reword noise.
//
// "Unchanged" is compared on whitespace-normalized text (trim + collapse runs), not
// raw bytes, so a trailing newline or a reflow doesn't force a re-distill and the
// reword storm that follows — the cheap mitigation for the structural diff's wording
// sensitivity. It does NOT make a genuine one-word edit free: that still re-distills
// in full (and its unchanged beliefs can reword); closing that needs wording-
// independent slot identity (see ettlemesh/directed.go beliefKey, tracked next step).
func distillCurrent(ctx context.Context, det *ettlemesh.Detector, curr, prev []participant, prevSelf map[string][]ettlemesh.Atom) (map[string][]ettlemesh.Atom, []string, error) {
	prevNote := map[string]string{} // normalized name → normalized prior note text
	prevName := map[string]string{} // normalized name → prior display name (prevSelf key)
	for _, p := range prev {
		k := strings.ToLower(strings.TrimSpace(p.Name))
		prevNote[k] = normalizeNote(p.Notes)
		prevName[k] = p.Name
	}
	out := make(map[string][]ettlemesh.Atom, len(curr))
	var reused []string
	var changed []participant
	for _, p := range curr {
		k := strings.ToLower(strings.TrimSpace(p.Name))
		if note, ok := prevNote[k]; ok && note == normalizeNote(p.Notes) {
			out[p.Name] = prevSelf[prevName[k]] // unchanged note → identical self-model
			reused = append(reused, p.Name)
			continue
		}
		changed = append(changed, p)
	}
	fresh, err := distillRound(ctx, det, changed)
	if err != nil {
		return nil, nil, err
	}
	for name, atoms := range fresh {
		out[name] = atoms
	}
	return out, reused, nil
}

// normalizeNote canonicalizes note text for the reuse-gate comparison: trim, then
// collapse every internal whitespace run (including newlines) to a single space. So
// a trailing newline, a reflowed paragraph, or doubled spacing reads as "unchanged"
// and the note's prior self-model is reused rather than re-distilled.
func normalizeNote(s string) string { return strings.Join(strings.Fields(s), " ") }

// loadDir loads one participant per note file (*.md or *.jsonl) in a round
// directory. Same filename across two round dirs = the same person over time.
func loadDir(dir string) ([]participant, error) {
	var paths []string
	for _, pat := range []string{"*.md", "*.jsonl"} {
		m, err := filepath.Glob(filepath.Join(dir, pat))
		if err != nil {
			return nil, err
		}
		paths = append(paths, m...)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no .md or .jsonl note files in %s", dir)
	}
	return loadParticipants(paths)
}

// runCapture previews the L1 digest a session transcript distills to — what
// would cross the boundary before any model call. The raw transcript stays
// local; this shows the lossy, privacy-respecting extraction.
func runCapture(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ettle capture <transcript.jsonl>")
	}
	for _, path := range args {
		s, err := capture.Read(path)
		if err != nil {
			return fmt.Errorf("capture %s: %w", path, err)
		}
		fmt.Printf("\n  ── %s ──\n", filepath.Base(path))
		if s.Empty() {
			fmt.Println("  (no L1 signal extracted — no prompts, edits, or commands)")
			continue
		}
		fmt.Println(s.Digest())
		fmt.Println()
	}
	return nil
}

// runMCP serves the coordination engine over an MCP stdio transport, so any MCP
// client (Claude Code, Cursor) can drive it: each participant's own agent calls
// ettle_emit with that person's notes, and ettle_horizon reconciles the team's
// atoms into coordination knots. The differentiated surface is the knot, not the
// per-person summary; MCP (not a meeting bot) is the consent-clean shape — see
// internal/mcpserver and docs/ADOPTION.md.
func runMCP(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	model := fs.String("model", "claude-haiku-4-5", "model id")
	noGround := fs.Bool("no-ground", false, "disable the cross-person coupling check (ON by default — see ground.go)")
	_ = fs.Parse(args)

	key := apiKey()
	if key == "" {
		return fmt.Errorf("no ANTHROPIC_API_KEY (set it in the environment or a .env file)")
	}
	client := anthropic.NewClient(option.WithAPIKey(key), option.WithMaxRetries(4))
	det := ettlemesh.NewDetector(&client, *model)
	det.Ground = !*noGround

	// The stdio MCP server owns stdout (the JSON-RPC channel); diagnostics go to
	// stderr. Run until the client disconnects or the process is interrupted.
	fmt.Fprintf(os.Stderr, "ettle mcp: serving on stdio (model %s) — tools: ettle_emit, ettle_horizon, ettle_self_check\n", *model)
	return mcpserver.Serve(context.Background(), det, buildVersion())
}

// loadParticipants reads one participant per input. A `.jsonl` input is a Claude
// Code session transcript — the live-reasoning L1 source — distilled to a digest
// by capture. Any other input is a note file: optional leading `name:` / `role:`
// lines set identity (and an optional `private:` line lists comma-separated
// phrases to keep off the boundary), otherwise the filename (sans extension) is
// the name, and everything after the header is the private note.
func loadParticipants(paths []string) ([]participant, error) {
	var out []participant
	for _, path := range paths {
		// Corpus metadata is never a person. A `standup testdata/dir/*.md` glob
		// will sweep up PROVENANCE.md / README.md sitting beside the notes; left
		// in, they become a phantom participant AND their text (which often
		// describes the eval itself) contaminates the distilled atoms. Skip them.
		if base := strings.ToUpper(filepath.Base(path)); base == "PROVENANCE.MD" || base == "README.MD" {
			continue
		}
		if strings.EqualFold(filepath.Ext(path), ".jsonl") {
			s, err := capture.Read(path)
			if err != nil {
				return nil, fmt.Errorf("capture %s: %w", path, err)
			}
			name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			out = append(out, participant{Name: name, Role: "", Notes: s.Digest()})
			continue
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		role := ""
		var private []string
		var body []string
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		header := true
		for sc.Scan() {
			ln := sc.Text()
			if header {
				low := strings.ToLower(strings.TrimSpace(ln))
				switch {
				case strings.HasPrefix(low, "name:"):
					// Only override the filename-derived default if a value is
					// actually given. A bare `name:` must NOT blank the name: an
					// empty From collides with the `--me ""` full-team sentinel, so
					// that participant could never be targeted and their atoms would
					// bucket with everyone else's.
					if v := strings.TrimSpace(ln[strings.Index(ln, ":")+1:]); v != "" {
						name = v
					}
					continue
				case strings.HasPrefix(low, "role:"):
					role = strings.TrimSpace(ln[strings.Index(ln, ":")+1:])
					continue
				case strings.HasPrefix(low, "private:"):
					// Per-person privacy override: comma-separated phrases the
					// developer marks private. Fed to BOTH boundary layers via
					// Distill (semantic suppress-list + structural scrub).
					private = splitPrivate(ln[strings.Index(ln, ":")+1:])
					continue
				case strings.TrimSpace(ln) == "":
					continue
				default:
					header = false
				}
			}
			body = append(body, ln)
		}
		f.Close()
		if err := sc.Err(); err != nil {
			return nil, err
		}
		out = append(out, participant{Name: name, Role: role, Private: private, Notes: strings.TrimSpace(strings.Join(body, "\n"))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// splitPrivate parses a `private:` header value into trimmed, non-empty phrases.
// Comma-separated so a note can mark several ("compensation, my health situation").
func splitPrivate(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// apiKey returns ANTHROPIC_API_KEY from the environment or a .env file in the
// working directory (KEY=value lines). The key is never written anywhere.
func apiKey() string {
	if k := os.Getenv("ANTHROPIC_API_KEY"); k != "" {
		return k
	}
	f, err := os.Open(".env")
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		ln := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(ln, "ANTHROPIC_API_KEY=") {
			return strings.Trim(strings.TrimPrefix(ln, "ANTHROPIC_API_KEY="), `"'`)
		}
	}
	return ""
}
