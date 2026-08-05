package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHooksJSONCoversEveryHook(t *testing.T) {
	body := hooksJSON()
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
	for _, h := range ettleHooks() {
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

func TestInstallHooksIsIdempotentAndPreservesOtherSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	existing := `{"model":"opus","hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"something-else"}]}]}}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	added, err := installHooks(path)
	if err != nil {
		t.Fatal(err)
	}
	if added != len(ettleHooks()) {
		t.Errorf("first install should add every hook: got %d, want %d", added, len(ettleHooks()))
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

	again, err := installHooks(path)
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
	if added, err := installHooks(fresh); err != nil || added != len(ettleHooks()) {
		t.Fatalf("a missing settings file should be created: added=%d err=%v", added, err)
	}

	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := installHooks(bad); err == nil {
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
	blocked := renderNextSteps("crew", "justin", false, []check{{ok: false, required: true, name: "LINEAR_API_KEY"}})
	if strings.Contains(blocked, "ettle horizon") {
		t.Errorf("don't hand someone a command that cannot work yet:\n%s", blocked)
	}
	if !strings.Contains(blocked, "LINEAR_SETUP.md") {
		t.Errorf("point at the doc that fixes it:\n%s", blocked)
	}
	ok := renderNextSteps("crew", "justin", true, []check{{ok: true, required: true, name: "LINEAR_API_KEY"}})
	for _, want := range []string{"ettle horizon --me justin", "ettle init crew"} {
		if !strings.Contains(ok, want) {
			t.Errorf("a working setup should lead with %q:\n%s", want, ok)
		}
	}
}
