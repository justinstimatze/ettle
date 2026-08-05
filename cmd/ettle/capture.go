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
	who := captureIdentity(me, room)

	// Read + digest each transcript; union the notes so one person's several
	// transcripts publish as a single envelope (Publish is replace-current).
	var notes []string
	for _, path := range paths {
		s, err := capture.Read(path)
		if err != nil {
			return fmt.Errorf("capture %s: %w", path, err)
		}
		if s.Empty() {
			continue
		}
		notes = append(notes, s.Digest())
	}
	if len(notes) == 0 {
		fmt.Println("ettle: no L1 signal in the session(s) — nothing to publish.")
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

	n, err := publishCapture(ctx, det, bus, who, note)
	if err != nil {
		return err
	}
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
	if err := bus.Publish(ctx, transport.Envelope{Participant: me, Atoms: atoms}); err != nil {
		return 0, fmt.Errorf("publish: %w", err)
	}
	return len(atoms), nil
}

// captureIdentity resolves who the published atoms belong to: an explicit --me
// wins; otherwise a leat room's configured agent; otherwise $USER. This keeps
// the hook config to just --room in the common case (the room already knows you).
func captureIdentity(me, room string) string {
	if strings.TrimSpace(me) != "" {
		return me
	}
	if room != "" {
		if rc, err := loadRoom(room); err == nil && rc.Agent != "" {
			return rc.Agent
		}
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
	if *room == "" && *transportName == "" {
		return fmt.Errorf("usage: ettle capture-hook --room <room>   (wire it to SessionEnd, and optionally Stop for mid-session freshness)")
	}

	// The hook payload (SessionEnd / Stop) carries transcript_path. Read stdin
	// fully so the pipe never blocks the caller, then pull the path out.
	var payload struct {
		TranscriptPath string `json:"transcript_path"`
	}
	if data, _ := io.ReadAll(os.Stdin); len(data) > 0 {
		_ = json.Unmarshal(data, &payload)
	}
	if strings.TrimSpace(payload.TranscriptPath) == "" {
		return nil // no transcript to distill — never error a hook
	}

	target := *room
	if target == "" {
		target = *transportName
	}
	due, err := dueForCapture(target, *debounce)
	if err != nil {
		return err
	}
	if !due {
		return nil // captured recently — nothing to do
	}

	// Detach a background `ettle capture` so the hook returns instantly. Flags come
	// before the positional transcript so flag parsing sees them (Go's flag package
	// stops at the first non-flag token).
	who := captureIdentity(*me, *room)
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
func dueForCapture(target string, window time.Duration) (bool, error) {
	path, err := captureRunPath(target)
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

// captureRunPath is the per-target debounce marker, under its own capture/ dir so
// it never collides with pull's hookrun marker.
func captureRunPath(target string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	return filepath.Join(dir, "ettle", "capture", transport.SanitizeID(target)+".hookrun"), nil
}
