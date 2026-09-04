package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The key table is written out twice — once in README.md for someone deciding whether
// to try this, once in docs/LINEAR_SETUP.md for someone setting it up. Duplication is
// the right call for those two audiences, but it drifts, and it drifted: both said
// ANTHROPIC_API_KEY was "one per room" long after envChecks started marking it
// required for every install. A person following the README told a teammate they
// needed no key, and that teammate's `ettle init` then failed on a required check.
//
// So the code is the arbiter. These tests do not compare the two documents to each
// other — that would just pin one wrong answer to another. They compare each document
// to what runInit actually enforces.

// docPaths are the human-facing files that describe the keys, relative to this
// package. Tests run with the package directory as the working directory.
var docPaths = []string{
	filepath.Join("..", "..", "README.md"),
	filepath.Join("..", "..", "docs", "LINEAR_SETUP.md"),
	filepath.Join("..", "..", "hooks", "README.md"),
}

// sharedKeyClaims are ways of saying "somebody else's key covers you". Any of them
// applied to a key the code marks required is a promise the code will break.
var sharedKeyClaims = []string{
	"one per room",
	"one key per room",
	"whoever reconciles",
}

// sharedClaimIn returns the first "somebody else's key covers you" phrase the line
// makes about key, or "" when it makes none.
func sharedClaimIn(line, key string) string {
	if !strings.Contains(line, key) {
		return ""
	}
	lower := strings.ToLower(line)
	for _, claim := range sharedKeyClaims {
		if strings.Contains(lower, claim) {
			return claim
		}
	}
	return ""
}

func TestDocsDoNotCallARequiredKeyShared(t *testing.T) {
	// envChecks is the same function `ettle init` reports from, so this asserts against
	// the enforcement itself rather than a restatement of it.
	var required []string
	for _, c := range envChecks() {
		if c.required {
			required = append(required, c.name)
		}
	}

	for _, path := range docPaths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, line := range strings.Split(string(body), "\n") {
			for _, key := range required {
				claim := sharedClaimIn(line, key)
				if claim == "" {
					continue
				}
				t.Errorf("%s describes %s as %q, but envChecks marks it required for every install "+
					"— a reader will tell a teammate they need no key and that teammate's `ettle init` will fail:\n  %s",
					filepath.Base(path), key, claim, strings.TrimSpace(line))
			}
		}
	}
}

// The multi-workspace section is what the README's setup step links to, and a link to
// a heading that has been renamed away is worse than no link: it looks authoritative
// and lands nowhere.
func TestReadmeLinksToASectionThatExists(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	setup, err := os.ReadFile(filepath.Join("..", "..", "docs", "LINEAR_SETUP.md"))
	if err != nil {
		t.Fatal(err)
	}
	const anchor = "LINEAR_SETUP.md#more-than-one-workspace"
	if !strings.Contains(string(readme), anchor) {
		t.Fatalf("the README setup step should point at the multi-workspace section (%s) — "+
			"a machine with two Linear workspaces gets a silently wrong room without it", anchor)
	}
	if !strings.Contains(string(setup), "## More than one workspace") {
		t.Error("the heading the README links to is gone; the link now lands nowhere")
	}
}

// The profile flag is the answer to a question nobody knows to ask, so it has to be
// visible from the command a person actually runs rather than only from --help.
func TestProfileIsDiscoverableFromTheTopLevelUsage(t *testing.T) {
	main, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(main), "--profile <name>") {
		t.Error("the top-level usage should mention --profile: on a room's FIRST init nothing " +
			"is recorded yet, so the wrong-workspace guard has no expectation to check and this " +
			"line is the only warning a person gets")
	}
}

// Choosing a room is the one decision a team has to make together, and getting it
// wrong fails silently: two people who type `crew` and `Crew` each sit in a room
// seeing only themselves, which looks like ettle having nothing to say. The guidance
// has to be in front of a person at all three moments they might be deciding — the
// pitch, the setup doc, and the usage text they hit when they run init with no room.
func TestChoosingARoomIsExplainedWhereItIsDecided(t *testing.T) {
	for _, tc := range []struct{ path, want, why string }{
		{filepath.Join("..", "..", "README.md"), "is the room, and it is the one thing you have to agree on",
			"the setup step is where most people meet the word `room` for the first time"},
		{filepath.Join("..", "..", "docs", "LINEAR_SETUP.md"), "## First: what a room is, and how to pick one",
			"this is the page init sends people to, and it opened by explaining keys for a room it never defined"},
		{"init.go", "A room is the space a GROUP coordinates in",
			"the usage error is what a person sees at the exact moment they have to pick a name"},
	} {
		body, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatalf("read %s: %v", tc.path, err)
		}
		if !strings.Contains(string(body), tc.want) {
			t.Errorf("%s no longer explains how to choose a room (looked for %q) — %s",
				filepath.Base(tc.path), tc.want, tc.why)
		}
	}
}
