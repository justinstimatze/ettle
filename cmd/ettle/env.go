package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// ettle's keys have to be readable by the HOOKS, which inherit whatever
// environment the Claude Code session was launched with — so "export it in your
// shell" is the only thing that works, and it is a change to a file ettle has no
// business editing. `<config>/ettle/env` is the alternative: one file, outside
// every repo, read by every command and therefore by every hook.
//
// It also fixes an asymmetry that was its own papercut: `ANTHROPIC_API_KEY` fell
// back to a `.env` in the working directory but `LINEAR_API_KEY` had no fallback at
// all, so a room configured in one directory silently failed from another.

// fromUserEnv records which variables loadUserEnv supplied from the global file.
// It exists so loadProfileEnv can tell "the global file set this" (overridable) from
// "the caller set this" (never overridable) — os.Getenv cannot distinguish them, and
// without the distinction a room's profile could never override the global default,
// which is the entire point of having profiles.
var fromUserEnv = map[string]bool{}

// readEnvFile parses the tiny KEY=VALUE format both env layers share. A missing or
// unreadable file yields nothing, because these are a convenience, not a requirement.
// Shared so the global file and a profile can never drift in what they accept.
func readEnvFile(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return out
}

// loadUserEnv fills any UNSET variable from <config>/ettle/env. An explicit
// environment variable always wins, so this can never override what a caller set —
// it only supplies what is missing.
func loadUserEnv() {
	for k, v := range readEnvFile(userEnvPath()) {
		if os.Getenv(k) != "" {
			continue
		}
		if os.Setenv(k, v) == nil {
			fromUserEnv[k] = true
		}
	}
}

// loadProfileEnv layers a named key profile over the global file, for the person who
// works across more than one Linear workspace: a Linear member key is workspace-scoped,
// so one global key cannot serve two workspaces, and the room a project belongs to is
// what should decide which key it uses.
//
// It sets a variable when it is unset OR when the global file is what supplied it.
// The resulting precedence is: real environment > named profile > global file. A
// caller's explicit export still wins over everything, which keeps loadUserEnv's
// contract intact.
//
// An unnamed or absent profile does nothing, so a project with no `profile` line
// behaves exactly as it did before this existed.
func loadProfileEnv(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	for k, v := range readEnvFile(profileEnvPath(name)) {
		if os.Getenv(k) != "" && !fromUserEnv[k] {
			continue // the caller set this one; a profile never overrides that
		}
		_ = os.Setenv(k, v)
	}
}

// userEnvPath is where loadUserEnv reads from — reported by `ettle init` so nobody
// has to guess where the keys go.
func userEnvPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "ettle", "env")
}

// profileEnvPath is where a named profile's keys live. Reported by `ettle init` for
// the same reason userEnvPath is: a path you have to guess is a path you get wrong.
func profileEnvPath(name string) string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "ettle", "env.d", strings.TrimSpace(name))
}
