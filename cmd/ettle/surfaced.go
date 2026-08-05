package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/mcpserver"
	"github.com/justinstimatze/ettle/internal/tanglestate"
	"github.com/justinstimatze/ettle/internal/transport"
)

// The recurrence a tangle had when it was shown is the feature every cut point
// thresholds on, and until now only the MCP server could attach it to a verdict: it
// holds the surfaced set in memory, so an agent answering the horizon it just read
// records a learnable row. The CLI is a fresh process each time and had nothing to
// look the tangle up in, so `ettle confirm` and `ettle mute` wrote rows with the kind
// and zero recurrence.
//
// That is backwards from where the rows come from. The default install is hooks-only
// — `ettle init --install-hooks` wires the SessionStart injector and no MCP server —
// so the surface that produces the MOST verdicts was producing the ones `ettle
// calibrate` counts and can never use. This is the CLI's equivalent of the server's
// in-memory map, persisted because the process is not.
//
// Keyed by room rather than by person: which tangles you SEE depends on --me, what
// their recurrence WAS does not. It is a property of the reconcile, so two people
// reading the same room read the same features.
//
// Replaced on every horizon run, never merged. A tangle that stopped surfacing should
// not leave a feature behind for a later verdict to pick up — a stale recurrence
// presented as the surfaced one is exactly the fabrication the zero was protecting
// against.

type surfacedFeature struct {
	Kind    string `json:"kind"`
	Votes   int    `json:"votes"`
	Samples int    `json:"samples"`
	Firm    bool   `json:"firm"`
	At      string `json:"at"` // RFC3339 UTC: when the reconcile that surfaced it ran
}

func surfacedPath(stateKey string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	return filepath.Join(dir, "ettle", "surfaced", transport.SanitizeID(stateKey)+".json"), nil
}

// writeSurfaced records the features of everything this horizon actually showed.
// Held-back tangles are deliberately absent: the drop floor means they were never
// shown, so no verdict can reference them and a feature for one would describe a
// surfacing that did not happen.
func writeSurfaced(stateKey string, res horizonResult, now time.Time) error {
	if stateKey == "" {
		return nil
	}
	at := now.UTC().Format(time.RFC3339)
	out := make(map[string]surfacedFeature, len(res.firm)+len(res.soft))
	for _, ks := range [][]ettlemesh.Tangle{res.firm, res.soft} {
		for _, k := range ks {
			out[tanglestate.Key(k.Kind, k.Parties)] = surfacedFeature{
				Kind: k.Kind, Votes: k.Votes, Samples: k.Samples, Firm: k.Firm(), At: at,
			}
		}
	}
	path, err := surfacedPath(stateKey)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(out)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// loadSurfaced returns the last horizon's features. A missing or unreadable file is
// an empty map, not an error: a verdict recorded without them is a worse row, not a
// failed command, and refusing to mute a nuisance because a cache was missing would
// be the wrong trade every time.
func loadSurfaced(stateKey string) map[string]surfacedFeature {
	path, err := surfacedPath(stateKey)
	if err != nil {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m map[string]surfacedFeature
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	return m
}

// enrichFromSurfaced fills a verdict's recurrence features from the last horizon, and
// leaves them zero when the tangle is not in it — the same rule the MCP server
// follows. A row with zero recurrence is honestly un-learnable; a row with invented
// recurrence is worse than no row, because `ettle calibrate` cannot tell it apart
// from a real one.
func enrichFromSurfaced(lbl *mcpserver.Label, stateKey, key string) {
	feat, ok := loadSurfaced(stateKey)[key]
	if !ok {
		return
	}
	lbl.Kind, lbl.Votes, lbl.Samples, lbl.Firm = feat.Kind, feat.Votes, feat.Samples, feat.Firm
}
