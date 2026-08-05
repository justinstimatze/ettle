package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/tanglestate"
	"github.com/justinstimatze/ettle/internal/transport"
)

// Horizon injection is the SURFACE half of set-and-forget: the coordination tangles
// relevant to you appear in your session at SessionStart, unprompted — you never
// run a command to see them. Reconcile is a model call, so it can't run inside a
// SessionStart hook without making every session start slow and costly. So the
// split is: `ettle horizon` reconciles and caches a rendered block (run in the
// background, human-paced), and `ettle horizon-hook` injects the CACHED block
// instantly and spawns a detached refresh for next time. Whisper-first: this
// surfaces privately to you; nothing is ever posted (see docs/SURFACES.md).

// runHorizon reconciles the room's published atoms into the tangles relevant to
// `me` and renders them. With --cache it writes the rendered block to the local
// cache (silently, for the hook's detached refresh); otherwise it prints — a
// standalone "what's my horizon right now" that needs no note files, just the
// atoms capture/pull already put on the bus.
func runHorizon(args []string) error {
	fs := flag.NewFlagSet("horizon", flag.ContinueOnError)
	room := fs.String("room", "", "reconcile this leat room's horizon (created by `ettle room init|join`)")
	transportName := fs.String("transport", "", "reconcile this transport's horizon when --room is not used: inproc | file://<path> | leat://<repoDir> | linear://<room> (needs LINEAR_API_KEY) | github://<owner>/<repo>[/<room>] (a PRIVATE repo's Discussions) | nats")
	me := fs.String("me", "", "surface only tangles involving this participant (default: the room's agent, else the identity `ettle init` saved, else $USER)")
	all := fs.Bool("all", false, "surface the WHOLE team's tangles instead of only yours — the unfiltered view. Needed because --me now falls back to your saved identity when unset, so passing it empty can no longer mean \"no filter\"")
	model := fs.String("model", "claude-haiku-4-5", "model id for the reconcile")
	samples := fs.Int("samples", 5, "independent reconcile samples to vote across; recurrence ranks tangles firm vs soft (1 disables voting)")
	cache := fs.Bool("cache", false, "write the rendered horizon to the local cache instead of printing it (used by the hook's background refresh)")
	insecureLocal := fs.Bool("insecure-local", false, "allow a plaintext local NATS connection (development only)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	*room, *transportName = applyRoomFile(*room, *transportName)
	if *room == "" && *transportName == "" {
		return fmt.Errorf("no room: run `ettle init <room>` in this project, or pass --room/--transport   (horizon reconciles the atoms capture/pull put on the bus into the tangles relevant to you)")
	}
	key := apiKey()
	if key == "" {
		return fmt.Errorf("no ANTHROPIC_API_KEY (set it in the environment or a .env file) — the reconcile runs locally")
	}
	who := captureIdentity(*me, *room, *transportName)
	if *all {
		who = "" // the reconcile's me-filter is skipped on an empty identity
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client := anthropic.NewClient(option.WithAPIKey(key), option.WithMaxRetries(4))
	det := ettlemesh.NewDetector(&client, *model)
	det.Ground = true // cross-person coupling check ON, matching standup/mcp defaults
	bus, err := selectBus(runConfig{room: *room, transport: *transportName, insecureLocal: *insecureLocal})
	if err != nil {
		return err
	}
	defer bus.Close()

	res, err := reconcileHorizon(ctx, det, bus, who, *samples)
	if err != nil {
		return err
	}
	// Parity with the MCP horizon. MUTING applies to every bus — a tangle the human
	// said was handled must stop re-surfacing whether the room is Linear, GitHub, or
	// a git repo — so the stores key by the room SPEC. ESCALATION is Linear-only, so
	// `escalated` stays nil elsewhere and the block shows no share tags.
	stateKey := roomStateKey(*room, *transportName)
	muted, _ := tanglestate.Load(tanglestate.Muted, stateKey)
	var escalated map[string]bool
	if _, isLinear := linearRoomOf(*room, *transportName); isLinear {
		escalated, _ = tanglestate.Load(tanglestate.Escalated, stateKey)
	}
	res = tagHorizon(res, muted, escalated)
	block := renderHorizonBlock(res, who, time.Now().UTC())

	if !*cache {
		fmt.Println(block)
		return nil
	}
	target := *room
	if target == "" {
		target = *transportName
	}
	return writeHorizonCache(target, who, block)
}

// horizonResult is the reconcile output the renderer consumes. escalated is nil for
// a non-Linear room (no escalation surface, so no tags); for a Linear room it is the
// set of tangle keys already posted to the coordination issue (possibly empty), which
// tells the reader which tangles a non-adopter can't yet see. muted counts tangles the
// human resolved (suppressed from firm/soft) so a clear-by-muting horizon stays honest.
type horizonResult struct {
	firm, soft, held []ettlemesh.Tangle
	floorHeld        int
	participants     []string
	escalated        map[string]bool
	muted            int
}

// tagHorizon brings the injected block to parity with the MCP ettle_horizon: it
// suppresses muted tangles (the human marked them handled) and records which tangles are
// already escalated, so the block can flag what's un-shared. Pure; runHorizon loads
// the stores. Only called for a Linear room — escalation is Linear-only, so a leat/
// in-proc horizon leaves escalated nil and shows no share tags.
func tagHorizon(res horizonResult, muted, escalated map[string]bool) horizonResult {
	res.escalated = escalated
	res.firm = dropMuted(res.firm, muted, &res.muted)
	res.soft = dropMuted(res.soft, muted, &res.muted)
	return res
}

func dropMuted(ks []ettlemesh.Tangle, muted map[string]bool, count *int) []ettlemesh.Tangle {
	var out []ettlemesh.Tangle
	for _, k := range ks {
		if muted[tanglestate.Key(k.Kind, k.Parties)] {
			*count++
			continue
		}
		out = append(out, k)
	}
	return out
}

// reconcileHorizon Collects the bus, folds to the latest envelope per participant,
// and runs the same reconcile the MCP horizon does (ReconcileVoted → GroundTangles),
// filtered to `me`. The reconcile primitives live in ettlemesh (single-sourced); this
// only orchestrates them, so it can't drift from the engine's behavior.
func reconcileHorizon(ctx context.Context, det *ettlemesh.Detector, bus transport.Transport, me string, samples int) (horizonResult, error) {
	envs, err := bus.Collect(ctx)
	if err != nil {
		return horizonResult{}, fmt.Errorf("collect: %w", err)
	}
	envs = foldLatest(envs)

	var res horizonResult
	for _, e := range envs {
		res.participants = append(res.participants, e.Participant)
	}
	sort.Strings(res.participants)

	atoms := transport.Atoms(envs)
	if len(atoms) == 0 {
		return res, nil // empty bus → no model call
	}
	if samples <= 0 {
		samples = 1
	}
	tangles, floorHeld, err := det.ReconcileVoted(ctx, atoms, samples)
	if err != nil {
		return horizonResult{}, err
	}
	kept, suppressed, err := det.GroundTangles(ctx, tangles, atoms)
	if err != nil {
		return horizonResult{}, err
	}
	return classifyHorizon(kept, suppressed, res.participants, floorHeld, me), nil
}

// classifyHorizon is the pure post-reconcile step: split kept tangles into
// firm/soft, collect the suppressed ones, and filter everything to `me` (your
// agent surfaces only your tangles, never a shared feed). Split out so it is
// unit-tested without a live detector.
func classifyHorizon(kept, suppressed []ettlemesh.Tangle, participants []string, floorHeld int, me string) horizonResult {
	res := horizonResult{participants: participants, floorHeld: floorHeld}
	for _, k := range kept {
		if me != "" && !partiesIncludeMe(k.Parties, me) {
			continue
		}
		if k.Firm() {
			res.firm = append(res.firm, k)
		} else {
			res.soft = append(res.soft, k)
		}
	}
	for _, k := range suppressed {
		if me != "" && !partiesIncludeMe(k.Parties, me) {
			continue
		}
		res.held = append(res.held, k)
	}
	return res
}

// foldLatest keeps one envelope per participant, last-writer-wins. LWW buses
// (file/leat/linear) already return one per author, but the in-process bus is
// append-only, so folding here makes the reconcile correct on any transport.
func foldLatest(envs []transport.Envelope) []transport.Envelope {
	at := map[string]int{}
	out := make([]transport.Envelope, 0, len(envs))
	for _, e := range envs {
		k := strings.ToLower(strings.TrimSpace(e.Participant))
		if i, ok := at[k]; ok {
			out[i] = e
			continue
		}
		at[k] = len(out)
		out = append(out, e)
	}
	return out
}

// linearRoomOf returns the Linear room name (and true) when the horizon's transport
// is a Linear bus, so the escalated/muted stores key the same way `ettle escalate`
// and the MCP server do. A leat --room is not a Linear room, so it yields false.
// roomStateKey names a room for the per-room tangle stores (muted/escalated). It is
// the transport SPEC, so every bus gets its own bucket — keying off the Linear room
// meant a mute on a github:// or leat room landed in a shared "default" bucket and
// non-Linear rooms muted each other's tangles.
func roomStateKey(room, transportName string) string {
	if strings.TrimSpace(transportName) != "" {
		return transportName
	}
	return room
}

func linearRoomOf(_ /*room*/, transportName string) (string, bool) {
	if r, ok := strings.CutPrefix(transportName, "linear://"); ok {
		if r = strings.TrimSpace(r); r != "" {
			return r, true
		}
	}
	return "", false
}

func partiesIncludeMe(parties []string, me string) bool {
	for _, p := range parties {
		if ettlemesh.SamePerson(p, me) {
			return true
		}
	}
	return false
}

// renderHorizonBlock formats the reconcile as a compact markdown block for context
// injection. It is written as an instruction to its actual reader — the agent — not a
// note to a human, because the agent is who receives it at SessionStart and decides
// when to raise it. It carries parity with the MCP ettle_horizon: un-shared tangles are
// flagged so the agent knows what to offer escalating, muted tangles are gone.
func renderHorizonBlock(res horizonResult, me string, now time.Time) string {
	var b strings.Builder
	who := me
	if who == "" {
		who = "the team"
	}
	fmt.Fprintf(&b, "# ettle horizon for %s — coordination tangles (as of %s)\n\n", who, now.Format("2006-01-02 15:04 MST"))
	if len(res.firm) == 0 && len(res.soft) == 0 {
		fmt.Fprintf(&b, "Horizon clear — no cross-person coordination tangles involving %s right now", who)
		if n := len(res.participants); n > 0 {
			fmt.Fprintf(&b, " (%s on the bus)", plural(n, "participant", "participants"))
		}
		b.WriteString(".")
		if res.muted > 0 {
			fmt.Fprintf(&b, " (%s muted as handled.)", plural(res.muted, "tangle", "tangles"))
		}
		b.WriteString("\n")
		return strings.TrimSpace(b.String())
	}
	// Address the agent directly: this is context to act on, not a message to post.
	fmt.Fprintf(&b, "You are %s's ettle agent. These are cross-person coordination tangles involving %s, surfaced privately for you — nothing is posted anywhere. When %s's current work touches one, raise it with them; don't dump the list unprompted. Firm = worth a look; soft = worth a question with the other person.\n", who, who, who)
	if res.escalated != nil {
		b.WriteString("A tangle flagged `not yet shared` is one the other person can't see; if " + who + " would want them to know, offer to escalate it (the `ettle_escalate` tool, or `ettle escalate`).\n")
	}
	if len(res.firm) > 0 {
		b.WriteString("\n**Firm (worth a look):**\n")
		for _, k := range res.firm {
			b.WriteString(horizonLine(k, shareTag(res, k)))
		}
	}
	if len(res.soft) > 0 {
		b.WriteString("\n**Soft (worth a question):**\n")
		for _, k := range res.soft {
			b.WriteString(horizonLine(k, shareTag(res, k)))
		}
	}
	if res.muted > 0 {
		fmt.Fprintf(&b, "\n_(%s muted as handled — not shown.)_\n", plural(res.muted, "tangle", "tangles"))
	}
	if n := len(res.held) + res.floorHeld; n > 0 {
		fmt.Fprintf(&b, "\n_(%s held back as likely-not-a-conflict.)_\n", plural(n, "candidate", "candidates"))
	}
	return strings.TrimSpace(b.String())
}

// shareTag flags whether a tangle has been escalated to a non-adopter — only for a
// Linear room (escalated != nil) and only for cross-person tangles (a self-tangle has
// nobody to share with). This is what lets the agent offer to escalate exactly what's
// still invisible to the other person.
func shareTag(res horizonResult, k ettlemesh.Tangle) string {
	if res.escalated == nil || !ettlemesh.MultiPerson(k.Parties) {
		return ""
	}
	if res.escalated[tanglestate.Key(k.Kind, k.Parties)] {
		return " · shared with the team"
	}
	return " · not yet shared"
}

func horizonLine(k ettlemesh.Tangle, tag string) string {
	parties := strings.Join(k.Parties, ", ")
	about := strings.TrimSpace(k.About)
	expl := strings.TrimSpace(k.Explanation)
	switch {
	case about != "" && expl != "":
		return fmt.Sprintf("- **%s** · %s · %s — %s%s\n", k.Kind, parties, about, expl, tag)
	case expl != "":
		return fmt.Sprintf("- **%s** · %s — %s%s\n", k.Kind, parties, expl, tag)
	default:
		return fmt.Sprintf("- **%s** · %s · %s%s\n", k.Kind, parties, about, tag)
	}
}

// --- cache + hook ---------------------------------------------------------

// horizonCachePath is the per-(target, me) rendered-horizon cache the hook injects.
func horizonCachePath(target, me string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	name := transport.SanitizeID(target) + "." + transport.SanitizeID(me) + ".md"
	return filepath.Join(dir, "ettle", "horizon", name), nil
}

func writeHorizonCache(target, me, block string) error {
	path, err := horizonCachePath(target, me)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(block), 0o644)
}

func readHorizonCache(target, me string) (string, error) {
	path, err := horizonCachePath(target, me)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // cold cache: nothing to inject yet
		}
		return "", err
	}
	return string(data), nil
}

// sessionStartOutput is the Claude Code SessionStart hook JSON: additionalContext
// is injected into the session's context at start.
type sessionStartOutput struct {
	HookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

// runHorizonHook is `ettle horizon-hook --room <room>` — the SessionStart injector.
// It injects the CACHED horizon instantly (no model call, never blocks) and spawns a
// detached `ettle horizon --cache` to refresh it for the next session, so the cache
// self-warms from normal use. Cold start (no cache yet) injects nothing and just
// warms the cache — the next session shows the horizon.
func runHorizonHook(args []string) error {
	fs := flag.NewFlagSet("horizon-hook", flag.ContinueOnError)
	room := fs.String("room", "", "the room whose horizon to inject")
	transportName := fs.String("transport", "", "transport whose horizon to inject when --room is not used")
	me := fs.String("me", "", "your identity (default: the room's agent, else $USER)")
	debounce := fs.Duration("debounce", 5*time.Minute, "skip the background refresh if one ran within this window")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// Drain the hook payload so the pipe never blocks the caller — before any early
	// return, or a no-op in a non-ettle project would leave the writer blocked.
	_, _ = io.Copy(io.Discard, os.Stdin)

	// Wired globally, this fires in every session of every project. A project with no
	// room is not an error, it is simply not an ettle project: say nothing and exit 0.
	*room, *transportName = applyRoomFile(*room, *transportName)
	if *room == "" && *transportName == "" {
		return nil
	}

	who := captureIdentity(*me, *room, *transportName)
	target := *room
	if target == "" {
		target = *transportName
	}

	// Inject the cached horizon (instant). Absent/empty cache → inject nothing.
	if block, err := readHorizonCache(target, who); err == nil && strings.TrimSpace(block) != "" {
		var out sessionStartOutput
		out.HookSpecificOutput.HookEventName = "SessionStart"
		out.HookSpecificOutput.AdditionalContext = block
		if data, mErr := json.Marshal(out); mErr == nil {
			fmt.Println(string(data))
		}
	}

	// Refresh for next time, detached — debounced so session churn doesn't spam the
	// reconcile. A refresh failure is invisible here; the next session just injects
	// the last good cache.
	if due, err := dueForHorizon(target, who, *debounce); err != nil || !due {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cargs := []string{"horizon", "--cache", "--me", who}
	if *room != "" {
		cargs = append(cargs, "--room", *room)
	}
	if *transportName != "" {
		cargs = append(cargs, "--transport", *transportName)
	}
	cmd := exec.Command(exe, cargs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // own process group: survives the hook exiting
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// dueForHorizon debounces the background refresh, keyed per (target, me), under its
// own marker dir so it never collides with pull's or capture's.
func dueForHorizon(target, me string, window time.Duration) (bool, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return false, fmt.Errorf("locate config dir: %w", err)
	}
	path := filepath.Join(dir, "ettle", "horizon", transport.SanitizeID(target)+"."+transport.SanitizeID(me)+".refresh")
	return markerDue(path, window)
}

// markerDue reports whether `window` has elapsed since the timestamp in `path`,
// writing "now" when it returns true. The shared core of the hook debouncers.
func markerDue(path string, window time.Duration) (bool, error) {
	if data, rerr := os.ReadFile(path); rerr == nil {
		if ts, perr := time.Parse(time.RFC3339, strings.TrimSpace(string(data))); perr == nil {
			if time.Since(ts) < window {
				return false, nil
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339)), 0o644); err != nil {
		return false, err
	}
	return true, nil
}
