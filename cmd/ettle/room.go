package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
		return fmt.Errorf("usage: ettle room <init|join|list> ...")
	}
	switch args[0] {
	case "init":
		return roomInit(args[1:])
	case "join":
		return roomJoin(args[1:])
	case "list":
		return roomList()
	default:
		return fmt.Errorf("unknown room subcommand %q (init | join | list)", args[0])
	}
}

// roomInit starts a room: clone <git-url> (or create a local-only repo with no
// URL), ensure a born HEAD, and save the config. The first person runs this.
func roomInit(args []string) error {
	fs := flag.NewFlagSet("room init", flag.ContinueOnError)
	as := fs.String("as", defaultAgent(), "your agent id in this room (becomes your lane filename)")
	name := fs.String("name", "", "room name (default: derived from the git URL, or \"local\")")
	if err := fs.Parse(args); err != nil {
		return err
	}
	url := fs.Arg(0)
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
	if err := fs.Parse(args); err != nil {
		return err
	}
	url := fs.Arg(0)
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
