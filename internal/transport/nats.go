//go:build nats

// Package transport — NATS adapter. Compiled only with `-tags nats` so the
// default build needs no external bus and stays infra-free.
//
// The nats.go dep is already in go.mod; the tag just gates compilation:
//	go build -tags nats ./...    /    go run -tags nats ./cmd/ettle ...
//
// Security: NATS is secure-by-default for this use when you point it at a
// `tls://` URL (encrypted in transit) and supply credentials. Both are wired
// below from the environment — there is no plaintext-without-auth path off
// localhost.
//
// Why JetStream, not core pub/sub: each participant publishes once and then
// collects the whole team's atoms. With core NATS (no retention) a Collect that
// subscribes after a peer already published would silently miss it — a timing
// race that drops atoms. JetStream retains the subject, so Collect reads every
// participant's envelope regardless of publish/subscribe ordering; the gather
// window only bounds how long we wait for stragglers.
package transport

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/justinstimatze/ettle/internal/loopback"
	"github.com/nats-io/nats.go"
)

// NATSConfig configures the bus adapter. URL should be tls:// for any
// cross-machine deployment; nats:// (plaintext) is for localhost only.
type NATSConfig struct {
	URL         string        // e.g. tls://nats.example.com:4222
	CredsFile   string        // NATS credentials file (auth); required off localhost
	Team        string        // subject namespace: ettle.<team>.atoms
	GatherFor   time.Duration // how long Collect waits for stragglers after draining retained atoms
	InsecureTCP bool          // allow a plaintext nats:// URL (localhost testing only)
}

// NATSBus is a distributed atom bus over NATS JetStream. Each participant
// publishes its Envelope to ettle.<team>.atoms (a retained stream) and Collect
// reads every retained Envelope, keeping the latest per participant.
type NATSBus struct {
	nc      *nats.Conn
	js      nats.JetStreamContext
	subject string
	gather  time.Duration
}

// DialNATS connects with TLS + credentials enforced unless InsecureTCP is set
// for localhost, then ensures the team's atom stream exists. Reads
// NATS_URL / NATS_CREDS / ETTLE_TEAM when the matching fields are empty.
func DialNATS(cfg NATSConfig) (*NATSBus, error) {
	if cfg.URL == "" {
		cfg.URL = os.Getenv("NATS_URL")
	}
	if cfg.CredsFile == "" {
		cfg.CredsFile = os.Getenv("NATS_CREDS")
	}
	if cfg.Team == "" {
		if t := os.Getenv("ETTLE_TEAM"); t != "" {
			cfg.Team = t
		} else {
			cfg.Team = "default"
		}
	}
	if cfg.GatherFor == 0 {
		cfg.GatherFor = 8 * time.Second
	}

	// InsecureTCP (plaintext, no auth) is allowed ONLY against loopback —
	// resolving the host (not a bare flag, not a string match), so a remote URL,
	// or a non-loopback name dressed up as local, can't be run unauthed.
	if cfg.InsecureTCP && !loopback.IsURL(cfg.URL) {
		return nil, fmt.Errorf("transport/nats: InsecureTCP is localhost-only; %s is not local — use TLS + NATS_CREDS", cfg.URL)
	}

	opts := []nats.Option{nats.Name("ettle")}
	if cfg.CredsFile != "" {
		opts = append(opts, nats.UserCredentials(cfg.CredsFile))
	} else if !cfg.InsecureTCP {
		return nil, fmt.Errorf("transport/nats: no credentials (set NATS_CREDS) — refusing to connect without auth; pass InsecureTCP for localhost only")
	}
	if !cfg.InsecureTCP {
		opts = append(opts, nats.Secure(&tls.Config{MinVersion: tls.VersionTLS12}))
	}

	nc, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("transport/nats: connect %s: %w", cfg.URL, err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("transport/nats: jetstream: %w", err)
	}

	team := sanitizeTeam(cfg.Team)
	subject := fmt.Sprintf("ettle.%s.atoms", team)
	// A short-retention stream: long enough for everyone to publish + collect
	// within a standup, not a permanent log of who-thought-what.
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "ettle_" + team,
		Subjects: []string{subject},
		Storage:  nats.MemoryStorage,
		MaxAge:   time.Hour,
		Discard:  nats.DiscardOld,
	})
	if err != nil && !strings.Contains(err.Error(), "already in use") {
		nc.Close()
		return nil, fmt.Errorf("transport/nats: ensure stream: %w", err)
	}
	return &NATSBus{nc: nc, js: js, subject: subject, gather: cfg.GatherFor}, nil
}

func (b *NATSBus) Publish(_ context.Context, env Envelope) error {
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	if _, err := b.js.Publish(b.subject, data); err != nil {
		return fmt.Errorf("transport/nats: publish: %w", err)
	}
	return nil
}

// Collect replays the retained stream (every participant's envelope, regardless
// of when they published), keeping the latest envelope per participant, then
// waits out the gather window for any stragglers still publishing.
func (b *NATSBus) Collect(ctx context.Context) ([]Envelope, error) {
	sub, err := b.js.SubscribeSync(b.subject, nats.OrderedConsumer())
	if err != nil {
		return nil, fmt.Errorf("transport/nats: subscribe: %w", err)
	}
	defer sub.Unsubscribe()

	latest := map[string]Envelope{}
	deadline := time.Now().Add(b.gather)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		// Poll in small slices so retained messages drain fast while we still
		// wait the full window for late publishers.
		wait := 250 * time.Millisecond
		if remaining < wait {
			wait = remaining
		}
		msg, err := sub.NextMsg(wait)
		if err != nil {
			if ctx.Err() != nil {
				return mapValues(latest), ctx.Err()
			}
			continue // timeout slice — keep waiting until the deadline
		}
		var env Envelope
		if json.Unmarshal(msg.Data, &env) == nil && env.Participant != "" {
			latest[strings.ToLower(strings.TrimSpace(env.Participant))] = env
		}
	}
	return mapValues(latest), nil
}

func (b *NATSBus) Close() error {
	if b.nc != nil {
		b.nc.Close()
	}
	return nil
}

func mapValues(m map[string]Envelope) []Envelope {
	out := make([]Envelope, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// sanitizeTeam makes a team name safe for a JetStream stream/subject token
// (no dots, spaces, or wildcards).
func sanitizeTeam(team string) string {
	var b strings.Builder
	for _, r := range team {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "default"
	}
	return b.String()
}
