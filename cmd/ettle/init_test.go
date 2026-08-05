package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHooksJSONCoversEveryHook(t *testing.T) {
	body := hooksJSON(true)
	var parsed struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("the fragment we tell people to paste must be valid JSON: %v\n%s", err, body)
	}
	got := map[string]bool{}
	for event, groups := range parsed.Hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				got[event+"|"+g.Matcher+"|"+h.Command] = true
			}
		}
	}
	for _, h := range ettleHooks(true) {
		if !got[h.event+"|"+h.matcher+"|"+h.command] {
			t.Errorf("hook %s %q missing from the printed fragment", h.event, h.command)
		}
	}
	// The commands must name no room — that is what lets one global config serve
	// every project (the room comes from .ettle-room).
	if strings.Contains(body, "--room") {
		t.Errorf("hook commands must not hard-code a room:\n%s", body)
	}
}

func TestHooksSkipTheLinearOnlyPullHookElsewhere(t *testing.T) {
	gh := hooksJSON(false)
	if strings.Contains(gh, "pull-hook") {
		t.Errorf("pull reads Linear agent activities; a GitHub room must not wire it:\n%s", gh)
	}
	if strings.Contains(gh, "mcp__linear") {
		t.Errorf("a GitHub room must not wire a matcher on a Linear MCP server:\n%s", gh)
	}
	for _, want := range []string{"horizon-hook", "capture-hook"} {
		if !strings.Contains(gh, want) {
			t.Errorf("the bus-and-whisper half still applies everywhere, missing %q:\n%s", want, gh)
		}
	}
	if !strings.Contains(hooksJSON(true), "pull-hook") {
		t.Error("a Linear room should still get pull-hook")
	}
}

func TestGitHubRemoteDerivesTheRoom(t *testing.T) {
	for _, url := range []string{
		"git@github.com:acme/widgets.git",
		"https://github.com/acme/widgets.git",
		"https://github.com/acme/widgets",
		"ssh://git@github.com/acme/widgets.git",
	} {
		owner, repo, ok := parseGitHubRemote(url)
		if !ok || owner != "acme" || repo != "widgets" {
			t.Errorf("%q → %q/%q ok=%v, want acme/widgets", url, owner, repo, ok)
		}
	}
	// Not GitHub → say so rather than invent a room that fails later, further from
	// the cause.
	for _, url := range []string{
		"git@gitlab.com:acme/widgets.git",
		"https://github.com/acme",
		"/some/local/path",
		"",
	} {
		if _, _, ok := parseGitHubRemote(url); ok {
			t.Errorf("%q must not parse as a GitHub room", url)
		}
	}
}

func TestInstallHooksIsIdempotentAndPreservesOtherSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	existing := `{"model":"opus","hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"something-else"}]}]}}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	added, err := installHooks(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if added != len(ettleHooks(true)) {
		t.Errorf("first install should add every hook: got %d, want %d", added, len(ettleHooks(true)))
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("the previous settings file should be backed up: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "opus" {
		t.Errorf("unrelated settings must survive the merge: %v", got["model"])
	}
	if !strings.Contains(string(data), "something-else") {
		t.Error("an existing unrelated hook must survive the merge")
	}

	again, err := installHooks(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if again != 0 {
		t.Errorf("re-running init must not duplicate hooks: added %d", again)
	}
}

func TestInstallHooksCreatesMissingFileAndRefusesGarbage(t *testing.T) {
	dir := t.TempDir()
	fresh := filepath.Join(dir, "nested", "settings.json")
	if added, err := installHooks(fresh, true); err != nil || added != len(ettleHooks(true)) {
		t.Fatalf("a missing settings file should be created: added=%d err=%v", added, err)
	}

	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := installHooks(bad, true); err == nil {
		t.Error("unparseable settings must be refused, not overwritten")
	}
	data, _ := os.ReadFile(bad)
	if string(data) != "{not json" {
		t.Errorf("the file we refused to parse must be left alone, got %q", data)
	}
}

func TestHasHookCommandToleratesAddedFlags(t *testing.T) {
	groups := []any{map[string]any{
		"hooks": []any{map[string]any{"type": "command", "command": "ettle capture-hook --debounce 5m"}},
	}}
	h := hookSpec{event: "Stop", command: "ettle capture-hook"}
	if !hasHookCommand(groups, h) {
		t.Error("a hand-tuned hook with extra flags should count as already installed")
	}
	if hasHookCommand(groups, hookSpec{event: "Stop", matcher: "mcp__linear", command: "ettle capture-hook"}) {
		t.Error("a different matcher is a different hook")
	}
}

func TestRenderChecksMarksRequiredAndOptional(t *testing.T) {
	got := renderChecks("environment", []check{
		{ok: true, required: true, name: "ANTHROPIC_API_KEY", what: "distill"},
		{ok: false, required: false, name: "LINEAR_AGENT_TOKEN", what: "escalation only"},
	})
	if !strings.Contains(got, "✓ ANTHROPIC_API_KEY") {
		t.Errorf("a present key should read as satisfied:\n%s", got)
	}
	if !strings.Contains(got, "✗ LINEAR_AGENT_TOKEN (optional)") {
		t.Errorf("a missing optional key should say it is optional:\n%s", got)
	}
}

func TestRenderNextStepsWithholdsCommandsThatCannotWork(t *testing.T) {
	blocked := renderNextSteps("linear://crew", "justin", false)
	if strings.Contains(blocked, "ettle horizon") {
		t.Errorf("don't hand someone a command that cannot work yet:\n%s", blocked)
	}
	// A URL, not a repo path: whoever hits this installed the binary and has no clone.
	if !strings.Contains(blocked, "https://github.com/justinstimatze/ettle/blob/main/docs/LINEAR_SETUP.md") {
		t.Errorf("point at a doc the reader can actually open:\n%s", blocked)
	}
	ok := renderNextSteps("linear://crew", "justin", true)
	for _, want := range []string{"ettle horizon --me justin", "ettle init linear://crew"} {
		if !strings.Contains(ok, want) {
			t.Errorf("a working setup should lead with %q:\n%s", want, ok)
		}
	}
}

func TestInitReportJSONCarriesTheSameFactsAsProse(t *testing.T) {
	rep := initReport{
		Room: "linear://crew", Label: `Linear room "crew"`, Me: "justin", OK: false,
		RoomFile: "/tmp/x/.ettle-room", Docs: docsLinearSetup,
		Environment: []check{{ok: true, required: true, name: "ANTHROPIC_API_KEY", what: "distill"}},
		Bus:         []check{{ok: false, required: true, name: "ettle-crew", what: "no key"}},
	}
	var got struct {
		Room, Me, Docs, RoomFile string
		OK                       bool
		Environment, Bus         []struct {
			Name     string
			OK       bool
			Required bool
			What     string
		}
	}
	if err := json.Unmarshal([]byte(renderInitReport(rep, true)), &got); err != nil {
		t.Fatalf("--json must emit valid JSON for an agent to branch on: %v", err)
	}
	if got.Room != "linear://crew" || got.Me != "justin" || got.OK {
		t.Errorf("headline facts lost: %+v", got)
	}
	if len(got.Environment) != 1 || got.Environment[0].Name != "ANTHROPIC_API_KEY" || !got.Environment[0].OK {
		t.Errorf("check fields must survive marshaling: %+v", got.Environment)
	}
	if len(got.Bus) != 1 || got.Bus[0].OK || got.Bus[0].What != "no key" {
		t.Errorf("the WHY of a failure is the useful part: %+v", got.Bus)
	}
	if got.Docs == "" {
		t.Error("an agent needs somewhere to send the human")
	}

	// Prose mode reports the same run, in the human rendering.
	prose := renderInitReport(rep, false)
	for _, want := range []string{`Linear room "crew"`, "✓ ANTHROPIC_API_KEY", "✗ ettle-crew"} {
		if !strings.Contains(prose, want) {
			t.Errorf("prose rendering missing %q:\n%s", want, prose)
		}
	}
}
