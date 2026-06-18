// Package transport is the swappable seam that carries typed atoms between
// participants. The detector (internal/ettlemesh) is transport-agnostic: it
// reconciles whatever atoms it is handed. How those atoms get from each
// person's machine to the reconciler is this package's job.
//
// Two adapters ship:
//
//   - InProcess (this file, zero dependencies) — everything in one process.
//     Used for local fixture runs and tests, so the whole loop is exercisable
//     with NO infrastructure. This is the default.
//   - NATS (nats.go, behind the `nats` build tag) — a secure distributed bus:
//     each participant publishes their atoms and collects the team's from a
//     subject. TLS + auth are NATS-native. Build with `-tags nats`.
//
// Other rails (Slack, Matrix, A2A) can implement Transport later without the
// detector or driver changing.
package transport

import (
	"context"
	"sync"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
)

// Envelope is one participant's contribution crossing the boundary: the typed
// atoms only, never their raw notes. This is the contextual-privacy invariant
// in its cheap form.
//
// The V/EmittedAt/Sig fields are additive and only set by the file transport
// (DirBus); the in-process and NATS paths leave them zero, which is harmless.
type Envelope struct {
	Participant string           `json:"participant"`
	Role        string           `json:"role"`
	Atoms       []ettlemesh.Atom `json:"atoms"`

	// V is the envelope schema version. A file written by one ettle version may
	// be read by another across a synced folder; readers tolerate a missing V
	// (treat as 1) and an unknown-higher V (best-effort). Zero on the inproc/NATS
	// paths, which never persist.
	V int `json:"v,omitempty"`
	// EmittedAt (RFC3339 UTC) is when the writer emitted this envelope. It is a
	// DISPLAY / staleness signal only — never used for ordering or correctness, so
	// cross-machine clock skew can't produce a wrong horizon (replace-current has
	// no cross-round ordering). Preferred over file mtime, which sync tools rewrite
	// to download time. Empty when unknown.
	EmittedAt string `json:"emitted_at,omitempty"`
	// Sig is RESERVED for a future per-envelope signature (the honest path to
	// enforced identity over a shared folder, where filename ownership is only a
	// convention). Always "" in v1; reserved now so adding it later is not a schema
	// break.
	Sig string `json:"sig,omitempty"`
}

// Transport moves Envelopes between participants. Publish announces this
// participant's atoms; Collect returns every participant's atoms (including
// this one's) so the reconciler can see the whole team.
type Transport interface {
	Publish(ctx context.Context, env Envelope) error
	Collect(ctx context.Context) ([]Envelope, error)
	Close() error
}

// InProcess is the zero-infra adapter: it just accumulates envelopes in memory.
// For a local fixture run the driver publishes every participant here, then
// collects them all. Safe for concurrent use.
type InProcess struct {
	mu   sync.Mutex
	envs []Envelope
}

// NewInProcess returns an empty in-process transport.
func NewInProcess() *InProcess { return &InProcess{} }

func (t *InProcess) Publish(_ context.Context, env Envelope) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.envs = append(t.envs, env)
	return nil
}

func (t *InProcess) Collect(_ context.Context) ([]Envelope, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Envelope, len(t.envs))
	copy(out, t.envs)
	return out, nil
}

func (t *InProcess) Close() error { return nil }

// Atoms flattens a set of envelopes into the atom slice the detector consumes.
func Atoms(envs []Envelope) []ettlemesh.Atom {
	var out []ettlemesh.Atom
	for _, e := range envs {
		out = append(out, e.Atoms...)
	}
	return out
}
