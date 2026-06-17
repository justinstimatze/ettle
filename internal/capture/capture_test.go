package capture

import (
	"strings"
	"testing"
)

func TestReadKitSession(t *testing.T) {
	s, err := Read("../../testdata/sessions/kit.jsonl")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if s.Branch != "auth-jwt" {
		t.Errorf("branch = %q, want auth-jwt", s.Branch)
	}
	// Prompts (stated intent) captured; the cookie-removal decision is the
	// load-bearing one for the cross-person collision.
	joined := strings.Join(s.Prompts, " | ")
	for _, want := range []string{"migrating our auth", "remove the old cookie-session", "Thursday"} {
		if !strings.Contains(joined, want) {
			t.Errorf("prompts missing %q; got %q", want, joined)
		}
	}
	// Edits = committed work (Write/Edit); commands = bash verbs.
	if !contains(s.Edits, "jwt.go") || !contains(s.Edits, "middleware.go") {
		t.Errorf("edits = %v, want jwt.go + middleware.go", s.Edits)
	}
	if !contains(s.Cmds, "go") || !contains(s.Cmds, "git") {
		t.Errorf("cmds = %v, want go + git", s.Cmds)
	}
}

func TestReadSkipsExploration(t *testing.T) {
	s, err := Read("../../testdata/sessions/sol.jsonl")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	// sol's session Reads internal/auth/middleware.go — that's the agent
	// exploring, NOT sol's committed work, so it must not appear as an edit.
	if contains(s.Edits, "middleware.go") {
		t.Errorf("a Read of middleware.go leaked into edits: %v", s.Edits)
	}
	if !contains(s.Edits, "login_client.go") {
		t.Errorf("edits = %v, want login_client.go (a Write)", s.Edits)
	}
	// The cookie-stability assumption — the other half of the collision — is kept.
	if !strings.Contains(strings.Join(s.Prompts, " "), "cookie-based /login stays stable") {
		t.Errorf("prompts missing the cookie-stability assumption: %v", s.Prompts)
	}
}

func TestCleanPromptDropsNoise(t *testing.T) {
	for _, n := range []string{
		"<local-command-stdout>compacted</local-command-stdout>",
		"<command-name>/compact</command-name>",
		"[Request interrupted by user]",
		"   ",
	} {
		if got := cleanPrompt(n); got != "" {
			t.Errorf("cleanPrompt(%q) = %q, want empty (noise)", n, got)
		}
	}
	// A real prompt with a trailing system-reminder keeps the human part only.
	got := cleanPrompt("ship the issuer by Thursday\n<system-reminder>blah blah</system-reminder>")
	if got != "ship the issuer by Thursday" {
		t.Errorf("cleanPrompt stripped reminder wrong: %q", got)
	}
}

func TestBashVerb(t *testing.T) {
	cases := []struct{ cmd, want string }{
		{"go test ./...", "go"},
		{"sudo env FOO=bar npm run build", "npm"},
		{"cd internal/auth && go build", "go"},
		{"git commit -m x 2>&1 | head", "git"},
		{"", ""},
	}
	for _, c := range cases {
		if got := bashVerb(map[string]any{"command": c.cmd}); got != c.want {
			t.Errorf("bashVerb(%q) = %q, want %q", c.cmd, got, c.want)
		}
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
