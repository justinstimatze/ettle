// Package transport — leat adapter (always compiled; leat is stdlib-only, so
// unlike the NATS adapter it needs no build tag).
//
// leat (github.com/justinstimatze/leat) is a git repository used as an
// append-only, per-author-lane message bus: durable, cross-machine, audited,
// with no central broker — each agent only ever appends to the one lane file it
// owns, so concurrent pushes never conflict. ettle rides it as its distributed
// transport: the room is a leat channel, each person's agent appends its atoms
// to its own lane, and Collect folds the latest atoms per participant. leat is
// owned by mcp-dispatch (the canonical Go impl of the shared git-transport wire
// contract); ettle is a consumer of it.
//
// Mapping ettle <-> leat:
//   - Publish → one leat record, Type="atom", Chan=<room>, Key=<participant>
//     (the LWW partition, so a participant's latest atoms replace their earlier
//     set), Body = the marshaled ettle Envelope (participant, role, atoms).
//   - Collect(TypeFilter="atom") → the latest record per (From,Key) across the
//     repo; we keep this room's and unmarshal each Body back to an Envelope.
//
// Config (read by the driver from the environment when the transport is
// "leat://<repoDir>"): LEAT_AGENT — this agent's stable id, which is also its
// lane filename and commit author; LEAT_REMOTE — a git remote (e.g. "origin")
// to enable push/fetch for cross-machine use, or empty for local-only;
// ETTLE_TEAM — the room channel name (default "default").
package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/justinstimatze/leat"
)

// LeatBus is an atom transport over a leat git bus. The room is a leat channel;
// each participant's atoms are one LWW record keyed by the participant, so
// Collect returns the latest envelope per person.
type LeatBus struct {
	bus  *leat.Bus
	room string
}

// NewLeatBus opens a leat bus at repoDir (an existing git working tree with a
// born HEAD) as agentID, posting to the given room channel. remote may be ""
// (local-only — single host or tests) or a git remote name (e.g. "origin") to
// push/fetch for cross-machine coordination.
func NewLeatBus(repoDir, agentID, remote, room string) (*LeatBus, error) {
	room = SanitizeID(room)
	var opts []leat.Option
	if remote != "" {
		opts = append(opts, leat.WithRemote(remote))
	}
	bus, err := leat.New(repoDir, agentID, opts...)
	if err != nil {
		return nil, fmt.Errorf("transport/leat: open %s as %q: %w", repoDir, agentID, err)
	}
	// Join the room. Collect scans every lane and we filter to this room below,
	// so the subscription is not strictly required for Collect — but it validates
	// the room name up front and joins us for any future Receive use.
	if err := bus.Subscribe(room); err != nil {
		_ = bus.Close()
		return nil, fmt.Errorf("transport/leat: join room %q: %w", room, err)
	}
	return &LeatBus{bus: bus, room: room}, nil
}

// Publish appends this participant's atoms to the room as one LWW record keyed
// by the participant, so their latest set replaces any earlier one.
func (b *LeatBus) Publish(ctx context.Context, env Envelope) error {
	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("transport/leat: marshal envelope: %w", err)
	}
	key := strings.ToLower(strings.TrimSpace(env.Participant))
	if _, err := b.bus.Publish(ctx, leat.Envelope{
		Type: "atom",
		Chan: b.room,
		Key:  key,
		Body: body,
	}); err != nil {
		return fmt.Errorf("transport/leat: publish: %w", err)
	}
	return nil
}

// Collect returns the latest atom envelope per participant in this room. leat
// gives the latest record per (From,Key) across the whole repo; we keep this
// room's records (ignoring DM lanes or other rooms sharing the repo) and
// unmarshal each Body back to an ettle Envelope.
func (b *LeatBus) Collect(ctx context.Context) ([]Envelope, error) {
	recs, err := b.bus.Collect(ctx, leat.CollectOptions{TypeFilter: "atom"})
	if err != nil {
		return nil, fmt.Errorf("transport/leat: collect: %w", err)
	}
	out := make([]Envelope, 0, len(recs))
	for _, r := range recs {
		if r.Chan != b.room {
			continue // another room, or a DM lane, sharing this repo
		}
		var env Envelope
		if err := json.Unmarshal(r.Body, &env); err != nil {
			continue // skip an unreadable body rather than fail the whole horizon
		}
		out = append(out, env)
	}
	return out, nil
}

func (b *LeatBus) Close() error { return b.bus.Close() }

// Warnings surfaces leat's non-fatal issues from the last Collect (dropped
// identity spoofs — a line whose author != lane owner — and malformed lines).
func (b *LeatBus) Warnings() []string { return b.bus.Warnings() }

// SanitizeID coerces a string into a valid leat id: letters, digits, '-' or '_',
// not starting with '-', capped at 128 runes (leat's id rule, shared by channel
// names and agent ids). Used here for the room channel and by the room command
// for agent ids, so the rule lives in exactly one place. Empty input yields
// "default".
func SanitizeID(s string) string {
	var sb strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			sb.WriteRune(r)
		default:
			sb.WriteRune('_')
		}
	}
	out := sb.String()
	if out == "" {
		return "default"
	}
	if out[0] == '-' { // leat ids may not start with '-'
		out = "_" + out
	}
	if len(out) > 128 {
		out = out[:128]
	}
	return out
}
