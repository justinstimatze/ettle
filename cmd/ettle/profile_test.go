package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Every failure mode in the profile/workspace machinery is SILENT — a profile that
// never loads, a workspace written then erased, a lookup under the wrong key. Each
// one leaves the build green and the feature inert, so these assertions are the only
// thing standing between "implemented" and "implemented and actually working."

// writeProfile lays out a config dir with a global env file and any named profiles,
// and points the process at it.
func writeProfile(t *testing.T, global string, profiles map[string]string) {
	t.Helper()
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	if err := os.MkdirAll(filepath.Join(cfg, "ettle", "env.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if global != "" {
		if err := os.WriteFile(filepath.Join(cfg, "ettle", "env"), []byte(global), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range profiles {
		if err := os.WriteFile(filepath.Join(cfg, "ettle", "env.d", name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// loadUserEnv accumulates into package state; a stale entry from an earlier test
	// would let a profile overwrite something a later test set explicitly.
	for k := range fromUserEnv {
		delete(fromUserEnv, k)
	}
}

func TestProfileOverridesTheGlobalFileButNotTheCaller(t *testing.T) {
	writeProfile(t,
		"LINEAR_API_KEY=global_key\nLINEAR_TEAM_ID=global_team\n",
		map[string]string{"work": "LINEAR_API_KEY=work_key\nANTHROPIC_API_KEY=from_profile\n"})
	// The caller set this one explicitly; no file may override it.
	t.Setenv("ANTHROPIC_API_KEY", "from_caller")
	t.Setenv("LINEAR_API_KEY", "")
	t.Setenv("LINEAR_TEAM_ID", "")

	loadUserEnv()
	loadProfileEnv("work")

	// This is the assertion the whole feature rests on: the global file supplied a key,
	// and the profile has to be able to replace it. os.Getenv alone cannot tell "the
	// global file set this" from "the caller set this", which is why fromUserEnv exists.
	if got := os.Getenv("LINEAR_API_KEY"); got != "work_key" {
		t.Errorf("the profile must beat the global file, got %q", got)
	}
	if got := os.Getenv("ANTHROPIC_API_KEY"); got != "from_caller" {
		t.Errorf("an explicit export must beat the profile, got %q", got)
	}
	// A key the profile does not mention keeps the global value, so a profile can
	// carry only what differs between workspaces.
	if got := os.Getenv("LINEAR_TEAM_ID"); got != "global_team" {
		t.Errorf("an unmentioned key should keep the global value, got %q", got)
	}
}

func TestAbsentOrUnnamedProfileChangesNothing(t *testing.T) {
	writeProfile(t, "LINEAR_API_KEY=global_key\n", nil)
	t.Setenv("LINEAR_API_KEY", "")
	loadUserEnv()

	loadProfileEnv("")       // no profile line at all — today's behavior
	loadProfileEnv("absent") // named, but no such file
	if got := os.Getenv("LINEAR_API_KEY"); got != "global_key" {
		t.Errorf("a missing profile must leave the global keys alone, got %q", got)
	}
}

func TestActiveProfilePrefersTheEnvironmentOverTheCommittedLine(t *testing.T) {
	// .ettle-room is SHARED, so a teammate may name their profile differently. The
	// per-machine override has to win or they cannot use the file at all.
	t.Setenv(profileEnvVar, "mine")
	if got := activeProfile(roomFile{Profile: "theirs"}); got != "mine" {
		t.Errorf("ETTLE_PROFILE should win, got %q", got)
	}
	t.Setenv(profileEnvVar, "")
	if got := activeProfile(roomFile{Profile: "theirs"}); got != "theirs" {
		t.Errorf("with no override the committed line applies, got %q", got)
	}
	if got := activeProfile(roomFile{}); got != "" {
		t.Errorf("no profile anywhere is not an error, got %q", got)
	}
}

func TestParseRoomFileReadsProfileAndStillIgnoresUnknownKeys(t *testing.T) {
	rf := parseRoomFile("# ettle\nroom = linear://crew\nprofile = work\nunknown = x\n")
	if rf.Spec != "linear://crew" || rf.Profile != "work" {
		t.Errorf("got %+v, want spec linear://crew and profile work", rf)
	}
	// Unknown keys stay ignored: the file is a pointer, not a config surface, and
	// failing a hook over a typo would be worse than skipping it.
	if bare := parseRoomFile("linear://crew\nprofile = side\n"); bare.Spec != "linear://crew" || bare.Profile != "side" {
		t.Errorf("the bare-room form should still carry a profile, got %+v", bare)
	}
	// Identity remains deliberately absent — a committed `me` would publish a
	// teammate's atoms under someone else's name.
	if only := parseRoomFile("me = alice\n"); only.Spec != "" || only.Profile != "" {
		t.Errorf("an identity-only file should yield nothing, got %+v", only)
	}
}

func TestRoomFunnelsLoadTheProjectProfile(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func()
	}{
		// Both funnels, and both on the path where a room was passed EXPLICITLY: a
		// typed --room does not make this a different project, so it still wants this
		// project's keys. That early return is where the load is easiest to omit.
		{"applyRoomFile with explicit flags", func() { applyRoomFile("", "linear://other") }},
		{"applyRoomFile from the file", func() { applyRoomFile("", "") }},
		{"linearRoomFor with an explicit room", func() { linearRoomFor("other") }},
		{"linearRoomFor from the file", func() { linearRoomFor("") }},
		// The keys-without-the-room funnel, for `ettle capture` and `ettle teams`.
		// capture was on NEITHER funnel until this was found by running the command
		// rather than a test: the hook path works because capture-hook calls
		// applyRoomFile and the detached child inherits the environment, so a typed
		// `ettle capture --transport linear://<room>` was the only way to see it.
		{"loadProjectProfile from the file", func() { loadProjectProfile("") }},
		{"loadProjectProfile with an explicit --profile", func() { loadProjectProfile("work") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeProfile(t, "LINEAR_API_KEY=global_key\n",
				map[string]string{"work": "LINEAR_API_KEY=work_key\n"})
			t.Setenv(profileEnvVar, "")
			t.Setenv("LINEAR_API_KEY", "")
			loadUserEnv()

			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, roomFileName),
				[]byte("room = linear://crew\nprofile = work\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Chdir(dir)

			tc.call()
			if got := os.Getenv("LINEAR_API_KEY"); got != "work_key" {
				t.Errorf("this funnel did not load the project's profile, got %q", got)
			}
		})
	}
}

func TestSaveIdentityPreservesTheRecordedWorkspace(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const spec = "linear://crew"

	if err := saveOrg(spec, "org_acme", "Acme"); err != nil {
		t.Fatal(err)
	}
	// saveIdentity runs AFTER the workspace is recorded in `ettle init`. It used to
	// marshal a fresh map of its own two fields, which erased the workspace nine lines
	// after it was written — leaving the guard permanently inert with the build green.
	if err := saveIdentity(spec, "alice"); err != nil {
		t.Fatal(err)
	}

	if got := loadOrg(spec); got.ID != "org_acme" || got.Name != "Acme" {
		t.Errorf("saveIdentity erased the workspace: got %+v", got)
	}
	if got := loadIdentity(spec); got != "alice" {
		t.Errorf("identity should survive too, got %q", got)
	}

	// And the mirror image: recording a workspace must not drop the identity.
	if err := saveOrg(spec, "org_acme", "Acme"); err != nil {
		t.Fatal(err)
	}
	if got := loadIdentity(spec); got != "alice" {
		t.Errorf("saveOrg erased the identity, got %q", got)
	}
}

func TestLoadOrgIsEmptyWithNoRecord(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// The no-expectation case every pre-existing install starts in, which is what
	// makes the guard additive rather than a breaking change.
	if got := loadOrg("linear://never-seen"); got.ID != "" {
		t.Errorf("an unknown room has no expectation, got %+v", got)
	}
}

func TestIdentityFileKeepsBothAuthorsFields(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const spec = "linear://crew"
	if err := saveIdentity(spec, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := saveOrg(spec, "org_acme", "Acme"); err != nil {
		t.Fatal(err)
	}

	path, err := identityPath(spec)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var v identityFile
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	if v.Me != "alice" || v.Room != spec || v.Org == nil || v.Org.ID != "org_acme" {
		t.Errorf("the record should carry every field on disk, got %+v", v)
	}
}

func TestResolveTeamIDSeesTheProfileValue(t *testing.T) {
	writeProfile(t, "LINEAR_TEAM_ID=global_team\n",
		map[string]string{"work": "LINEAR_TEAM_ID=work_team\n"})
	t.Setenv("LINEAR_TEAM_ID", "")
	loadUserEnv()

	// Reading this as a flag DEFAULT happens before any room resolves, so it would
	// capture "global_team" and the project's own workspace would never get a look in.
	loadProfileEnv("work")
	if got := resolveTeamID(""); got != "work_team" {
		t.Errorf("the team id should come from the project's profile, got %q", got)
	}
	if got := resolveTeamID("explicit"); got != "explicit" {
		t.Errorf("an explicit --team still wins, got %q", got)
	}
}

func TestOtherWorkspacesSeesOnlyTheNeighbours(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, r := range []struct{ spec, id, name string }{
		{"linear://crew", "org_acme", "Acme"},
		{"linear://infra", "org_acme", "Acme"}, // same workspace, must not double-count
		{"linear://side", "org_side", "Side Project"},
	} {
		if err := saveIdentity(r.spec, "alice"); err != nil {
			t.Fatal(err)
		}
		if err := saveOrg(r.spec, r.id, r.name); err != nil {
			t.Fatal(err)
		}
	}

	// A non-empty result is the positive evidence `ettle init` warns on: this machine
	// demonstrably works across workspaces, so a project naming no profile is sharing
	// one key with all of them.
	got := otherWorkspaces("org_acme")
	if len(got) != 1 || got[0] != "Side Project" {
		t.Errorf("got %v, want just [Side Project]", got)
	}
	if only := otherWorkspaces("org_side"); len(only) != 1 || only[0] != "Acme" {
		t.Errorf("got %v, want just [Acme]", only)
	}
	// A machine with one workspace must stay quiet — a warning everyone sees is a
	// warning nobody reads.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := saveOrg("linear://crew", "org_acme", "Acme"); err != nil {
		t.Fatal(err)
	}
	if quiet := otherWorkspaces("org_acme"); len(quiet) != 0 {
		t.Errorf("a single-workspace machine should produce no warning, got %v", quiet)
	}
}

// `.ettle-room` is COMMITTED and travels with a repository, and the Claude Code hooks
// run applyRoomFile in every project on the machine with no report shown. So a
// profile name is attacker-controllable input from any repo you merely clone, and
// filepath.Join CLEANS ".." rather than confining it — without a check, `profile =
// ../../../x` reads an arbitrary file as KEY=VALUE and os.Setenv's the result into
// every ettle process, including the detached children that hold real API keys.
func TestProfileNameCannotEscapeTheConfigDirectory(t *testing.T) {
	for _, bad := range []string{
		"../../PLANTED", "../evil", "a/b", `a\b`, "..", ".", ".hidden", "", "   ",
	} {
		if validProfileName(bad) {
			t.Errorf("%q must be rejected as a profile name", bad)
		}
		if got := profileEnvPath(bad); got != "" {
			t.Errorf("%q resolved to a path (%s) — traversal is reachable", bad, got)
		}
	}
	for _, ok := range []string{"work", "dayjob", "side-project", "a_b.c"} {
		if !validProfileName(ok) {
			t.Errorf("%q is a plain name and should be accepted", ok)
		}
	}
}

func TestLoadProfileEnvIgnoresATraversalName(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	// A file the attacker points at, outside env.d.
	if err := os.WriteFile(filepath.Join(cfg, "PLANTED"), []byte("HTTPS_PROXY=http://attacker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HTTPS_PROXY", "")

	loadProfileEnv("../../PLANTED")

	if got := os.Getenv("HTTPS_PROXY"); got != "" {
		t.Fatalf("a committed .ettle-room injected HTTPS_PROXY=%q into every ettle process", got)
	}
}
