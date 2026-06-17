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
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

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
		}
	}
	if len(os.Args) < 2 || os.Args[1] != "standup" {
		fmt.Fprintln(os.Stderr, "usage: ettle standup [flags] <input>...")
		fmt.Fprintln(os.Stderr, "  each input is one participant: a note file, or a Claude Code")
		fmt.Fprintln(os.Stderr, "  session transcript (.jsonl) — the live-reasoning L1 source.")
		fmt.Fprintln(os.Stderr, "  ettle capture <transcript.jsonl>   # preview what a session distills to")
		fmt.Fprintln(os.Stderr, "  cost: ~2N+3 model calls for N participants (+2 per extra --samples)")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("standup", flag.ExitOnError)
	me := fs.String("me", "", "surface knots relevant to this participant (their agent's view); empty = full team view")
	model := fs.String("model", "claude-haiku-4-5", "model id")
	gemotURL := fs.String("gemot", "", "gemot MCP endpoint for contested knots (e.g. https://gemot.example/mcp); empty = inline either/or")
	transportName := fs.String("transport", "inproc", "atom transport: inproc | nats (nats needs -tags nats)")
	insecureLocal := fs.Bool("insecure-local", false, "dev only: allow plaintext/tokenless connections to localhost gemot + NATS (e.g. local docker)")
	gemotTimeout := fs.Duration("gemot-timeout", 180*time.Second, "how long to wait for a gemot deliberation's analysis")
	samples := fs.Int("samples", 1, "run the reconcile passes N times and keep only knots that recur across a majority (stabilizes the stochastic detector; costs N× the reconcile calls)")
	_ = fs.Parse(os.Args[2:])

	cfg := runConfig{me: *me, model: *model, gemotURL: *gemotURL, transport: *transportName, insecureLocal: *insecureLocal, gemotTimeout: *gemotTimeout, samples: *samples, paths: fs.Args()}
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "ettle:", err)
		os.Exit(1)
	}
}

type participant struct {
	Name, Role, Notes string
}

type runConfig struct {
	me, model, gemotURL, transport string
	insecureLocal                  bool
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

	client := anthropic.NewClient(option.WithAPIKey(key))
	det := ettlemesh.NewDetector(&client, cfg.model)

	bus, err := busFor(cfg.transport, cfg.insecureLocal)
	if err != nil {
		return err
	}
	defer bus.Close()

	// 1+2: distill each person and publish their atoms over the seam.
	var allQuestions []string
	for _, p := range people {
		atoms, err := det.Distill(ctx, p.Name, p.Role, p.Notes)
		if err != nil {
			return fmt.Errorf("distill %s: %w", p.Name, err)
		}
		inferred, qs, err := det.InferImplicit(ctx, p.Name, p.Role, p.Notes)
		if err != nil {
			return fmt.Errorf("infer %s: %w", p.Name, err)
		}
		atoms = append(atoms, inferred...)
		allQuestions = append(allQuestions, qs...)
		if err := bus.Publish(ctx, transport.Envelope{Participant: p.Name, Role: p.Role, Atoms: atoms}); err != nil {
			return fmt.Errorf("publish %s: %w", p.Name, err)
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
func surface(ctx context.Context, me string, knots []ettlemesh.Knot, atoms []ettlemesh.Atom, questions []string, resolver crux.Resolver) {
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
		section("worth a question (soft — rests on an inference)")
		for _, k := range soft {
			printKnot(k)
		}
	}

	// Questions the inference step couldn't answer confidently — surfaced only
	// to the person they're about (friction in the right spot).
	var mine []string
	for _, q := range questions {
		if me == "" || strings.Contains(strings.ToLower(q), "["+strings.ToLower(me)+"]") {
			mine = append(mine, q)
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

// detectFor runs the detector over participants and returns FIRM + soft knots.
// samples>1 routes the cross-person passes through majority voting. Self-knots
// are deduped against the cross-person set. (No transport — eval reconciles
// directly, the bus is only for the distributed standup.)
func detectFor(ctx context.Context, det *ettlemesh.Detector, people []participant, samples int) (firm, soft []ettlemesh.Knot, err error) {
	var atoms []ettlemesh.Atom
	for _, p := range people {
		a, err := det.Distill(ctx, p.Name, p.Role, p.Notes)
		if err != nil {
			return nil, nil, fmt.Errorf("distill %s: %w", p.Name, err)
		}
		inf, _, err := det.InferImplicit(ctx, p.Name, p.Role, p.Notes)
		if err != nil {
			return nil, nil, fmt.Errorf("infer %s: %w", p.Name, err)
		}
		atoms = append(atoms, append(a, inf...)...)
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
	_ = fs.Parse(args)
	if len(fs.Args()) == 0 {
		return fmt.Errorf("usage: ettle eval [--ab] [--samples K] <corpus.json>...")
	}
	key := apiKey()
	if key == "" {
		return fmt.Errorf("no ANTHROPIC_API_KEY (set it in the environment or a .env file)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	client := anthropic.NewClient(option.WithAPIKey(key))
	det := ettlemesh.NewDetector(&client, *model)

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
			return err
		}
		s := eval.Adjudicate(firm, soft, c.Expected)
		printScore("single-shot", s)

		if *ab {
			fv, sv, err := detectFor(ctx, det, people, *samples)
			if err != nil {
				return err
			}
			sV := eval.Adjudicate(fv, sv, c.Expected)
			printScore(fmt.Sprintf("voted (%d samples)", *samples), sV)
			// Paired McNemar over the real labels' recovery under each condition.
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
			p := eval.McNemarTwoTailed(b, cc)
			fmt.Printf("    A/B (recall): discordant single-only=%d voted-only=%d → McNemar p=%.3f %s\n",
				b, cc, p, abVerdict(p, b, cc))
			fmt.Printf("    A/B (precision): single FP=%d, voted FP=%d (descriptive — not a paired test)\n", s.FP, sV.FP)
		}
	}
	return nil
}

func printScore(label string, s eval.Score) {
	fmt.Printf("    %-22s precision %d/%d=%.2f · recall %d/%d=%.2f · would-ask %d",
		label, s.TP, s.TP+s.FP, s.Precision(), s.RecallHits, s.RecallTotal, s.Recall(), s.WouldAsk)
	if len(s.Missed) > 0 {
		fmt.Printf(" · missed %s", strings.Join(s.Missed, ","))
	}
	fmt.Println()
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
// lines set identity, otherwise the filename (sans extension) is the name, and
// everything after the header is the private note.
func loadParticipants(paths []string) ([]participant, error) {
	var out []participant
	for _, path := range paths {
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
					name = strings.TrimSpace(ln[strings.Index(ln, ":")+1:])
					continue
				case strings.HasPrefix(low, "role:"):
					role = strings.TrimSpace(ln[strings.Index(ln, ":")+1:])
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
		out = append(out, participant{Name: name, Role: role, Notes: strings.TrimSpace(strings.Join(body, "\n"))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
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
