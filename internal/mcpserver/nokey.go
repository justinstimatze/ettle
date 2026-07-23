package mcpserver

import (
	"context"
	"errors"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
)

// ErrNoKey is returned by NoKey for every operation that needs a model call.
//
// It names the key-free half of the protocol on purpose: the most likely reader
// is a teammate's agent that just tried the wrong tool, and the fix ("distill
// locally, send atoms") is not guessable from a bare "missing API key".
var ErrNoKey = errors.New("this ettle server has no ANTHROPIC_API_KEY, so it cannot call a model: " +
	"ettle_emit with `atoms` (get the `ettle_distill` prompt and distill on your side) and ettle_respond " +
	"work without one; ettle_emit with `notes` and ettle_horizon need a key — ask whoever runs reconcile for the room")

// NoKey is the reconciler used when `ettle mcp` starts without an API key.
//
// Serving beats refusing to start. Client-side distillation exists precisely so a
// teammate in Claude Code never needs a key of their own, and the tools that carry
// that path — ettle_emit with client-distilled atoms, and ettle_respond — make no
// model call at all. Requiring a key to *start* made the key-free path unreachable
// by the key-free person. The deterministic privacy boundary (SealAtoms) is
// unaffected: it never needed a model.
type NoKey struct{}

func (NoKey) Distill(context.Context, string, string, string, []string) ([]ettlemesh.Atom, error) {
	return nil, ErrNoKey
}

func (NoKey) ReconcileVoted(context.Context, []ettlemesh.Atom, int) ([]ettlemesh.Tangle, int, error) {
	return nil, 0, ErrNoKey
}

func (NoKey) ReconcileSelf(context.Context, []ettlemesh.Atom) ([]ettlemesh.Tangle, error) {
	return nil, ErrNoKey
}

// GroundTangles passes through rather than erroring: it is only ever reached with
// tangles a reconcile produced, which cannot happen here.
func (NoKey) GroundTangles(_ context.Context, tangles []ettlemesh.Tangle, _ []ettlemesh.Atom) (kept, suppressed []ettlemesh.Tangle, err error) {
	return tangles, nil, nil
}
