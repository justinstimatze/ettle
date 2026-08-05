package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/transport"
)

// TestRoomInitLocalAndBus drives the room command end to end with a local-only
// room (no remote): init writes a usable config + seeded repo, and the room's
// resolved transport round-trips an envelope. git must be on PATH; skipped
// otherwise. UserConfigDir is redirected to a temp dir via XDG_CONFIG_HOME.
func TestRoomInitLocalAndBus(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // os.UserConfigDir honors this on Linux

	if err := roomInit([]string{"--as", "Alice Smith", "--name", "crew"}); err != nil {
		t.Fatalf("roomInit: %v", err)
	}

	rc, err := loadRoom("crew")
	if err != nil {
		t.Fatalf("loadRoom: %v", err)
	}
	if rc.Agent != "Alice_Smith" { // sanitized to a valid leat id
		t.Fatalf("agent id not sanitized: %q", rc.Agent)
	}
	if rc.Remote != "" {
		t.Fatalf("local room should have no remote, got %q", rc.Remote)
	}

	bus, err := roomBus("crew")
	if err != nil {
		t.Fatalf("roomBus: %v", err)
	}
	defer bus.Close()

	ctx := context.Background()
	env := transport.Envelope{
		Participant: "Alice Smith",
		Atoms: []ettlemesh.Atom{
			{From: "alice", Typ: ettlemesh.Intent, Subject: "x", Content: "wiring the room", Confidence: 0.9},
		},
	}
	if err := bus.Publish(ctx, env); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	got, err := bus.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(got) != 1 || len(got[0].Atoms) != 1 || got[0].Atoms[0].Content != "wiring the room" {
		t.Fatalf("round-trip through the room failed: %+v", got)
	}
}

// TestRenderRoomStatus pins the presence view: participants sorted, atoms framed
// by type ("working on" for intent), and per-person freshness from EmittedAt.
func TestRenderRoomStatus(t *testing.T) {
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	envs := []transport.Envelope{
		{
			Participant: "bob", Role: "checkout",
			EmittedAt: now.Add(-3 * 24 * time.Hour).Format(time.RFC3339),
			Atoms: []ettlemesh.Atom{
				{Typ: ettlemesh.Dependency, Subject: "pricing", Content: "calls pricing in-process"},
			},
		},
		{
			Participant: "alice", Role: "user-service",
			EmittedAt: now.Add(-30 * time.Minute).Format(time.RFC3339),
			Atoms: []ettlemesh.Atom{
				{Typ: ettlemesh.Intent, Subject: "rename", Content: "renaming GetUser"},
			},
		},
	}
	out := renderRoomStatus("crew", envs, nil, nil, now)

	if !strings.Contains(out, `room "crew" — 2 present`) {
		t.Fatalf("header/count missing:\n%s", out)
	}
	// alice (30m ago) must sort before bob (3d ago).
	if strings.Index(out, "alice") > strings.Index(out, "bob") {
		t.Fatalf("participants not sorted:\n%s", out)
	}
	// Coarse on purpose — a per-minute age across a team is a working-patterns feed —
	// and never a presence claim, because everyone here is offline right now.
	if !strings.Contains(out, "alice (user-service) · recently") {
		t.Fatalf("freshness should be a coarse bucket:\n%s", out)
	}
	if strings.Contains(out, "active") {
		t.Errorf("nothing on a session-end bus warrants calling anyone active:\n%s", out)
	}
	if strings.Contains(out, "30m") || strings.Contains(out, "0h ago") {
		t.Errorf("minute/hour resolution leaks when each person works:\n%s", out)
	}
	if !strings.Contains(out, "3d ago") {
		t.Fatalf("freshness '3d ago' missing for bob:\n%s", out)
	}
	if !strings.Contains(out, "working on:") || !strings.Contains(out, "renaming GetUser") {
		t.Fatalf("intent must render as 'working on':\n%s", out)
	}
	if !strings.Contains(out, "depends on:") {
		t.Fatalf("dependency must render as 'depends on':\n%s", out)
	}

	// Empty room gives the join hint, not a bare header.
	if empty := renderRoomStatus("crew", nil, nil, nil, now); !strings.Contains(empty, "nobody has published yet") {
		t.Fatalf("empty room should hint how to publish:\n%s", empty)
	}
}

// TestRoomInitFlagAfterURL regression-guards the flag-stops-at-positional bug:
// `room init <url> --as bob` must honor --as, not fall back to $USER. Go's flag
// package stops parsing at the first non-flag token, so this only works because
// roomInit lifts the URL out first (liftURL). Uses a local bare repo as the URL.
func TestRoomInitFlagAfterURL(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("USER", "someoneelse") // the default that must NOT win

	bare := t.TempDir() + "/crew.git"
	if out, err := exec.Command("git", "init", "--bare", "-b", "main", bare).CombinedOutput(); err != nil {
		t.Fatalf("init bare: %v\n%s", err, out)
	}
	// Seed the bare so clone yields a born HEAD (roomInit's ensureSeed pushes).
	if err := roomInit([]string{bare, "--as", "bob", "--name", "crew"}); err != nil {
		t.Fatalf("roomInit: %v", err)
	}
	rc, err := loadRoom("crew")
	if err != nil {
		t.Fatalf("loadRoom: %v", err)
	}
	if rc.Agent != "bob" {
		t.Fatalf("--as after URL ignored: agent = %q, want bob", rc.Agent)
	}
	if rc.Remote != "origin" {
		t.Fatalf("cloned room should have remote origin, got %q", rc.Remote)
	}
}

func TestLiftURL(t *testing.T) {
	cases := []struct {
		args     []string
		wantURL  string
		wantRest []string
	}{
		{[]string{"git@h:crew.git", "--as", "bob"}, "git@h:crew.git", []string{"--as", "bob"}},
		{[]string{"--as", "bob", "git@h:crew.git"}, "", []string{"--as", "bob", "git@h:crew.git"}},
		{[]string{"--as", "bob"}, "", []string{"--as", "bob"}},
		{nil, "", nil},
	}
	for _, c := range cases {
		url, rest := liftURL(c.args)
		if url != c.wantURL {
			t.Errorf("liftURL(%v) url = %q, want %q", c.args, url, c.wantURL)
		}
		if strings.Join(rest, " ") != strings.Join(c.wantRest, " ") {
			t.Errorf("liftURL(%v) rest = %v, want %v", c.args, rest, c.wantRest)
		}
	}
}

func TestRoomNameFromURL(t *testing.T) {
	cases := map[string]string{
		"git@github.com:crew/standup-room.git":     "standup-room",
		"https://github.com/crew/standup-room.git": "standup-room",
		"https://gitlab.com/team/ettle-room/":      "ettle-room",
	}
	for url, want := range cases {
		if got := roomNameFromURL(url); got != want {
			t.Errorf("roomNameFromURL(%q) = %q, want %q", url, got, want)
		}
	}
}

func TestIssueIdentsAndLinkFooter(t *testing.T) {
	envs := []transport.Envelope{
		{Participant: "ivo", Atoms: []ettlemesh.Atom{
			{Typ: ettlemesh.Intent, Subject: "IWS-33 rollout", Content: "picking up IWS-33, blocked on CUR-97"},
			{Typ: ettlemesh.Dependency, Subject: "encoding", Content: "needs UTF-8 output and IWS-33 merged"},
		}},
	}
	got := issueIdents(envs)
	// Order of appearance, deduped. UTF-8 rides along on purpose: the resolver drops
	// what the workspace doesn't own, so a loose regex costs a filter term, not a
	// wrong link, while a strict one would miss real ticket prefixes.
	want := []string{"IWS-33", "CUR-97", "UTF-8"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("issueIdents = %v, want %v", got, want)
	}

	// Only what came back from the workspace is rendered, sorted, with the title —
	// an identifier alone doesn't tell the reader whether the link is worth a click.
	out := renderIssueLinks([]transport.IssueRef{
		{Identifier: "IWS-40", Title: "Second", URL: "https://linear.app/x/issue/IWS-40"},
		{Identifier: "IWS-33", Title: "First", URL: "https://linear.app/x/issue/IWS-33"},
	})
	if strings.Index(out, "IWS-33") > strings.Index(out, "IWS-40") {
		t.Errorf("issue links should be sorted:\n%s", out)
	}
	for _, want := range []string{"issues this work refers to:", "First", "https://linear.app/x/issue/IWS-33"} {
		if !strings.Contains(out, want) {
			t.Errorf("footer missing %q:\n%s", want, out)
		}
	}
	if renderIssueLinks(nil) != "" {
		t.Error("a room whose atoms name no ticket gets no footer at all")
	}
}

func TestIssueIdentsCapsRunawayInput(t *testing.T) {
	var atoms []ettlemesh.Atom
	for i := 0; i < 60; i++ {
		atoms = append(atoms, ettlemesh.Atom{Subject: fmt.Sprintf("ABC-%d", i+1)})
	}
	if got := issueIdents([]transport.Envelope{{Atoms: atoms}}); len(got) != 20 {
		t.Errorf("one noisy session must not turn the view into a link dump: got %d", len(got))
	}
}
