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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/justinstimatze/ettle/internal/capture"
	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/transport"
)

// Auto-capture is the SEND half of set-and-forget: a session's own reasoning
// reaches the bus without anyone running an emit tool by hand. `ettle capture
// --room <room>` distills a Claude Code transcript LOCALLY (raw prose never
// crosses — only the typed atoms) and publishes it as your own atoms; the
// capture-hook fires it from a Stop / SessionEnd hook so it just happens. This
// pairs with pull.go (the receive half): pull brings teammates in, capture puts
// you on the bus. See docs/SURFACES.md.

// capturePublish reads the given transcript(s), distills them locally, and
// publishes the atoms as `me` to the selected bus. Called by `ettle capture`
// when a --room/--transport target is set (the capture-hook's detached child).
func capturePublish(paths []string, room, transportName, me, model string, insecureLocal bool) error {
	key := apiKey()
	if key == "" {
		return fmt.Errorf("no ANTHROPIC_API_KEY (set it in the environment or a .env file) — capture distills the session locally")
	}
	who := captureIdentity(me, room, transportName)

	// Read + digest each transcript; union the notes so one person's several
	// transcripts publish as a single envelope (Publish is replace-current).
	var notes []string
	consumed := map[string]int64{}
	incremental := false
	for _, path := range paths {
		from := captureOffset(room, transportName, path)
		s, total, err := capture.ReadFrom(path, from)
		if err != nil {
			return fmt.Errorf("capture %s: %w", path, err)
		}
		consumed[path] = total
		if from > 0 {
			incremental = true
		}
		if s.Empty() {
			continue
		}
		notes = append(notes, s.Digest())
	}
	if len(notes) == 0 {
		// Nothing new since last time is the NORMAL case on a quiet stretch, and those
		// lines are still consumed — re-reading them next time would cost a distill for
		// signal already known to be absent.
		saveCaptureOffsets(room, transportName, consumed)
		fmt.Println("ettle: no new L1 signal in the session(s) — nothing to publish.")
		return nil
	}
	note := strings.Join(notes, "\n\n")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client := anthropic.NewClient(option.WithAPIKey(key), option.WithMaxRetries(4))
	det := ettlemesh.NewDetector(&client, model)
	bus, err := selectBus(runConfig{room: room, transport: transportName, insecureLocal: insecureLocal})
	if err != nil {
		return err
	}
	defer bus.Close()

	n, err := publishCaptureMerging(ctx, det, bus, who, note, incremental)
	if err != nil {
		return err
	}
	// Only after a successful publish: a failed run must re-read the same range rather
	// than dropping those turns on the floor.
	saveCaptureOffsets(room, transportName, consumed)
	if n == 0 {
		fmt.Println("ettle: session distilled to no atoms — nothing published.")
		return nil
	}
	fmt.Printf("ettle: captured %s as %q.\n", plural(n, "atom", "atoms"), who)
	return nil
}

// publishCapture distills a session note and publishes it as `me`'s atoms. Split
// out so tests exercise the distill+publish contract key-free (fake distiller +
// fake bus), the same shape as pullReplies. Reuses pull.go's `distiller`.
//
// A zero-atom distill publishes NOTHING: an empty envelope would replace the
// person's current atoms with none, silently erasing them from the bus.
func publishCapture(ctx context.Context, det distiller, bus transport.Transport, me, note string) (int, error) {
	return publishCaptureMerging(ctx, det, bus, me, note, false)
}

// publishCaptureMerging distills `note` and publishes the result. When `merge` is set
// the note covers only the turns since the last capture, so what is already on the bus
// for `me` is folded in first — Publish is replace-current, and publishing a slice of
// the last few minutes on its own would erase the morning.
//
// A read failure is not fatal but is NOT silently treated as "nothing there": that
// would replace a full self-model with a fragment. It aborts instead, leaving the bus
// as it was and the offset unmoved, so the next capture retries over the same range.
func publishCaptureMerging(ctx context.Context, det distiller, bus transport.Transport, me, note string, merge bool) (int, error) {
	if strings.TrimSpace(note) == "" {
		return 0, nil
	}
	atoms, err := det.Distill(ctx, me, "", note, nil)
	if err != nil {
		return 0, fmt.Errorf("distill session: %w", err)
	}
	if len(atoms) == 0 {
		return 0, nil
	}
	if merge {
		prev, rerr := ownAtoms(ctx, bus, me)
		if rerr != nil {
			return 0, fmt.Errorf("read own atoms before merging an incremental capture: %w", rerr)
		}
		atoms = ettlemesh.MergeSelf(prev, atoms)
	}
	if err := bus.Publish(ctx, transport.Envelope{Participant: me, Atoms: atoms}); err != nil {
		return 0, fmt.Errorf("publish: %w", err)
	}
	return len(atoms), nil
}

// ownAtoms reads back what this participant last published. Identity comes from the
// transport's own slug rule, so it matches however the bus stored it.
func ownAtoms(ctx context.Context, bus transport.Transport, me string) ([]ettlemesh.Atom, error) {
	envs, err := bus.Collect(ctx)
	if err != nil {
		return nil, err
	}
	for _, e := range envs {
		if transport.SamePartic(e.Participant, me) {
			return e.Atoms, nil
		}
	}
	return nil, nil // never published here before — a first capture, not an error
}

// captureIdentity resolves who the published atoms belong to: an explicit --me
// wins; otherwise a leat room's configured agent; otherwise $USER. This keeps
// the hook config to just --room in the common case (the room already knows you).
func captureIdentity(me, room, transportName string) string {
	if strings.TrimSpace(me) != "" {
		return me
	}
	if room != "" {
		if rc, err := loadRoom(room); err == nil && rc.Agent != "" {
			return rc.Agent
		}
	}
	// A Linear room stores no agent, so identity comes from what `ettle init` saved
	// for this room on this machine — the path most people are on.
	spec := transportName
	if spec == "" {
		spec = room
	}
	if who := loadIdentity(spec); who != "" {
		return who
	}
	return defaultAgent()
}

// runCaptureHook is `ettle capture-hook --room <room>` — the trigger a Claude
// Code Stop / SessionEnd hook fires so a session's atoms reach the bus without
// anyone running `ettle capture`. It reads the transcript path off the hook
// payload, debounces (so Stop firing every turn collapses to one distill), then
// spawns `ettle capture` DETACHED and returns immediately — the hook must never
// block the agent waiting on a model call.
func runCaptureHook(args []string) error {
	fs := flag.NewFlagSet("capture-hook", flag.ContinueOnError)
	room := fs.String("room", "", "the room to publish this session's atoms into")
	transportName := fs.String("transport", "", "transport to publish to when --room is not used")
	me := fs.String("me", "", "your identity for the published atoms (default: the room's agent, else $USER)")
	debounce := fs.Duration("debounce", 2*time.Minute, "skip if a capture already ran within this window (matters when wired to Stop, which fires each turn)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// The hook payload (SessionEnd / Stop) carries transcript_path. Read stdin fully
	// so the pipe never blocks the caller — before any early return — then pull the
	// path out.
	var payload struct {
		TranscriptPath string `json:"transcript_path"`
	}
	if data, _ := io.ReadAll(os.Stdin); len(data) > 0 {
		_ = json.Unmarshal(data, &payload)
	}

	// Wired globally, this fires in every session of every project. A project with no
	// room is not an error, it is simply not an ettle project: say nothing and exit 0.
	*room, *transportName = applyRoomFile(*room, *transportName)
	if *room == "" && *transportName == "" {
		return nil
	}
	if strings.TrimSpace(payload.TranscriptPath) == "" {
		return nil // no transcript to distill — never error a hook
	}

	target := *room
	if target == "" {
		target = *transportName
	}
	due, err := dueForCapture(target, payload.TranscriptPath, *debounce)
	if err != nil {
		return err
	}
	if !due {
		return nil // captured recently — nothing to do
	}

	// Detach a background `ettle capture` so the hook returns instantly. Flags come
	// before the positional transcript so flag parsing sees them (Go's flag package
	// stops at the first non-flag token).
	who := captureIdentity(*me, *room, *transportName)
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cargs := []string{"capture", "--me", who}
	if *room != "" {
		cargs = append(cargs, "--room", *room)
	}
	if *transportName != "" {
		cargs = append(cargs, "--transport", *transportName)
	}
	cargs = append(cargs, payload.TranscriptPath)
	cmd := exec.Command(exe, cargs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // own process group: survives the hook exiting
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// dueForCapture reports whether enough time has passed since the last
// hook-triggered capture for this target, recording "now" when it returns true.
// A separate marker from pull's (dueForPull) so the two debounce independently.
func dueForCapture(target, transcript string, window time.Duration) (bool, error) {
	path, err := captureRunPath(target, transcript)
	if err != nil {
		return false, err
	}
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

// captureRunPath is the debounce marker, keyed per (room, SESSION).
//
// Keying on the room alone was wrong for anyone running several sessions at once,
// which is the normal case here: five sessions in five worktrees all resolve the same
// room, so they shared one marker and four of them had their capture silently
// skipped — different work, different atoms, dropped because a sibling captured
// ninety seconds earlier. No error, just a thin bus.
//
// The transcript path is the session identity the hook payload already carries, and
// its basename is the session id, so it is both unique and short. Within one session
// Stop still collapses per-turn firing exactly as before; across sessions each one
// now captures on its own clock.
func captureRunPath(target, transcript string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	name := transport.SanitizeID(target)
	if sess := strings.TrimSuffix(filepath.Base(strings.TrimSpace(transcript)), ".jsonl"); sess != "" && sess != "." {
		name += "." + transport.SanitizeID(sess)
	}
	return filepath.Join(dir, "ettle", "capture", name+".hookrun"), nil
}

// The per-(room, transcript) read offset: how many transcript lines a previous capture
// already distilled.
//
// It is what makes a long session affordable. Without it every capture re-digests the
// whole transcript, so on a session running for hours the cost of each two-minute
// capture climbs with the session's length — and the fix for the debounce, keying it
// per session so parallel sessions stop suppressing each other, multiplied that by
// however many sessions are open.
//
// Keyed the same way the debounce marker is, and stored beside it.
func captureOffsetPath(room, transportName, transcript string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	target := room
	if target == "" {
		target = transportName
	}
	name := transport.SanitizeID(target) + "." +
		transport.SanitizeID(strings.TrimSuffix(filepath.Base(transcript), ".jsonl"))
	return filepath.Join(dir, "ettle", "capture", name+".offset"), nil
}

// captureOffset returns where the last capture stopped, or 0 to read the whole
// transcript. Any problem reading it means 0: re-distilling costs money, but skipping
// turns loses work, and only one of those is recoverable.
func captureOffset(room, transportName, transcript string) int64 {
	path, err := captureOffsetPath(room, transportName, transcript)
	if err != nil {
		return 0
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// saveCaptureOffsets records how far each transcript was read. Best-effort: failing to
// write it costs a re-distill next time, which is not worth failing a capture over.
func saveCaptureOffsets(room, transportName string, consumed map[string]int64) {
	for transcript, n := range consumed {
		path, err := captureOffsetPath(room, transportName, transcript)
		if err != nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			continue
		}
		_ = os.WriteFile(path, []byte(strconv.FormatInt(n, 10)), 0o644)
	}
}

// SeedCaptureOffsets marks every transcript that already exists for `dir` and the tree
// beneath it as already-distilled, so the first capture after `ettle init` covers only
// what happens NEXT.
//
// This is a consent boundary before it is a cost one. `ettle init` is the opt-in act,
// and ADOPTION.md is explicit that state enters the shared layer only from a person's
// own act — so distilling hours of work that predate the install, and publishing it to
// a room the person joined thirty seconds ago, is the wrong default no matter how
// cheap it is. It happens to also avoid an expensive first run on a long session.
//
// Seeding at install rather than on first sight is deliberate. "Skip a transcript we
// have not seen before" would also skip a genuinely NEW session's first capture, which
// on a short session loses it entirely. Install is the one moment where "everything
// before now is pre-consent" is exactly true and needs no heuristic.
//
// Scope is deliberately narrow. Capture only ever sees a transcript a HOOK hands it,
// and hooks fire for the session that is running — so a transcript from a session that
// ended last month never reaches capture and seeding it protects against nothing. Only
// sessions that could still be open matter, which is the handful written recently, not
// the thousands in a year of history. Scanning them all would mean a full parse of
// every transcript on the machine to compute a line count, and an offset file per
// transcript that nothing would ever read again.
//
// The limit that leaves, stated rather than hidden: a session idle longer than the
// window and then resumed is captured from its start — one backfill, once, for a
// session left sitting. That beats parsing an entire history.
//
// Returns how many were marked, so the report can say so rather than doing it silently.
func SeedCaptureOffsets(dir, target string) int {
	if strings.TrimSpace(target) == "" {
		return 0
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return 0
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return 0
	}
	root := filepath.Join(home, ".claude", "projects")
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	// Claude Code names a project's transcript directory by replacing every "/" and
	// "." in the absolute path with "-". The encoding is lossy, so match by prefix
	// rather than trying to decode: a sibling directory whose name happens to encode
	// the same way would be included, which costs one skipped first capture there and
	// nothing else.
	want := claudeProjectKey(abs)
	consumed := map[string]int64{}
	for _, e := range entries {
		if !e.IsDir() || (e.Name() != want && !strings.HasPrefix(e.Name(), want+"-")) {
			continue
		}
		paths, _ := filepath.Glob(filepath.Join(root, e.Name(), "*.jsonl"))
		for _, p := range paths {
			fi, err := os.Stat(p)
			if err != nil || time.Since(fi.ModTime()) > seedLiveWindow {
				continue // a session that can no longer be live never reaches capture
			}
			consumed[p] = fi.Size() // a stat, not a scan: the offset IS the size
		}
	}
	if len(consumed) == 0 {
		return 0
	}
	saveCaptureOffsets(target, "", consumed)
	return len(consumed)
}

// seedLiveWindow is how recently a transcript must have been written to count as a
// session that could still be open. Generous, because the cost of including a dead one
// is a stray offset file and the cost of missing a live one is a backfill.
const seedLiveWindow = 2 * time.Hour

// claudeProjectKey encodes an absolute path the way Claude Code names its transcript
// directories.
func claudeProjectKey(abs string) string {
	return strings.NewReplacer("/", "-", ".", "-").Replace(abs)
}
