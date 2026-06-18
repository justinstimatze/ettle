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
		}
	}
	if len(os.Args) < 2 || os.Args[1] != "standup" {
		fmt.Fprintln(os.Stderr, "usage: ettle standup [flags] <input>...")
		fmt.Fprintln(os.Stderr, "  each input is one participant: a note file, or a Claude Code")
		fmt.Fprintln(os.Stderr, "  session transcript (.jsonl) — the live-reasoning L1 source.")
		fmt.Fprintln(os.Stderr, "  ettle capture <transcript.jsonl>   # preview what a session distills to")
		fmt.Fprintln(os.Stderr, "  ettle drift <prev-dir> <curr-dir>  # L2: directed models + surprise-gated deltas across two rounds")
		fmt.Fprintln(os.Stderr, "  cost: ~2N+3 model calls per sample for N participants; voting defaults to --samples 5 (set --samples 1 to disable)")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("standup", flag.ExitOnError)
	me := fs.String("me", "", "surface knots relevant to this participant (their agent's view); empty = full team view")
	model := fs.String("model", "claude-haiku-4-5", "model id")
	gemotURL := fs.String("gemot", "", "gemot MCP endpoint for contested knots (e.g. https://gemot.example/mcp); empty = inline either/or")
	transportName := fs.String("transport", "inproc", "atom transport: inproc | nats (nats needs -tags nats)")
	insecureLocal := fs.Bool("insecure-local", false, "dev only: allow plaintext/tokenless connections to localhost gemot + NATS (e.g. local docker)")
	gemotTimeout := fs.Duration("gemot-timeout", 180*time.Second, "how long to wait for a gemot deliberation's analysis")
	samples := fs.Int("samples", 5, "run the reconcile passes N times; recurrence frequency ranks knots firm (assert) vs soft (ask) — majority-recurring knots are asserted, flickery ones become questions (not dropped). N=1 disables voting and falls back to confidence. Costs N× the reconcile calls.")
	showAtoms := fs.Bool("show-atoms", false, "print exactly what crosses the boundary (each person's typed atoms) before surfacing knots")
	ground := fs.Bool("ground", false, "experimental: run the semantic grounding pass on cross-person knots (off by default — a measured negative result; see ground.go)")
	groundModel := fs.String("ground-model", "", "verify cross-person knots with this (stronger) model instead of --model; empty = same as --model")
	_ = fs.Parse(os.Args[2:])

	cfg := runConfig{me: *me, model: *model, gemotURL: *gemotURL, transport: *transportName, insecureLocal: *insecureLocal, gemotTimeout: *gemotTimeout, samples: *samples, showAtoms: *showAtoms, ground: *ground, groundModel: *groundModel, paths: fs.Args()}
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
	me, model, gemotURL, transport string
	insecureLocal                  bool
	showAtoms                      bool
	ground                         bool
	groundModel                    string
	gemotTimeout                   time.Duration
	samples                        int
	paths                          []string
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

	bus, err := busFor(cfg.transport, cfg.insecureLocal)
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
		printAtoms(results)
	}
	// 2: publish each person's atoms over the seam.
	var allQuestions []authoredQuestion
	for _, r := range results {
		for _, q := range r.questions {
			allQuestions = append(allQuestions, authoredQuestion{who: r.name, text: q})
		}
		if err := bus.Publish(ctx, transport.Envelope{Participant: r.name, Role: r.role, Atoms: r.atoms}); err != nil {
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
	atoms := transport.Atoms(envs)
	// Cross-person detection (pairwise + team-wide). With --samples>1, runs the
	// passes N times and keeps only knots that recur across a majority — the
	// stochastic detector's noise becomes a confidence signal.
	knots, err := det.ReconcileVoted(ctx, atoms, cfg.samples)
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
	// Grounding pass: drop cross-person knots whose parties don't share a concrete
	// referent (a no-op when det.Ground is off — set via --no-ground).
	knots, err = det.GroundKnots(ctx, knots, atoms)
	if err != nil {
		return fmt.Errorf("ground: %w", err)
	}

	// 4+5: resolve contested knots, then surface to --me.
	var resolver crux.Resolver = crux.Inline{}
	if cfg.gemotURL != "" {
		resolver = crux.Gemot{URL: cfg.gemotURL, Token: os.Getenv("ETTLE_GEMOT_TOKEN"), InsecureLocal: cfg.insecureLocal, Timeout: cfg.gemotTimeout}
	}
	surface(ctx, cfg.me, knots, atoms, allQuestions, resolver)
	return nil
}

// surface prints the knots relevant to `me` (or all, in team view), routed FIRM
// vs SOFT, with contested knots resolved. This is the agent → its own human.
func surface(ctx context.Context, me string, knots []ettlemesh.Knot, atoms []ettlemesh.Atom, questions []authoredQuestion, resolver crux.Resolver) {
	var firm, soft []ettlemesh.Knot
	for _, k := range knots {
		if me != "" && !partyOf(k, me) {
			continue
		}
		if k.Firm() {
			firm = append(firm, k)
		} else {
			soft = append(soft, k)
		}
	}

	who := "the team"
	if me != "" {
		who = me
	}
	fmt.Printf("\n  ettle — coordination horizon for %s\n", who)
	fmt.Printf("  %s across %s; %s surfaced\n",
		plural(len(atoms), "atom", "atoms"),
		plural(countPeople(atoms), "person", "people"),
		plural(len(firm)+len(soft), "knot", "knots"))

	section("worth a look (firm)")
	if len(firm) == 0 {
		fmt.Println("    — nothing; the horizon is clear.")
	}
	for _, k := range firm {
		printKnot(k)
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

	if len(soft) > 0 {
		section("worth a question (soft — surfaced by a minority of samples, or rests on an inference)")
		for _, k := range soft {
			printKnot(k)
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
	fmt.Println()
}

func printKnot(k ettlemesh.Knot) {
	vote := ""
	if k.Samples > 0 {
		vote = fmt.Sprintf(" · seen in %d/%d samples", k.Votes, k.Samples)
	}
	fmt.Printf("    • [%s] %s\n      %s\n      parties: %s · confidence %.1f%s\n",
		k.Kind, k.About, k.Explanation, strings.Join(k.Parties, ", "), k.Confidence, vote)
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

// personResult is one participant's distilled atoms + the questions the
// inference step couldn't answer confidently.
type personResult struct {
	name, role string
	atoms      []ettlemesh.Atom
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
			results[i] = personResult{name: p.Name, role: p.Role, atoms: append(a, inf...), questions: qs}
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
func printAtoms(results []personResult) {
	fmt.Printf("\n  atoms crossing the boundary (this is ALL that leaves each machine):\n")
	for _, r := range results {
		fmt.Printf("    %s:\n", r.name)
		if len(r.atoms) == 0 {
			fmt.Println("      (none)")
		}
		for _, a := range r.atoms {
			tag := "stated"
			if a.Inferred {
				tag = fmt.Sprintf("inferred %.1f", a.Confidence)
			}
			fmt.Printf("      • [%s] %s — %s  (%s)\n", a.Typ, a.Subject, a.Content, tag)
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
	var atoms []ettlemesh.Atom
	for _, r := range results {
		atoms = append(atoms, r.atoms...)
	}
	knots, err := det.ReconcileVoted(ctx, atoms, samples)
	if err != nil {
		return nil, nil, err
	}
	self, err := det.ReconcileSelf(ctx, atoms)
	if err != nil {
		return nil, nil, err
	}
	knots = append(knots, ettlemesh.DedupeSelf(self, knots)...)
	// Grounding pass: drop cross-person knots whose parties don't share a concrete
	// referent (a no-op when det.Ground is off — set via --no-ground).
	knots, err = det.GroundKnots(ctx, knots, atoms)
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
	stability := fs.Bool("stability", false, "determinism mode: run each corpus K times and report run-to-run knot-set agreement (Jaccard)")
	runs := fs.Int("runs", 5, "number of repeated runs for --stability")
	superposition := fs.Bool("superposition", false, "locality mode: check f(A∪B)=f(A)∪f(B) for independent groups — flags fabricated cross-group knots")
	separability := fs.Bool("separability", false, "diagnostic: over K joint runs, contrast fabricated vs real knots on recurrence-frequency and confidence (picks the fork: voting/threshold vs upstream grounding)")
	ground := fs.Bool("ground", false, "experimental: run the semantic grounding pass on cross-person knots (off by default — a measured negative result; see ground.go)")
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
	det.Ground = *ground
	det.GroundModel = *groundModel

	if *leak {
		return runLeakEval(ctx, det, fs.Args())
	}
	if *stability {
		return runStabilityEval(ctx, det, fs.Args(), *runs)
	}
	if *superposition {
		return runSuperpositionEval(ctx, det, fs.Args(), *runs)
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
func runSuperpositionEval(ctx context.Context, det *ettlemesh.Detector, paths []string, runs int) error {
	if runs < 1 {
		runs = 1
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

		keysA, err := detectKeys(ctx, det, peopleA)
		if err != nil {
			fmt.Printf("    ⚠ group A detection failed (skipped): %v\n", err)
			continue
		}
		keysB, err := detectKeys(ctx, det, peopleB)
		if err != nil {
			fmt.Printf("    ⚠ group B detection failed (skipped): %v\n", err)
			continue
		}
		fmt.Printf("    f(A)=%d knots · f(B)=%d knots (intra-group baseline)\n", len(keysA), len(keysB))

		fabricated := map[string]bool{} // union of distinct cross-boundary links seen
		var perRun []int                // cross-boundary count per run (for the stats bands)
		var localitySum float64
		var dropped, spurious, orphan int
		ok := 0
		for i := 0; i < runs; i++ {
			keysAB, err := detectKeys(ctx, det, joint)
			if err != nil {
				fmt.Printf("    ⚠ joint run %d failed: %v\n", i+1, err)
				continue
			}
			ok++
			r := eval.ComputeSuperposition(keysA, keysB, keysAB, groupA, groupB)
			localitySum += r.LocalityScore()
			dropped += len(r.Dropped)
			spurious += len(r.SpuriousIntra)
			orphan += len(r.Orphan)
			perRun = append(perRun, len(r.CrossBoundary))
			for _, k := range r.CrossBoundary {
				fabricated[k] = true
			}
		}
		if ok == 0 {
			continue
		}

		st := eval.ComputeSuperStats(perRun)
		// Continuous mean (primary A/B signal — lower variance) with its 95% band,
		// then the coarser binary rate with its Wilson interval. Overlapping bands
		// between two conditions = the run did not distinguish them (underpowered).
		fmt.Printf("    %-22s %.2f knots/run  [95%% CI %.2f–%.2f]  — PROVABLY FABRICATED (primary signal)\n",
			"CROSS-BOUNDARY MEAN", st.MeanPerRun, st.MeanCILow, st.MeanCIHigh)
		fmt.Printf("    %-22s %.0f%% of %d runs invented ≥1 A↔B knot  [95%% CI %.0f–%.0f%%]\n",
			"  (binary rate)", st.Rate*100, ok, st.RateCILow*100, st.RateCIHigh*100)
		for _, k := range sortedKeys(fabricated) {
			fmt.Printf("      %s\n", prettyKey(k))
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

// detectKeys runs one detection cycle and returns its stability-key set.
func detectKeys(ctx context.Context, det *ettlemesh.Detector, people []participant) (map[string]bool, error) {
	firm, soft, err := detectFor(ctx, det, people, 1)
	if err != nil {
		return nil, err
	}
	return eval.RunKeys(firm, soft), nil
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
	prev, err := loadDir(rest[0])
	if err != nil {
		return err
	}
	curr, err := loadDir(rest[1])
	if err != nil {
		return err
	}
	if cfg, cur := names(prev), names(curr); strings.Join(cfg, ",") != strings.Join(cur, ",") {
		// Same roster both rounds: drift is per-person across time, so a name present
		// in only one round has no counterpart to diff. Make the mismatch loud.
		fmt.Fprintf(os.Stderr, "ettle: WARNING round rosters differ (prev: %s; curr: %s) — a person missing from one round can't be diffed.\n",
			strings.Join(cfg, ", "), strings.Join(cur, ", "))
	}
	if *me != "" && !hasParticipant(curr, *me) {
		return fmt.Errorf("--me %q matches none of the current-round participants (%s)", *me, strings.Join(names(curr), ", "))
	}
	key := apiKey()
	if key == "" {
		return fmt.Errorf("no ANTHROPIC_API_KEY (set it in the environment or a .env file)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	client := anthropic.NewClient(option.WithAPIKey(key), option.WithMaxRetries(4))
	det := ettlemesh.NewDetector(&client, *model)

	prevSelf, err := distillRound(ctx, det, prev)
	if err != nil {
		return err
	}
	// Gate L1 on note-change, not just L2 on atom-change: a note byte-identical to
	// last round has the SAME self-model, so reuse its prior atoms verbatim instead
	// of re-rolling the stochastic distiller (which would reword identical input and
	// manufacture phantom drift). This is the emit-gate discipline applied one layer
	// down — and it's what makes the surprise gate actually bite: only a note that
	// truly changed is re-distilled, so only real deltas can cross.
	currSelf, reused, err := distillCurrent(ctx, det, curr, prev, prevSelf)
	if err != nil {
		return err
	}

	state := ettlemesh.NewMeshState()
	seed := state.Advance(prevSelf)    // round 1: seed every directed model
	deltas := state.Surprise(currSelf) // round 2: the surprise-gated deltas (read-only; doesn't absorb)
	printDrift(*me, state, seed, deltas, currSelf, reused)
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
