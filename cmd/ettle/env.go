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

// loadUserEnv fills any UNSET variable from <config>/ettle/env. An explicit
// environment variable always wins, so this can never override what a caller set —
// it only supplies what is missing. Missing or unreadable file: silently nothing,
// because this is a convenience, not a requirement.
func loadUserEnv() {
	dir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	f, err := os.Open(filepath.Join(dir, "ettle", "env"))
	if err != nil {
		return
	}
	defer f.Close()

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
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if k == "" || os.Getenv(k) != "" {
			continue
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
