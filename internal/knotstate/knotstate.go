// Package knotstate holds the small per-room facts about coordination knots that
// must be shared between the CLI and the MCP server: which knots have been
// escalated to Linear, and which the human has muted (marked handled/not-real).
// Both surfaces key a knot the SAME way — so a knot escalated by `ettle escalate`
// is recognized as escalated by the MCP `ettle_horizon`, and a knot muted via
// `ettle_respond` is skipped by both horizon and escalate. Stored as a JSON string
// array per room under <config>/ettle/<store>/<room>.json.
package knotstate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/justinstimatze/ettle/internal/transport"
)

// Store names (the on-disk subdir per kind of fact).
const (
	Escalated = "emit"  // knots posted to Linear (escalate's store)
	Muted     = "muted" // knots the human marked handled/not-real — stop surfacing
)

// Key is the canonical, wording-independent identity of a coordination knot: its
// kind plus its distinct parties (lowercased, trimmed, sorted), joined "kind|a+b".
// This is the ONE key function; every store and every tag goes through it so a knot
// is the same knot across the CLI and the MCP server regardless of wording drift.
func Key(kind string, parties []string) string {
	ps := make([]string, 0, len(parties))
	seen := map[string]bool{}
	for _, p := range parties {
		n := strings.ToLower(strings.TrimSpace(p))
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		ps = append(ps, n)
	}
	sort.Strings(ps)
	return kind + "|" + strings.Join(ps, "+")
}

func storePath(store, room string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	return filepath.Join(dir, "ettle", store, transport.SanitizeID(room)+".json"), nil
}

// Load reads a per-room key set (a missing file is an empty set, not an error).
func Load(store, room string) (map[string]bool, error) {
	p, err := storePath(store, room)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, err
	}
	var keys []string
	if err := json.Unmarshal(data, &keys); err != nil {
		return nil, fmt.Errorf("knotstate %s: corrupt: %w", p, err)
	}
	set := make(map[string]bool, len(keys))
	for _, k := range keys {
		set[k] = true
	}
	return set, nil
}

// Save writes the set back, sorted, for a stable file.
func Save(store, room string, set map[string]bool) error {
	p, err := storePath(store, room)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	data, _ := json.Marshal(keys)
	return os.WriteFile(p, data, 0o644)
}

// Add marks one key in a store (load-add-save); a no-op if already present.
func Add(store, room, key string) error {
	set, err := Load(store, room)
	if err != nil {
		return err
	}
	if set[key] {
		return nil
	}
	set[key] = true
	return Save(store, room, set)
}
