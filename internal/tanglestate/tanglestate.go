// Package tanglestate holds the small per-room facts about coordination tangles that
// must be shared between the CLI and the MCP server: which tangles have been
// escalated to Linear, and which the human has muted (marked handled/not-real).
// Both surfaces key a tangle the SAME way — so a tangle escalated by `ettle escalate`
// is recognized as escalated by the MCP `ettle_horizon`, and a tangle muted via
// `ettle_respond` is skipped by both horizon and escalate. Stored as a JSON string
// array per room under <config>/ettle/<store>/<room>.json.
package tanglestate

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
	Escalated = "emit"  // tangles posted to Linear (escalate's store)
	Muted     = "muted" // tangles the human marked handled/not-real — stop surfacing
	// Confirmed holds tangles the human judged `real`. It exists so that verdict
	// changes something they can see. Muting pays off immediately — the nuisance
	// stops — while confirming used to leave the tangle exactly as it was, asked
	// about again every session. That asymmetry is a sampling bias in the only
	// ground truth the calibration loop will ever have: `ettle calibrate` can bound
	// the false-alarm rate and cannot estimate the hit rate, because nobody is paid
	// to record a hit. A confirmed tangle still surfaces (it is a live conflict) but
	// stops being asked about, which is the symmetric payoff: mute means stop showing
	// me this, confirm means stop asking me about this.
	Confirmed = "confirmed"
)

// Key is the canonical, wording-independent identity of a coordination tangle: its
// kind plus its distinct parties (lowercased, trimmed, sorted), joined "kind|a+b".
// This is the ONE key function; every store and every tag goes through it so a tangle
// is the same tangle across the CLI and the MCP server regardless of wording drift.
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
	// The kind normalizes like the parties do. The detector already emits lowercase
	// kinds, so no stored key moves; what this fixes is a human typing "Duplication"
	// at `ettle mute` and silencing a tangle that does not exist.
	return strings.ToLower(strings.TrimSpace(kind)) + "|" + strings.Join(ps, "+")
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
		return nil, fmt.Errorf("tanglestate %s: corrupt: %w", p, err)
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

// Remove unmarks one key. Reports whether it was there, because a caller telling a
// human "unmuted" when nothing changed is worse than saying nothing happened.
// Muting has to be reversible: a mute that can only be added is a trap, since the
// human who silenced the wrong tangle has no way back.
func Remove(store, room, key string) (bool, error) {
	set, err := Load(store, room)
	if err != nil {
		return false, err
	}
	if !set[key] {
		return false, nil
	}
	delete(set, key)
	return true, Save(store, room, set)
}
