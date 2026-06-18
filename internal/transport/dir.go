package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DirBus is the zero-infrastructure multiplayer transport: a directory of
// per-participant JSONL files under <root>/.ettle/, meant to live in a folder a
// team already shares (Dropbox/Drive/git/Syncthing). Each participant writes
// ONLY its own file, so there is no write contention — per-writer files are
// exactly what every sync tool replicates cleanly. Securing/replicating the
// folder is deferred to whatever syncs it; what crosses is already
// boundary-distilled atoms, not raw notes.
//
// Storage is REPLACE-CURRENT: a participant's file holds their latest envelope,
// overwritten each Publish. No accumulation (trivial clean-exit, no unbounded
// growth, no longitudinal pile-up); history, if wanted, is an orthogonal concern
// (keep round-directory snapshots / git commits — `ettle drift` already models
// rounds that way).
//
// Honest limits: single-writer-per-participant is a convention backed by the
// folder's coarse access control, NOT enforced structurally (anyone who can
// write the folder can write any file) — real enforcement is the reserved
// Envelope.Sig. And a teammate whose file never reached this folder is invisible
// (Coverage reports who/how-stale among files PRESENT, not an out-of-band roster).
type DirBus struct {
	dir      string     // <root>/.ettle
	warnings []string   // non-fatal Collect issues, surfaced via Warnings()
	lastEnvs []Envelope // envelopes from the last Collect, for Coverage to reuse
}

const (
	ettleSubdir = ".ettle"
	atomsExt    = ".atoms.jsonl"
	envelopeV   = 1
)

// NewDirBus roots the bus at <root>/.ettle, creating it if absent.
func NewDirBus(root string) (*DirBus, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("dir transport: empty path (use file:///abs/path)")
	}
	dir := filepath.Join(root, ettleSubdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("dir transport: create %s: %w", dir, err)
	}
	return &DirBus{dir: dir}, nil
}

// slug folds a participant name to a stable filename stem (lowercased, trimmed,
// spaces and path separators to '-'), matching ettlemesh.SamePerson's identity
// semantics so "Alice " and "alice" share one file.
func slug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '/', '\\':
			return '-'
		}
		return r
	}, s)
	return s
}

func (d *DirBus) pathFor(participant string) string {
	return filepath.Join(d.dir, slug(participant)+atomsExt)
}

// Publish writes this participant's current envelope, replacing any prior one,
// via a temp file + atomic rename in the same directory so a concurrent Collect
// never observes a torn write.
func (d *DirBus) Publish(_ context.Context, env Envelope) error {
	if strings.TrimSpace(env.Participant) == "" {
		return fmt.Errorf("dir transport: envelope has no participant")
	}
	env.V = envelopeV
	env.EmittedAt = time.Now().UTC().Format(time.RFC3339)

	line, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("dir transport: marshal %s: %w", env.Participant, err)
	}
	line = append(line, '\n')

	final := d.pathFor(env.Participant)
	tmp, err := os.CreateTemp(d.dir, slug(env.Participant)+".*.tmp")
	if err != nil {
		return fmt.Errorf("dir transport: temp for %s: %w", env.Participant, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed; cleans up on any error path
	if _, err := tmp.Write(line); err != nil {
		tmp.Close()
		return fmt.Errorf("dir transport: write %s: %w", env.Participant, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("dir transport: close %s: %w", env.Participant, err)
	}
	if err := os.Rename(tmpName, final); err != nil {
		return fmt.Errorf("dir transport: rename %s: %w", env.Participant, err)
	}
	return nil
}

// isConflictCopy spots filenames a sync tool produces when it can't merge two
// versions of the "same" file. We never want to ingest one as a phantom
// participant (the PROVENANCE.md glob-junk lesson, in transport form).
func isConflictCopy(name string) bool {
	low := strings.ToLower(name)
	for _, mark := range []string{
		"conflicted copy", // Dropbox
		"conflict",        // Drive / generic ("case conflict", "sync conflict")
		".sync-conflict-", // Syncthing
	} {
		if strings.Contains(low, mark) {
			return true
		}
	}
	return false
}

// Collect reads every participant file under <root>/.ettle, tolerating junk: it
// only reads *.atoms.jsonl, skips conflict-copies, skips an unparseable file
// (recording a warning rather than failing the whole horizon), and treats the
// FILENAME as the authoritative identity (an in-file participant that disagrees
// is overridden and warned — cheap anti-spoof).
func (d *DirBus) Collect(_ context.Context) ([]Envelope, error) {
	matches, err := filepath.Glob(filepath.Join(d.dir, "*"+atomsExt))
	if err != nil {
		return nil, fmt.Errorf("dir transport: glob %s: %w", d.dir, err)
	}
	var out []Envelope
	d.warnings = d.warnings[:0]
	for _, path := range matches {
		base := filepath.Base(path)
		if isConflictCopy(base) {
			d.warnings = append(d.warnings, fmt.Sprintf("skipped sync conflict-copy %q", base))
			continue
		}
		want := strings.TrimSuffix(base, atomsExt)
		env, ok := d.readEnvelope(path, want)
		if !ok {
			continue
		}
		out = append(out, env)
	}
	d.lastEnvs = out
	return out, nil
}

// readEnvelope parses one participant file leniently. JSONL with replace-current
// is one line, but we read the LAST non-blank parseable line so a torn trailing
// write (or a stray earlier line) doesn't lose the envelope.
func (d *DirBus) readEnvelope(path, wantSlug string) (Envelope, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		d.warnings = append(d.warnings, fmt.Sprintf("skipped unreadable %q: %v", filepath.Base(path), err))
		return Envelope{}, false
	}
	lines := strings.Split(string(raw), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		ln := strings.TrimSpace(lines[i])
		if ln == "" {
			continue
		}
		var env Envelope
		if err := json.Unmarshal([]byte(ln), &env); err != nil {
			continue // torn/partial or non-JSON line — try an earlier one
		}
		// Filename slug is authoritative identity; correct a disagreeing in-file
		// claim and warn.
		if env.Participant == "" {
			env.Participant = wantSlug
		} else if slug(env.Participant) != wantSlug {
			d.warnings = append(d.warnings, fmt.Sprintf(
				"%q claims participant %q; using filename identity %q", filepath.Base(path), env.Participant, wantSlug))
			env.Participant = wantSlug
		}
		return env, true
	}
	d.warnings = append(d.warnings, fmt.Sprintf("skipped %q: no parseable envelope line", filepath.Base(path)))
	return Envelope{}, false
}

func (d *DirBus) Close() error { return nil }

// MemberStatus is one present participant's freshness, for the coverage report.
type MemberStatus struct {
	Participant string
	EmittedAt   string        // raw RFC3339 as written; "" if the writer didn't set it
	Staleness   time.Duration // now - EmittedAt; 0 if EmittedAt is unparseable/empty
}

// Coverage reports who is present in the folder and how stale each is, computed
// from the envelopes the last Collect returned (no re-read). It is NOT part of
// the Transport interface (InProcess/NATS don't implement it); the driver
// type-asserts for it. This is the false-all-clear guard: surface the roster +
// staleness so "clear" is never shown bare over a partially-synced folder.
func (d *DirBus) Coverage() []MemberStatus {
	now := time.Now().UTC()
	out := make([]MemberStatus, 0, len(d.lastEnvs))
	for _, env := range d.lastEnvs {
		ms := MemberStatus{Participant: env.Participant, EmittedAt: env.EmittedAt}
		if env.EmittedAt != "" {
			if t, err := time.Parse(time.RFC3339, env.EmittedAt); err == nil {
				if s := now.Sub(t.UTC()); s > 0 {
					ms.Staleness = s
				}
			}
		}
		out = append(out, ms)
	}
	return out
}

// Warnings returns and clears the non-fatal issues seen on the last Collect/
// Coverage pass (skipped conflict-copies, unparseable files, identity mismatches).
func (d *DirBus) Warnings() []string { return d.warnings }
