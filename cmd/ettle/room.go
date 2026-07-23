package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/transport"
)

// The `ettle room` command collapses the leat-transport setup ceremony (create
// or clone a git repo, seed a HEAD, remember three env vars and an absolute
// path) into a one-time join, so day-to-day use is just `standup --room <name>`.
// A room is a private git repo used as a leat bus; the git URL is the invite.

// roomConfig is the saved per-room setup, written under the user config dir at
// <config>/ettle/rooms/<name>/config.json. Remote == "" means a local-only room.
type roomConfig struct {
	Name    string `json:"name"`
	RepoDir string `json:"repo_dir"`
	Remote  string `json:"remote"`
	Agent   string `json:"agent"`
}

func roomsBase() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	return filepath.Join(dir, "ettle", "rooms"), nil
}

func roomDir(name string) (string, error) {
	base, err := roomsBase()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, name), nil
}

func loadRoom(name string) (roomConfig, error) {
	dir, err := roomDir(name)
	if err != nil {
		return roomConfig{}, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		return roomConfig{}, fmt.Errorf("room %q not found — run `ettle room join <git-url>` (or `ettle room list`): %w", name, err)
	}
	var rc roomConfig
	if err := json.Unmarshal(data, &rc); err != nil {
		return roomConfig{}, fmt.Errorf("room %q: corrupt config: %w", name, err)
	}
	return rc, nil
}

func saveRoom(rc roomConfig) error {
	dir, err := roomDir(rc.Name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644)
}

// roomBus builds the leat transport for a configured room — what `--room <name>`
// resolves to, so standup needs no LEAT_* env vars or --transport string.
func roomBus(name string) (transport.Transport, error) {
	rc, err := loadRoom(name)
	if err != nil {
		return nil, err
	}
	return transport.NewLeatBus(rc.RepoDir, rc.Agent, rc.Remote, rc.Name)
}

// selectBus picks the transport for a standup run: a configured room (if --room
// is set) overrides the --transport string.
func selectBus(cfg runConfig) (transport.Transport, error) {
	if cfg.room != "" {
		return roomBus(cfg.room)
	}
	return busFor(cfg.transport, cfg.insecureLocal)
}

func runRoom(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ettle room <init|join|list|status> ...")
	}
	switch args[0] {
	case "init":
		return roomInit(args[1:])
	case "join":
		return roomJoin(args[1:])
	case "list":
		return roomList()
	case "status", "who":
		return roomStatus(args[1:])
	default:
		return fmt.Errorf("unknown room subcommand %q (init | join | list | status)", args[0])
	}
}

// roomStatus is the presence view: who's in the room and what each is currently
// working on — read straight off the bus (the atoms standup already published),
// no tangle detection and no model call. This is the L0 co-presence layer: useful
// before any reconciliation, just "what is my crew's agents doing right now."
func roomStatus(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ettle room status <name>")
	}
	name := args[0]
	bus, err := roomBus(name)
	if err != nil {
		return err
	}
	defer bus.Close()

	envs, err := bus.Collect(context.Background())
	if err != nil {
		return err
	}
	var warnings []string
	if w, ok := bus.(interface{ Warnings() []string }); ok {
		warnings = w.Warnings()
	}
	// NOTE: the leat bus reads via Collect, which drops identity spoofs SILENTLY
	// (only Receive records warnings), so today this is effectively always empty —
	// the spoof is still dropped (security holds), just not surfaced here. The
	// rendering below is forward-compatible for when the transport reports them.
	fmt.Print(renderRoomStatus(name, envs, warnings, time.Now()))
	return nil
}

// renderRoomStatus formats the presence view. Pure (now is injected) so it is
// testable without a clock or a bus.
func renderRoomStatus(name string, envs []transport.Envelope, warnings []string, now time.Time) string {
	sort.Slice(envs, func(i, j int) bool {
		return strings.ToLower(envs[i].Participant) < strings.ToLower(envs[j].Participant)
	})
	var b strings.Builder
	fmt.Fprintf(&b, "\n  room %q — %d present\n", name, len(envs))
	if len(envs) == 0 {
		fmt.Fprintf(&b, "    nobody has published yet — run: ettle standup --room %s --me you notes.md\n", name)
	}
	// Stable type order, friendly framing — presence reads as "what are you doing".
	order := []ettlemesh.AtomType{ettlemesh.Intent, ettlemesh.Commitment, ettlemesh.Dependency, ettlemesh.Assumption}
	for _, e := range envs {
		who := e.Participant
		if e.Role != "" {
			who += " (" + e.Role + ")"
		}
		if fresh := freshnessLabel(e.EmittedAt, now); fresh != "" {
			who += " · " + fresh
		}
		fmt.Fprintf(&b, "\n    %s\n", who)
		if len(e.Atoms) == 0 {
			fmt.Fprintln(&b, "      (no atoms)")
			continue
		}
		for _, typ := range order {
			var items []ettlemesh.Atom
			for _, a := range e.Atoms {
				if a.Typ == typ {
					items = append(items, a)
				}
			}
			if len(items) == 0 {
				continue
			}
			fmt.Fprintf(&b, "      %s:\n", workLabel(typ))
			for _, a := range items {
				fmt.Fprintf(&b, "        • %s\n", roomAtomLine(a))
			}
		}
	}
	if len(warnings) > 0 {
		fmt.Fprintf(&b, "\n    ⚠ %d transport warning(s) (dropped identity spoofs / malformed lines):\n", len(warnings))
		for _, w := range warnings {
			fmt.Fprintf(&b, "      - %s\n", w)
		}
	}
	return b.String()
}

func workLabel(t ettlemesh.AtomType) string {
	switch t {
	case ettlemesh.Intent:
		return "working on"
	case ettlemesh.Commitment:
		return "committed"
	case ettlemesh.Dependency:
		return "depends on"
	case ettlemesh.Assumption:
		return "assuming"
	default:
		return string(t)
	}
}

func roomAtomLine(a ettlemesh.Atom) string {
	if strings.TrimSpace(a.Subject) != "" {
		return a.Subject + " — " + a.Content
	}
	return a.Content
}

// freshnessLabel turns an envelope's emit time into a coarse presence cue. Empty
// or unparseable yields "" (no cue) rather than a wrong one.
func freshnessLabel(emittedAt string, now time.Time) string {
	if emittedAt == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, emittedAt)
	if err != nil {
		return ""
	}
	switch d := now.Sub(t); {
	case d < 0:
		return "active"
	case d < 2*time.Hour:
		return "active"
	case d < 24*time.Hour:
		return "today"
	case d < 48*time.Hour:
		return "yesterday"
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	}
}

// roomInit starts a room: clone <git-url> (or create a local-only repo with no
// URL), ensure a born HEAD, and save the config. The first person runs this.
func roomInit(args []string) error {
	fs := flag.NewFlagSet("room init", flag.ContinueOnError)
	as := fs.String("as", defaultAgent(), "your agent id in this room (becomes your lane filename)")
	name := fs.String("name", "", "room name (default: derived from the git URL, or \"local\")")
	url, rest := liftURL(args)
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if url == "" {
		url = fs.Arg(0)
	}
	agent := transport.SanitizeID(*as)

	rname := transport.SanitizeID(*name)
	if *name == "" {
		if url != "" {
			rname = roomNameFromURL(url)
		} else {
			rname = "local"
		}
	}

	dir, err := roomDir(rname)
	if err != nil {
		return err
	}
	repoDir := filepath.Join(dir, "repo")
	remote := ""
	if url != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if _, err := git("", "clone", url, repoDir); err != nil {
			return err
		}
		remote = "origin"
	} else {
		if err := os.MkdirAll(repoDir, 0o755); err != nil {
			return err
		}
		if _, err := git(repoDir, "init", "-b", "main"); err != nil {
			return err
		}
	}
	if err := ensureSeed(repoDir, remote); err != nil {
		return err
	}
	if err := saveRoom(roomConfig{Name: rname, RepoDir: repoDir, Remote: remote, Agent: agent}); err != nil {
		return err
	}

	fmt.Printf("room %q ready (you are %q)\n", rname, agent)
	if url != "" {
		fmt.Printf("  invite your crew:  ettle room join %s\n", url)
	} else {
		fmt.Printf("  local-only (no remote yet) — add one with:  git -C %s remote add origin <url>\n", repoDir)
	}
	fmt.Printf("  use it:            ettle standup --room %s --me %s notes.md\n", rname, agent)
	return nil
}

// roomJoin joins an existing room by cloning its git URL and saving the config.
func roomJoin(args []string) error {
	fs := flag.NewFlagSet("room join", flag.ContinueOnError)
	as := fs.String("as", defaultAgent(), "your agent id in this room (becomes your lane filename)")
	name := fs.String("name", "", "room name (default: derived from the git URL)")
	url, rest := liftURL(args)
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if url == "" {
		url = fs.Arg(0)
	}
	if url == "" {
		return fmt.Errorf("usage: ettle room join <git-url> [--as <id>] [--name <room>]")
	}
	agent := transport.SanitizeID(*as)
	rname := transport.SanitizeID(*name)
	if *name == "" {
		rname = roomNameFromURL(url)
	}

	dir, err := roomDir(rname)
	if err != nil {
		return err
	}
	repoDir := filepath.Join(dir, "repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if _, err := git("", "clone", url, repoDir); err != nil {
		return err
	}
	if err := ensureSeed(repoDir, "origin"); err != nil {
		return err
	}
	if err := saveRoom(roomConfig{Name: rname, RepoDir: repoDir, Remote: "origin", Agent: agent}); err != nil {
		return err
	}

	fmt.Printf("joined room %q (you are %q)\n", rname, agent)
	fmt.Printf("  use it:  ettle standup --room %s --me %s notes.md\n", rname, agent)
	return nil
}

func roomList() error {
	base, err := roomsBase()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("no rooms yet — start one with `ettle room init|join <git-url>`")
			return nil
		}
		return err
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		rc, err := loadRoom(e.Name())
		if err != nil {
			continue
		}
		remote := rc.Remote
		if remote == "" {
			remote = "(local-only)"
		}
		fmt.Printf("  %-18s  you=%-12s  remote=%s\n      %s\n", rc.Name, rc.Agent, remote, rc.RepoDir)
		n++
	}
	if n == 0 {
		fmt.Println("no rooms yet — start one with `ettle room init|join <git-url>`")
	}
	return nil
}

// --- helpers ---------------------------------------------------------------

// liftURL pulls a leading positional URL out of args before flag parsing,
// because Go's flag package stops at the first non-flag token — so without this
// `room init <url> --as alice` would silently ignore --as. Returns the URL (""
// if the first arg is a flag) and the remaining args to hand to fs.Parse. The
// caller still falls back to fs.Arg(0) for the flags-first form
// (`room init --as alice <url>`).
func liftURL(args []string) (url string, rest []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "", args
}

func defaultAgent() string {
	for _, k := range []string{"USER", "USERNAME", "LOGNAME"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return "me"
}

// roomNameFromURL derives a room name from a git URL's last path segment, e.g.
// git@github.com:crew/standup-room.git -> standup-room.
func roomNameFromURL(url string) string {
	u := strings.TrimRight(strings.TrimSuffix(strings.TrimSpace(url), ".git"), "/")
	if i := strings.LastIndexAny(u, "/:"); i >= 0 {
		u = u[i+1:]
	}
	return transport.SanitizeID(u)
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %v\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// ensureSeed guarantees repoDir has a born HEAD (leat.New requires one). If the
// repo has no commits, it writes a seed commit; if a remote is set, it pushes so
// the room is initialized for everyone who joins.
func ensureSeed(repoDir, remote string) error {
	if _, err := git(repoDir, "rev-parse", "HEAD"); err == nil {
		return nil // already has history
	}
	readme := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readme, []byte("# ettle room\n\nA leat message bus (per-author lanes under channels/). Managed by `ettle room`.\n"), 0o644); err != nil {
		return err
	}
	if _, err := git(repoDir, "add", "README.md"); err != nil {
		return err
	}
	if _, err := git(repoDir, "-c", "user.name=ettle", "-c", "user.email=ettle@local", "commit", "-m", "ettle: seed room"); err != nil {
		return err
	}
	if remote != "" {
		if _, err := git(repoDir, "push", "-u", remote, "HEAD"); err != nil {
			return err
		}
	}
	return nil
}
