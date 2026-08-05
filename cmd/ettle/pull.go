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

	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/transport"
)

// `ettle pull` is the RECEIVE half of the Linear agent path: a teammate who never
// installs ettle replies in Linear's native agent UI, and pull turns that prose
// into their contribution to the tangle. It reads the human replies off Linear's
// agent activities (a member key, no OAuth app token — see internal/transport/
// linearagent.go), distills each LOCALLY under that teammate's identity, and
// publishes the atoms to the room bus. Raw prose never touches the bus.
//
// This is also invoked automatically before Collect on the read path, so nobody
// has to remember to run it (see pullBeforeCollect); the command is for explicit
// or one-off runs.

// pullCursorPath is where the per-room "newest activity I have seen" timestamp
// lives, following room.go's config-dir convention (<config>/ettle/...).
func pullCursorPath(room string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	return filepath.Join(dir, "ettle", "pull", transport.SanitizeID(room)+".json"), nil
}

func loadPullCursor(room string) (string, error) {
	path, err := pullCursorPath(room)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // first run: no cursor, read everything
		}
		return "", err
	}
	var c struct {
		Cursor string `json:"cursor"`
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return "", fmt.Errorf("pull cursor %s: corrupt: %w", path, err)
	}
	return c.Cursor, nil
}

func savePullCursor(room, cursor string) error {
	path, err := pullCursorPath(room)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, _ := json.Marshal(struct {
		Cursor string `json:"cursor"`
	}{cursor})
	return os.WriteFile(path, data, 0o644)
}

// distiller is the slice of Detector pull needs — factored so tests inject a fake
// and run key-free. *ettlemesh.Detector satisfies it.
type distiller interface {
	Distill(ctx context.Context, from, role, text string, private []string) ([]ettlemesh.Atom, error)
}

// replyPuller is the slice of LinearAgentReader pull needs — factored so tests
// inject canned replies. *transport.LinearAgentReader satisfies it.
type replyPuller interface {
	Pull(ctx context.Context, since string) (replies []transport.Reply, cursor string, more bool, err error)
}

// pullReplies is the reusable step both the command and the auto-hook call. It
// pulls new human replies, distills each under its author's identity, and
// publishes one envelope per author to the bus. Returns how many replies were
// ingested and how many distinct teammates were published.
//
// Idempotent by construction: the doc bus is replace-current per participant, and
// the cursor is saved ONLY after every publish succeeds — so a mid-run failure
// re-pulls and re-publishes the same envelopes rather than losing or duplicating.
// Multiple replies from the same teammate in one pull are unioned into one
// envelope (Publish would otherwise overwrite). Across pulls that envelope is
// replaced (a teammate's doc reflects their most recent replies, matching how a
// local participant's envelope is their current notes — not lifetime history).
func pullReplies(ctx context.Context, det distiller, reader replyPuller, bus transport.Transport, room string) (replies, authors int, err error) {
	since, err := loadPullCursor(room)
	if err != nil {
		return 0, 0, err
	}
	got, cursor, more, err := reader.Pull(ctx, since)
	if err != nil {
		return 0, 0, err
	}

	// Distill each reply locally, unioning atoms per author so one Publish per
	// author doesn't overwrite an earlier reply in the same batch.
	byAuthor := map[string][]ettlemesh.Atom{}
	var order []string
	for _, rep := range got {
		atoms, derr := det.Distill(ctx, rep.Author, "", rep.Body, nil)
		if derr != nil {
			// Don't advance the cursor: return so the next pull retries this reply
			// rather than skipping past it silently.
			return replies, authors, fmt.Errorf("distill reply from %s: %w", rep.Author, derr)
		}
		if len(atoms) == 0 {
			continue // Distill already warns on an empty distill
		}
		if _, seen := byAuthor[rep.Author]; !seen {
			order = append(order, rep.Author)
		}
		byAuthor[rep.Author] = append(byAuthor[rep.Author], atoms...)
		replies++
	}

	for _, author := range order {
		if perr := bus.Publish(ctx, transport.Envelope{Participant: author, Atoms: byAuthor[author]}); perr != nil {
			return replies, authors, fmt.Errorf("publish %s: %w", author, perr)
		}
		authors++
	}

	// Advance the cursor only after every publish landed.
	if cursor != since {
		if serr := savePullCursor(room, cursor); serr != nil {
			return replies, authors, fmt.Errorf("save pull cursor: %w", serr)
		}
	}
	if more {
		fmt.Fprintf(os.Stderr, "ettle: pull hit the %d-activity page cap — some replies may remain; re-run to continue.\n", agentPullPageForMsg)
	}
	return replies, authors, nil
}

// agentPullPageForMsg mirrors the transport's page cap for the user-facing notice
// (the const itself is unexported to the transport package).
const agentPullPageForMsg = 250

// maybePullBeforeCollect auto-ingests teammates' Linear replies right before a
// standup Collect, so a linear:// run reconciles over fresh replies without anyone
// remembering to run `ettle pull`. No-op unless the transport is a Linear room and
// LINEAR_API_KEY is set; non-fatal — a pull failure warns and the standup goes on.
// (MCP sessions get the same freshness via the SessionStart / Linear-tool-use hook
// rather than in-server plumbing.)
func maybePullBeforeCollect(ctx context.Context, det distiller, bus transport.Transport, transportName string) {
	room, ok := strings.CutPrefix(transportName, "linear://")
	if !ok || strings.TrimSpace(room) == "" {
		return // only the Linear doc-bus has agent replies to pull
	}
	lkey := strings.TrimSpace(os.Getenv("LINEAR_API_KEY"))
	if lkey == "" {
		return
	}
	reader := transport.NewLinearAgentReader(lkey, buildVersion())
	n, who, err := pullReplies(ctx, det, reader, bus, room)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ettle: auto-pull skipped (non-fatal): %v\n", err)
		return
	}
	if n > 0 {
		fmt.Fprintf(os.Stderr, "ettle: pulled %s from %s before reconciling.\n",
			plural(n, "reply", "replies"), plural(who, "teammate", "teammates"))
	}
}

// runPullHook is `ettle pull-hook --room <room>` — the trigger a Claude Code
// SessionStart / Linear-tool-use hook fires so pull runs recurringly without
// anyone remembering it. It drains the hook payload, debounces (so a burst of
// Linear tool calls collapses to one pull), then spawns `ettle pull` DETACHED and
// returns immediately — the hook must never block the agent waiting on a distill.
func runPullHook(args []string) error {
	fs := flag.NewFlagSet("pull-hook", flag.ContinueOnError)
	room := fs.String("room", "", "the Linear room to pull teammate replies into")
	debounce := fs.Duration("debounce", 30*time.Second, "skip if a pull already ran within this window")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *room == "" {
		return fmt.Errorf("usage: ettle pull-hook --room <room>   (wire it to SessionStart + a PostToolUse matcher on the Linear MCP tools)")
	}
	// Claude Code writes the hook payload to stdin; we don't need it, but drain it
	// so the pipe never blocks the caller.
	_, _ = io.Copy(io.Discard, os.Stdin)

	due, err := dueForPull(*room, *debounce)
	if err != nil {
		return err
	}
	if !due {
		return nil // pulled recently — nothing to do
	}

	// Detach a background `ettle pull` so the hook returns instantly. Its output is
	// discarded here; any error surfaces on the next explicit `ettle pull`.
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "pull", "--room", *room)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // own process group: survives the hook exiting
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// dueForPull reports whether enough time has passed since the last hook-triggered
// pull for this room, recording "now" when it returns true — so a burst of tool
// calls collapses to a single pull and firing the hook on every Linear tool use
// stays cheap.
func dueForPull(room string, window time.Duration) (bool, error) {
	path, err := hookRunPath(room)
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

// hookRunPath is the per-room debounce marker, beside the cursor file.
func hookRunPath(room string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	return filepath.Join(dir, "ettle", "pull", transport.SanitizeID(room)+".hookrun"), nil
}

// runPull is the standalone `ettle pull --room <room>` command.
func runPull(args []string) error {
	fs := flag.NewFlagSet("pull", flag.ContinueOnError)
	room := fs.String("room", "", "the Linear room to pull teammate replies into (maps to project ettle-<room>)")
	model := fs.String("model", "claude-haiku-4-5", "model id for distilling replies")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *room == "" {
		return fmt.Errorf("usage: ettle pull --room <room>   (reads teammates' Linear agent replies into the room; needs LINEAR_API_KEY + ANTHROPIC_API_KEY)")
	}
	key := apiKey()
	if key == "" {
		return fmt.Errorf("no ANTHROPIC_API_KEY (set it in the environment or a .env file) — pull distills replies locally")
	}
	lkey := strings.TrimSpace(os.Getenv("LINEAR_API_KEY"))
	if lkey == "" {
		return fmt.Errorf("ettle pull needs LINEAR_API_KEY (a Linear member key) to read agent replies")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client := anthropic.NewClient(option.WithAPIKey(key), option.WithMaxRetries(4))
	det := ettlemesh.NewDetector(&client, *model)
	reader := transport.NewLinearAgentReader(lkey, buildVersion())
	bus, err := linearBusFor(*room)
	if err != nil {
		return err
	}
	defer bus.Close()

	replies, authors, err := pullReplies(ctx, det, reader, bus, *room)
	if err != nil {
		return err
	}
	if replies == 0 {
		fmt.Println("ettle: no new teammate replies to pull.")
		return nil
	}
	fmt.Printf("ettle: pulled %s from %s into room %q.\n",
		plural(replies, "reply", "replies"), plural(authors, "teammate", "teammates"), *room)
	return nil
}
