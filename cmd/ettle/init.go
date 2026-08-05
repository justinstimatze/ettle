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

	"github.com/justinstimatze/ettle/internal/transport"
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

// ettleHooks is the bundle for a room. The two pull-hook entries are LINEAR-ONLY:
// pull reads replies off Linear's agent activities, a surface GitHub has no
// counterpart for, so installing them for a GitHub room would wire two hooks that
// can only ever no-op — and a PostToolUse matcher on a Linear MCP server the team
// doesn't run.
func ettleHooks(linear bool) []hookSpec {
	hooks := []hookSpec{
		{"SessionStart", "", "ettle horizon-hook", "inject the knots relevant to you when a session opens"},
		{"SessionEnd", "", "ettle capture-hook", "distill this session and publish your atoms"},
		{"Stop", "", "ettle capture-hook", "same, mid-session (debounced, so not every turn)"},
	}
	if !linear {
		return hooks
	}
	return append(hooks,
		hookSpec{"SessionStart", "", "ettle pull-hook", "ingest teammates' Linear replies before you look"},
		hookSpec{"PostToolUse", "mcp__linear", "ettle pull-hook", "touching Linear catches you up"},
	)
}

// check is one line of the init report. required=false means the feature it gates is
// optional, so a missing one is information rather than a failure.
type check struct {
	ok       bool
	required bool
	name     string
	what     string
}

// MarshalJSON exports a check for --json. Written here rather than by exporting the
// fields so the terse literals above stay readable; the two renderings therefore
// carry exactly the same facts, which is the point of having a machine mode at all.
func (c check) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name     string `json:"name"`
		OK       bool   `json:"ok"`
		Required bool   `json:"required"`
		What     string `json:"what"`
	}{c.name, c.ok, c.required, c.what})
}

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	me := fs.String("me", defaultAgent(), "your identity in the room — how your atoms are attributed to you")
	dir := fs.String("dir", "", "project directory for .ettle-room (default: the git root above the cwd, else the cwd)")
	install := fs.Bool("install-hooks", false, "merge the ettle hooks into ~/.claude/settings.json (a .bak is written first); default prints the JSON to merge yourself")
	settings := fs.String("settings", "", "which settings file --install-hooks writes (default: ~/.claude/settings.json, which serves every project)")
	asJSON := fs.Bool("json", false, "emit the report as JSON instead of prose — for an agent driving the setup, so it can branch on what's missing rather than parse English")
	room, rest := liftURL(args)
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if room == "" {
		room = fs.Arg(0)
	}
	derived := false
	if strings.TrimSpace(room) == "" {
		// Naming a room is a decision nobody wants to make, and inside a GitHub repo it
		// is one nobody has to: the repo IS the room. Derive it from the origin remote,
		// which also guarantees teammates land on the same room without agreeing on a
		// name first — the failure mode where two people typo different rooms and each
		// sees an empty bus.
		if spec, ok := roomFromGitRemote(*dir); ok {
			room, derived = spec, true
		}
	}
	if strings.TrimSpace(room) == "" {
		return fmt.Errorf(`usage: ettle init [<room>] [--me <you>] [--install-hooks]

  Inside a GitHub repo, the room needs no name — it is derived from the origin
  remote, so teammates cannot land in different rooms by typing different names.
  Elsewhere, name it:

    ettle init github://<owner>/<repo>[/<room>]   a PRIVATE repo's Discussions
    ettle init <name>                             a Linear room (project ettle-<name>)

  Either way this verifies the keys, resolves (or creates) the thing that carries
  the atoms, writes .ettle-room here, and wires the Claude Code hooks. Neither
  platform? ` + "`ettle room init <git-url>`" + ` is the plain git-repo bus.

  (No origin remote found here, which is why you are reading this.)`)
	}

	spec, label, env, verify := initTarget(room)
	if derived {
		label += " — derived from the origin remote, so a teammate runs the same bare `ettle init`"
	}
	rep := initReport{Room: spec, Label: label, Me: *me, Derived: derived, Docs: docsLinearSetup, Environment: env}

	// The report is emitted on EVERY exit, including a failure partway down: the
	// checks already gathered are the most useful thing to show whoever's setup just
	// broke, and swallowing them to print one error line would hide exactly the
	// diagnosis they need.
	defer func() { fmt.Print(renderInitReport(rep, *asJSON)) }()

	busOK, busName, busLine := verify()
	rep.Bus = []check{{ok: busOK, required: true, name: busName, what: busLine}}

	target, err := projectDir(*dir)
	if err != nil {
		return err
	}
	path := filepath.Join(target, roomFileName)
	if err := os.WriteFile(path, []byte(renderRoomFile(spec)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := saveIdentity(spec, *me); err != nil {
		return fmt.Errorf("save identity: %w", err)
	}
	rep.RoomFile = path
	rep.Project = []check{{
		ok: true, required: true, name: path,
		what: fmt.Sprintf("room = %s — every ettle command in this tree reads it, so none need --room. Safe to commit; you are %q on this machine only, so a teammate's atoms are never published as yours", spec, *me),
	}}

	hookLine, hookBody, hookErr := setupHooks(*install, *settings, strings.HasPrefix(spec, "linear://"))
	rep.HooksInstalled = hookErr == nil && *install
	rep.HooksJSON = hookBody
	rep.Hooks = []check{{ok: rep.HooksInstalled, required: false, name: "Claude Code", what: hookLine}}

	rep.OK = busOK && allRequiredOK(env)
	if !rep.OK {
		return fmt.Errorf("setup is incomplete — see the ✗ lines above (%s has each key)", docsLinearSetup)
	}
	return nil
}

// roomFromGitRemote derives github://<owner>/<repo> from the origin remote of the
// repo containing dir (or the cwd), so `ettle init` inside a GitHub checkout needs
// no argument at all.
func roomFromGitRemote(dir string) (string, bool) {
	at := strings.TrimSpace(dir)
	if at == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", false
		}
		at = cwd
	}
	out, err := git(at, "remote", "get-url", "origin")
	if err != nil {
		return "", false
	}
	owner, repo, ok := parseGitHubRemote(out)
	if !ok {
		return "", false
	}
	return "github://" + owner + "/" + repo, true
}

// parseGitHubRemote pulls owner/repo out of the GitHub remote URL forms git actually
// hands back. Anything that is not github.com returns false rather than a guess — a
// GitLab remote is not a GitHub room, and silently inventing one would fail later and
// further from the cause.
func parseGitHubRemote(url string) (owner, repo string, ok bool) {
	u := strings.TrimSuffix(strings.TrimSpace(url), ".git")
	switch {
	case strings.HasPrefix(u, "git@github.com:"):
		u = strings.TrimPrefix(u, "git@github.com:")
	case strings.HasPrefix(u, "ssh://git@github.com/"):
		u = strings.TrimPrefix(u, "ssh://git@github.com/")
	case strings.HasPrefix(u, "https://github.com/"):
		u = strings.TrimPrefix(u, "https://github.com/")
	case strings.HasPrefix(u, "http://github.com/"):
		u = strings.TrimPrefix(u, "http://github.com/")
	default:
		return "", "", false
	}
	parts := strings.Split(strings.Trim(u, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// docsLinearSetup is a URL, not a repo-relative path: the person most likely to hit
// a ✗ line installed the binary with `go install` and has no clone to look in.
const docsLinearSetup = "https://github.com/justinstimatze/ettle/blob/main/docs/LINEAR_SETUP.md"

// initReport is the whole outcome of a setup run, so it can render as prose for a
// human or as JSON for an agent driving the install — the same facts either way,
// rather than a machine mode that quietly reports less.
type initReport struct {
	Room           string  `json:"room"`
	Label          string  `json:"label"`
	Me             string  `json:"me"`
	OK             bool    `json:"ok"`
	Derived        bool    `json:"derived_from_git_remote"`
	RoomFile       string  `json:"room_file,omitempty"`
	HooksInstalled bool    `json:"hooks_installed"`
	HooksJSON      string  `json:"hooks_json,omitempty"`
	Docs           string  `json:"docs"`
	Environment    []check `json:"environment"`
	Bus            []check `json:"bus"`
	Project        []check `json:"project,omitempty"`
	Hooks          []check `json:"hooks,omitempty"`
}

func renderInitReport(rep initReport, asJSON bool) string {
	if asJSON {
		data, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return ""
		}
		return string(data) + "\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n  ettle init — %s\n", rep.Label)
	b.WriteString(renderChecks("environment", rep.Environment))
	b.WriteString(renderChecks("bus", rep.Bus))
	if len(rep.Project) > 0 {
		b.WriteString(renderChecks("project", rep.Project))
	}
	if len(rep.Hooks) > 0 {
		b.WriteString(renderChecks("hooks", rep.Hooks))
		if rep.HooksJSON != "" {
			b.WriteString(indentBlock(rep.HooksJSON, "      "))
		}
	}
	b.WriteString(renderNextSteps(rep.Room, rep.Me, rep.OK))
	return b.String()
}

// initTarget picks which bus the room argument names and returns everything that
// differs between them: the spec to record, the headline, the environment checks,
// and the live verification. Everything after this point — the project pointer, the
// identity, the hooks, the next steps — is identical for both, which is the point of
// splitting here rather than forking runInit.
func initTarget(room string) (spec, label string, env []check, verify func() (bool, string, string)) {
	if ghSpec, ok := strings.CutPrefix(room, "github://"); ok {
		owner, repo, name, err := transport.ParseGitHubSpec(ghSpec)
		if err != nil {
			bad := err.Error()
			return room, "GitHub room " + room, githubEnvChecks(),
				func() (bool, string, string) { return false, room, bad }
		}
		spec = fmt.Sprintf("github://%s/%s/%s", owner, repo, name)
		return spec, fmt.Sprintf("GitHub room %q in %s/%s", name, owner, repo), githubEnvChecks(),
			func() (bool, string, string) { return verifyGitHubRoom(spec, owner, repo, name) }
	}
	name := strings.TrimSpace(strings.TrimPrefix(room, "linear://"))
	return "linear://" + name, fmt.Sprintf("Linear room %q", name), envChecks(),
		func() (bool, string, string) {
			ok, line := verifyLinearRoom(name)
			return ok, "ettle-" + name, line
		}
}

// githubEnvChecks is the GitHub path's environment: two required, and no optional
// escalation token — escalation posts Linear agent activities and has no GitHub
// equivalent yet, so the honest report is that the feature isn't there rather than
// that a key is missing.
func githubEnvChecks() []check {
	tok := githubToken()
	how := "not found — set GITHUB_TOKEN, or sign in once with `gh auth login`"
	if tok != "" {
		how = "a token with `repo` scope, for the repository Discussion that carries the atoms"
		if strings.TrimSpace(os.Getenv("GITHUB_TOKEN")) == "" && strings.TrimSpace(os.Getenv("GH_TOKEN")) == "" {
			how += " — using the one `gh auth` already holds, so there is no new secret to manage"
		}
	}
	return []check{
		{ok: apiKey() != "", required: true, name: "ANTHROPIC_API_KEY",
			what: "distill + reconcile, both run on this machine — raw notes never leave it"},
		{ok: tok != "", required: true, name: "GitHub token", what: how},
	}
}

// verifyGitHubRoom builds the real transport, which is also where the private-repo
// refusal and the Discussions-enabled check live, then reads the room back.
func verifyGitHubRoom(spec, owner, repo, room string) (bool, string, string) {
	name := fmt.Sprintf("%s/%s → discussion \"ettle/%s\"", owner, repo, room)
	if githubToken() == "" {
		return false, name, "not checked — no GitHub token, so nothing to authenticate with"
	}
	bus, err := githubBusFor(strings.TrimPrefix(spec, "github://"))
	if err != nil {
		return false, name, err.Error()
	}
	defer bus.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	envs, err := bus.Collect(ctx)
	if err != nil {
		return false, name, "reachable, but reading it failed: " + err.Error()
	}
	return true, name, "private repo, Discussions on, " + presentLine(envs)
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
	audience := ""
	if lb, ok := bus.(*transport.LinearBus); ok {
		if teams, aErr := lb.Audience(ctx); aErr == nil {
			audience = linearAudienceNote(teams)
		}
	}
	return true, "project reachable, " + presentLine(envs) + audience
}

// presentLine says who is already on the bus — the one fact that tells a new joiner
// whether they typed the same room name their teammates did.
func presentLine(envs []transport.Envelope) string {
	if len(envs) == 0 {
		return "nobody publishing yet — you'll be first"
	}
	who := make([]string, 0, len(envs))
	for _, e := range envs {
		who = append(who, e.Participant)
	}
	sort.Strings(who)
	return fmt.Sprintf("%d already publishing: %s", len(who), strings.Join(who, ", "))
}

// linearAudienceNote names who can read the room. Linear has no internet-public
// project — the nearest thing is the owning team's visibility, where "public" means
// the whole WORKSPACE, not the world — so unlike the GitHub path there is nothing to
// refuse here. What there is, is a reader who should know the audience they just
// picked, because "public" reads alarming and "restricted" reads safe when the
// difference is which colleagues can see it.
func linearAudienceNote(teams []transport.TeamScope) string {
	if len(teams) == 0 {
		return ""
	}
	var parts []string
	for _, t := range teams {
		scope := "everyone in the workspace"
		switch t.Visibility {
		case "private":
			scope = "that team's members only"
		case "restricted":
			scope = "that team's members, discoverable by others"
		}
		parts = append(parts, fmt.Sprintf("%s (%s → %s)", t.Name, t.Visibility, scope))
	}
	return ". Readable by: " + strings.Join(parts, ", ") + " — Linear has no internet-public project, so this is the audience, not a leak"
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
func setupHooks(install bool, settingsPath string, linear bool) (string, string, error) {
	if !install {
		return "not installed — merge this into ~/.claude/settings.json, or re-run with --install-hooks:", hooksJSON(linear), nil
	}
	path, err := settingsFilePath(settingsPath)
	if err != nil {
		return "install failed: " + err.Error(), hooksJSON(linear), err
	}
	added, err := installHooks(path, linear)
	if err != nil {
		return "install failed: " + err.Error(), hooksJSON(linear), err
	}
	if added == 0 {
		return fmt.Sprintf("%s — all %d already present, nothing to add", path, len(ettleHooks(linear))), "", nil
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
func hooksJSON(linear bool) string {
	hooks := map[string]any{}
	for _, h := range ettleHooks(linear) {
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
func installHooks(path string, linear bool) (int, error) {
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
	for _, h := range ettleHooks(linear) {
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
func renderNextSteps(room, me string, ok bool) string {
	var b strings.Builder
	b.WriteString("\n  next\n")
	if !ok {
		fmt.Fprintf(&b, "    fix the ✗ lines first — %s\n", docsLinearSetup)
		b.WriteString("    walks each key, including minting the OAuth app-actor token escalation needs.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "    ettle horizon --me %s          # what the room already knows that concerns you\n", me)
	b.WriteString("    claude mcp add ettle -- ettle mcp   # operate it from inside a session\n")
	fmt.Fprintf(&b, "    tell a teammate:  ettle init %s\n", room)
	b.WriteString("\n    With the hooks in, nothing else is a command you run: your sessions publish\n")
	b.WriteString("    your atoms, teammates' Linear replies come in, and the knots that involve you\n")
	b.WriteString("    surface at the start of your next session. Nothing is posted anywhere shared\n")
	b.WriteString("    unless you escalate it on purpose.\n")
	return b.String()
}
