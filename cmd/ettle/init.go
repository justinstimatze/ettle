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
)

// `ettle init` is the one-command setup for a team already on Linear and Claude Code.
//
// It exists because the pieces were all built and the assembly was six manual steps —
// mint keys, find the team id, create the project, remember the room in every command,
// hand-merge four hooks, and know which of the four env vars gates which feature. This
// does the assembly and, more importantly, REPORTS it: every check names what it unlocks,
// so a missing piece reads as "escalation is off" rather than a stack trace three days
// later.

// hookSpec is one Claude Code hook entry ettle wants installed. The commands name no
// room — they resolve it from the project's `.ettle-room` — which is what lets a single
// global settings.json serve every project (see roomfile.go).
type hookSpec struct {
	event   string
	matcher string
	command string
	why     string
}

func ettleHooks() []hookSpec {
	return []hookSpec{
		{"SessionStart", "", "ettle horizon-hook", "inject the knots relevant to you when a session opens"},
		{"SessionStart", "", "ettle pull-hook", "ingest teammates' Linear replies before you look"},
		{"SessionEnd", "", "ettle capture-hook", "distill this session and publish your atoms"},
		{"Stop", "", "ettle capture-hook", "same, mid-session (debounced, so not every turn)"},
		{"PostToolUse", "mcp__linear", "ettle pull-hook", "touching Linear catches you up"},
	}
}

// check is one line of the init report. required=false means the feature it gates is
// optional, so a missing one is information rather than a failure.
type check struct {
	ok       bool
	required bool
	name     string
	what     string
}

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	me := fs.String("me", defaultAgent(), "your identity in the room — how your atoms are attributed to you")
	dir := fs.String("dir", "", "project directory for .ettle-room (default: the git root above the cwd, else the cwd)")
	install := fs.Bool("install-hooks", false, "merge the ettle hooks into ~/.claude/settings.json (a .bak is written first); default prints the JSON to merge yourself")
	settings := fs.String("settings", "", "which settings file --install-hooks writes (default: ~/.claude/settings.json, which serves every project)")
	room, rest := liftURL(args)
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if room == "" {
		room = fs.Arg(0)
	}
	room = strings.TrimSpace(strings.TrimPrefix(room, "linear://"))
	if room == "" {
		return fmt.Errorf(`usage: ettle init <room> [--me <you>] [--install-hooks]

  Sets up a Linear-backed room for this project: verifies the keys, resolves (or
  creates) the Linear project that carries the atoms, writes .ettle-room here, and
  wires the Claude Code hooks. The room is any short name — teammates pass the same
  one. Not on Linear? ` + "`ettle room init <git-url>`" + ` is the git-repo bus instead.`)
	}

	var out strings.Builder
	fmt.Fprintf(&out, "\n  ettle init — Linear room %q\n", room)

	env := envChecks()
	fmt.Fprint(&out, renderChecks("environment", env))

	busOK, busLine := verifyLinearRoom(room)
	fmt.Fprint(&out, renderChecks("bus", []check{{ok: busOK, required: true, name: "ettle-" + room, what: busLine}}))

	target, err := projectDir(*dir)
	if err != nil {
		return err
	}
	spec := "linear://" + room
	path := filepath.Join(target, roomFileName)
	if err := os.WriteFile(path, []byte(renderRoomFile(spec)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := saveIdentity(spec, *me); err != nil {
		return fmt.Errorf("save identity: %w", err)
	}
	fmt.Fprint(&out, renderChecks("project", []check{{
		ok: true, required: true, name: path,
		what: fmt.Sprintf("room = %s — every ettle command in this tree reads it, so none need --room. Safe to commit; you are %q on this machine only, so a teammate's atoms are never published as yours", spec, *me),
	}}))

	hookLine, hookBody, hookErr := setupHooks(*install, *settings)
	fmt.Fprint(&out, renderChecks("hooks", []check{{ok: hookErr == nil && *install, required: false, name: "Claude Code", what: hookLine}}))
	if hookBody != "" {
		fmt.Fprint(&out, indentBlock(hookBody, "      "))
	}

	fmt.Fprint(&out, renderNextSteps(room, *me, busOK, env))
	fmt.Print(out.String())

	if !busOK || !allRequiredOK(env) {
		return fmt.Errorf("setup is incomplete — see the ✗ lines above (docs/LINEAR_SETUP.md has each key)")
	}
	return nil
}

// envChecks reports which of the four env vars are present and what each one buys.
// Presence only: spending a model call to prove the Anthropic key works would make
// `init` slow and billable for a setup command.
func envChecks() []check {
	present := func(k string) bool { return strings.TrimSpace(os.Getenv(k)) != "" }
	return []check{
		{ok: apiKey() != "", required: true, name: "ANTHROPIC_API_KEY",
			what: "distill + reconcile, both run on this machine — raw notes never leave it"},
		{ok: present("LINEAR_API_KEY"), required: true, name: "LINEAR_API_KEY",
			what: "the atom bus (a Linear project's documents) and reading teammates' replies — a personal member key"},
		{ok: present("LINEAR_TEAM_ID"), required: false, name: "LINEAR_TEAM_ID",
			what: "only needed the first time, to create the room's project; ignored once it exists"},
		{ok: present("LINEAR_AGENT_TOKEN"), required: false, name: "LINEAR_AGENT_TOKEN",
			what: "escalation only — posting a knot to the coordination issue for teammates who don't run ettle (OAuth app-actor token)"},
	}
}

func allRequiredOK(cs []check) bool {
	for _, c := range cs {
		if c.required && !c.ok {
			return false
		}
	}
	return true
}

// verifyLinearRoom is the check that matters: build the real transport, which resolves
// or creates the project, then read it. An env var being set proves nothing about
// whether the key works or the project exists.
func verifyLinearRoom(room string) (bool, string) {
	if strings.TrimSpace(os.Getenv("LINEAR_API_KEY")) == "" {
		return false, "not checked — no LINEAR_API_KEY, so nothing to authenticate with"
	}
	bus, err := linearBusFor(room)
	if err != nil {
		return false, "unreachable: " + err.Error()
	}
	defer bus.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	envs, err := bus.Collect(ctx)
	if err != nil {
		return false, "reachable, but reading it failed: " + err.Error()
	}
	if len(envs) == 0 {
		return true, "project reachable, nobody publishing yet — you'll be first"
	}
	who := make([]string, 0, len(envs))
	for _, e := range envs {
		who = append(who, e.Participant)
	}
	sort.Strings(who)
	return true, fmt.Sprintf("project reachable, %d already publishing: %s", len(who), strings.Join(who, ", "))
}

// projectDir picks where `.ettle-room` goes: the explicit --dir, else the git root
// above the cwd (so a session started in a subdirectory still finds it), else the cwd.
func projectDir(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return filepath.Abs(explicit)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if out, err := git(cwd, "rev-parse", "--show-toplevel"); err == nil {
		if root := strings.TrimSpace(out); root != "" {
			return root, nil
		}
	}
	return cwd, nil
}

// setupHooks either merges the hooks into the settings file or renders the JSON to
// merge by hand. Returns the status line, the JSON body to print (empty when
// installed), and any install error.
func setupHooks(install bool, settingsPath string) (string, string, error) {
	if !install {
		return "not installed — merge this into ~/.claude/settings.json, or re-run with --install-hooks:", hooksJSON(), nil
	}
	path, err := settingsFilePath(settingsPath)
	if err != nil {
		return "install failed: " + err.Error(), hooksJSON(), err
	}
	added, err := installHooks(path)
	if err != nil {
		return "install failed: " + err.Error(), hooksJSON(), err
	}
	if added == 0 {
		return path + " — all five already present, nothing to add", "", nil
	}
	return fmt.Sprintf("%s — %d added (a .bak of the previous file is beside it)", path, added), "", nil
}

func settingsFilePath(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return filepath.Abs(explicit)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

// hooksJSON renders the fragment to merge by hand — the same entries installHooks
// would add, so the two paths never drift.
func hooksJSON() string {
	hooks := map[string]any{}
	for _, h := range ettleHooks() {
		groups, _ := hooks[h.event].([]any)
		hooks[h.event] = addToGroups(groups, h)
	}
	data, err := json.MarshalIndent(map[string]any{"hooks": hooks}, "", "  ")
	if err != nil {
		return ""
	}
	return string(data) + "\n"
}

// addToGroups appends a command to ettle's own group for this event and matcher —
// two SessionStart hooks belong in one group, not two. It never joins a group
// someone else's hooks live in: merging into a stranger's group would make ettle
// hard to remove and easy to blame for their hook's behavior.
func addToGroups(groups []any, h hookSpec) []any {
	entry := map[string]any{"type": "command", "command": h.command}
	for _, g := range groups {
		gm, ok := g.(map[string]any)
		if !ok {
			continue
		}
		if matcher, _ := gm["matcher"].(string); matcher != h.matcher {
			continue
		}
		inner, _ := gm["hooks"].([]any)
		if !allEttleCommands(inner) {
			continue
		}
		gm["hooks"] = append(inner, entry)
		return groups
	}
	g := map[string]any{"hooks": []any{entry}}
	if h.matcher != "" {
		g["matcher"] = h.matcher
	}
	return append(groups, g)
}

func allEttleCommands(entries []any) bool {
	for _, e := range entries {
		em, ok := e.(map[string]any)
		if !ok {
			return false
		}
		cmd, _ := em["command"].(string)
		if !strings.HasPrefix(strings.TrimSpace(cmd), "ettle ") {
			return false
		}
	}
	return true
}

// installHooks merges ettle's hooks into a Claude Code settings file, idempotently:
// an entry whose command already appears under the same event and matcher is left
// alone, so re-running init never duplicates hooks. The previous file is copied to
// <path>.bak first — this is the user's global config and a bad merge should be one
// `mv` from undone.
func installHooks(path string) (int, error) {
	settings := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if len(strings.TrimSpace(string(data))) > 0 {
			if err := json.Unmarshal(data, &settings); err != nil {
				return 0, fmt.Errorf("%s is not valid JSON, refusing to overwrite it: %w", path, err)
			}
		}
		if err := os.WriteFile(path+".bak", data, 0o600); err != nil {
			return 0, fmt.Errorf("write backup %s.bak: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return 0, err
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	added := 0
	for _, h := range ettleHooks() {
		groups, _ := hooks[h.event].([]any)
		if hasHookCommand(groups, h) {
			continue
		}
		hooks[h.event] = addToGroups(groups, h)
		added++
	}
	if added == 0 {
		return 0, nil
	}
	settings["hooks"] = hooks

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return 0, err
	}
	return added, nil
}

// hasHookCommand reports whether this exact (matcher, command) is already wired under
// an event, so a re-run is a no-op. Matching on the command PREFIX rather than equality
// keeps a hand-edited entry that added flags (`ettle capture-hook --debounce 5m`) from
// being duplicated.
func hasHookCommand(groups []any, h hookSpec) bool {
	for _, g := range groups {
		gm, ok := g.(map[string]any)
		if !ok {
			continue
		}
		matcher, _ := gm["matcher"].(string)
		if matcher != h.matcher {
			continue
		}
		inner, _ := gm["hooks"].([]any)
		for _, e := range inner {
			em, ok := e.(map[string]any)
			if !ok {
				continue
			}
			if cmd, _ := em["command"].(string); strings.HasPrefix(strings.TrimSpace(cmd), h.command) {
				return true
			}
		}
	}
	return false
}

// --- rendering (pure, so the report is testable without keys or a network) ------

func renderChecks(section string, cs []check) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n  %s\n", section)
	for _, c := range cs {
		mark := "✗"
		if c.ok {
			mark = "✓"
		}
		opt := ""
		if !c.required && !c.ok {
			opt = " (optional)"
		}
		fmt.Fprintf(&b, "    %s %s%s\n        %s\n", mark, c.name, opt, c.what)
	}
	return b.String()
}

func indentBlock(s, pad string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n") + "\n"
}

// renderNextSteps says what to do next, branching on what actually succeeded — the
// same discipline as printRoomNextSteps: never hand someone a command that cannot
// work for them yet.
func renderNextSteps(room, me string, busOK bool, env []check) string {
	var b strings.Builder
	b.WriteString("\n  next\n")
	if !allRequiredOK(env) || !busOK {
		b.WriteString("    fix the ✗ lines first — docs/LINEAR_SETUP.md walks each key, including\n")
		b.WriteString("    minting the OAuth app-actor token escalation needs.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "    ettle horizon --me %s          # what the room already knows that concerns you\n", me)
	fmt.Fprintf(&b, "    claude mcp add ettle -- ettle mcp --transport linear://%s   # operate it from inside a session\n", room)
	fmt.Fprintf(&b, "    tell a teammate:  ettle init %s\n", room)
	b.WriteString("\n    With the hooks in, nothing else is a command you run: your sessions publish\n")
	b.WriteString("    your atoms, teammates' Linear replies come in, and the knots that involve you\n")
	b.WriteString("    surface at the start of your next session. Nothing is posted anywhere shared\n")
	b.WriteString("    unless you escalate it on purpose.\n")
	return b.String()
}
