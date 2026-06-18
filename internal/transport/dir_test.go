package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
)

func atom(subj string) ettlemesh.Atom {
	return ettlemesh.Atom{From: "x", Typ: ettlemesh.Dependency, Subject: subj, Confidence: 1}
}

// ettleDir is where DirBus keeps its files, given a root.
func ettleDir(root string) string { return filepath.Join(root, ettleSubdir) }

func TestDirBusPublishCollectRoundTrip(t *testing.T) {
	root := t.TempDir()
	b, err := NewDirBus(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := b.Publish(ctx, Envelope{Participant: "Alice", Role: "backend", Atoms: []ettlemesh.Atom{atom("cache")}}); err != nil {
		t.Fatal(err)
	}
	envs, err := b.Collect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 {
		t.Fatalf("want 1 envelope, got %d", len(envs))
	}
	e := envs[0]
	// In-file name's slug matches the filename, so the original display casing is
	// preserved (identity matching downstream is case-insensitive via SamePerson).
	if e.Participant != "Alice" {
		t.Errorf("participant = %q, want preserved 'Alice'", e.Participant)
	}
	if len(e.Atoms) != 1 || e.Atoms[0].Subject != "cache" {
		t.Errorf("atoms not round-tripped: %+v", e.Atoms)
	}
	if e.V != envelopeV {
		t.Errorf("schema version = %d, want %d", e.V, envelopeV)
	}
	if e.EmittedAt == "" {
		t.Error("EmittedAt should be set by Publish")
	}
}

func TestDirBusReplaceCurrentNotAppend(t *testing.T) {
	root := t.TempDir()
	b, _ := NewDirBus(root)
	ctx := context.Background()
	_ = b.Publish(ctx, Envelope{Participant: "alice", Atoms: []ettlemesh.Atom{atom("first")}})
	_ = b.Publish(ctx, Envelope{Participant: "alice", Atoms: []ettlemesh.Atom{atom("second")}})

	envs, _ := b.Collect(ctx)
	if len(envs) != 1 {
		t.Fatalf("replace-current: want 1 envelope, got %d", len(envs))
	}
	if envs[0].Atoms[0].Subject != "second" {
		t.Errorf("want latest atoms 'second', got %q", envs[0].Atoms[0].Subject)
	}
	// The file itself must hold one logical line, not an append log.
	raw, _ := os.ReadFile(filepath.Join(ettleDir(root), "alice"+atomsExt))
	nonEmpty := 0
	for _, ln := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.TrimSpace(ln) != "" {
			nonEmpty++
		}
	}
	if nonEmpty != 1 {
		t.Errorf("replace-current file should have 1 line, got %d", nonEmpty)
	}
}

func TestDirBusTwoParticipants(t *testing.T) {
	root := t.TempDir()
	b, _ := NewDirBus(root)
	ctx := context.Background()
	_ = b.Publish(ctx, Envelope{Participant: "alice", Atoms: []ettlemesh.Atom{atom("a")}})
	_ = b.Publish(ctx, Envelope{Participant: "bob", Atoms: []ettlemesh.Atom{atom("b")}})
	envs, _ := b.Collect(ctx)
	if len(envs) != 2 {
		t.Fatalf("want 2 participants, got %d", len(envs))
	}
}

func TestDirBusSkipsConflictCopyAndJunk(t *testing.T) {
	root := t.TempDir()
	b, _ := NewDirBus(root)
	ctx := context.Background()
	_ = b.Publish(ctx, Envelope{Participant: "alice", Atoms: []ettlemesh.Atom{atom("a")}})

	dir := ettleDir(root)
	// A sync conflict-copy of alice's file (Dropbox style) and an unrelated file.
	good, _ := json.Marshal(Envelope{Participant: "alice", Atoms: []ettlemesh.Atom{atom("dupe")}})
	if err := os.WriteFile(filepath.Join(dir, "alice (conflicted copy 2026-06-18).atoms.jsonl"), append(good, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("not a participant"), 0o644); err != nil {
		t.Fatal(err)
	}
	envs, _ := b.Collect(ctx)
	if len(envs) != 1 || envs[0].Participant != "alice" {
		t.Fatalf("conflict-copy/junk should be skipped, got %+v", envs)
	}
	if !hasWarning(b.Warnings(), "conflict-copy") {
		t.Errorf("expected a conflict-copy warning, got %v", b.Warnings())
	}
}

func TestDirBusToleratesTornTrailingLine(t *testing.T) {
	root := t.TempDir()
	b, _ := NewDirBus(root)
	dir := ettleDir(root)
	good, _ := json.Marshal(Envelope{Participant: "alice", Atoms: []ettlemesh.Atom{atom("ok")}})
	// A complete line, then a torn/partial JSON write (as a crash mid-write would
	// leave). The reader should fall back to the last PARSEABLE line.
	content := string(good) + "\n" + `{"participant":"alice","ato`
	if err := os.WriteFile(filepath.Join(dir, "alice"+atomsExt), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	envs, _ := b.Collect(context.Background())
	if len(envs) != 1 || envs[0].Atoms[0].Subject != "ok" {
		t.Fatalf("torn trailing line should be tolerated, got %+v", envs)
	}
}

func TestDirBusSkipsUnparseableFile(t *testing.T) {
	root := t.TempDir()
	b, _ := NewDirBus(root)
	dir := ettleDir(root)
	if err := os.WriteFile(filepath.Join(dir, "alice"+atomsExt), []byte("not json at all\n{also bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	envs, _ := b.Collect(context.Background())
	if len(envs) != 0 {
		t.Fatalf("a fully-unparseable file should yield no envelope, got %+v", envs)
	}
	if !hasWarning(b.Warnings(), "no parseable envelope") {
		t.Errorf("expected an unparseable-file warning, got %v", b.Warnings())
	}
}

func TestDirBusFilenameIsAuthoritativeIdentity(t *testing.T) {
	root := t.TempDir()
	b, _ := NewDirBus(root)
	dir := ettleDir(root)
	// File named alice, but the content claims to be bob (spoof / stale rename).
	spoof, _ := json.Marshal(Envelope{Participant: "bob", Atoms: []ettlemesh.Atom{atom("x")}})
	if err := os.WriteFile(filepath.Join(dir, "alice"+atomsExt), append(spoof, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	envs, _ := b.Collect(context.Background())
	if len(envs) != 1 || envs[0].Participant != "alice" {
		t.Fatalf("filename identity should win, got %+v", envs)
	}
	if !hasWarning(b.Warnings(), "claims participant") {
		t.Errorf("expected an identity-mismatch warning, got %v", b.Warnings())
	}
}

func TestDirBusConcurrentPublishDistinctRaceFree(t *testing.T) {
	root := t.TempDir()
	b, _ := NewDirBus(root)
	ctx := context.Background()
	const n = 16
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = b.Publish(ctx, Envelope{Participant: fmt.Sprintf("p%02d", i), Atoms: []ettlemesh.Atom{atom("x")}})
		}(i)
	}
	wg.Wait()
	envs, _ := b.Collect(ctx)
	if len(envs) != n {
		t.Errorf("want %d participants after concurrent publish, got %d", n, len(envs))
	}
}

func TestDirBusCoverage(t *testing.T) {
	root := t.TempDir()
	b, _ := NewDirBus(root)
	ctx := context.Background()
	_ = b.Publish(ctx, Envelope{Participant: "alice", Atoms: []ettlemesh.Atom{atom("a")}})
	_ = b.Publish(ctx, Envelope{Participant: "bob", Atoms: []ettlemesh.Atom{atom("b")}})
	if _, err := b.Collect(ctx); err != nil {
		t.Fatal(err)
	}
	cov := b.Coverage()
	if len(cov) != 2 {
		t.Fatalf("coverage should list 2 members, got %d", len(cov))
	}
	for _, m := range cov {
		if m.EmittedAt == "" {
			t.Errorf("member %q should have an EmittedAt", m.Participant)
		}
		if m.Staleness < 0 {
			t.Errorf("member %q staleness should be >= 0, got %v", m.Participant, m.Staleness)
		}
	}
}

func TestSlugFolds(t *testing.T) {
	cases := map[string]string{"Alice ": "alice", "Bob Smith": "bob-smith", " a/b ": "a-b"}
	for in, want := range cases {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsConflictCopy(t *testing.T) {
	yes := []string{"alice (conflicted copy).atoms.jsonl", "x.sync-conflict-abc.atoms.jsonl", "doc (case conflict).atoms.jsonl"}
	for _, n := range yes {
		if !isConflictCopy(n) {
			t.Errorf("%q should be a conflict copy", n)
		}
	}
	if isConflictCopy("alice.atoms.jsonl") {
		t.Error("a clean filename is not a conflict copy")
	}
	// A participant whose name merely contains "conflict" must NOT be mistaken for
	// a sync conflict-copy and silently dropped (the over-greedy-substring bug).
	if isConflictCopy("conflict-resolution.atoms.jsonl") {
		t.Error("a name containing 'conflict' is not a conflict copy")
	}
}

// TestDirBusKeepsParticipantNamedLikeConflict locks the silent-drop fix end to
// end: a participant whose folded name contains "conflict" is collected, not
// skipped.
func TestDirBusKeepsParticipantNamedLikeConflict(t *testing.T) {
	root := t.TempDir()
	b, _ := NewDirBus(root)
	ctx := context.Background()
	if err := b.Publish(ctx, Envelope{Participant: "conflict-team", Atoms: []ettlemesh.Atom{atom("x")}}); err != nil {
		t.Fatal(err)
	}
	envs, _ := b.Collect(ctx)
	if len(envs) != 1 || envs[0].Participant != "conflict-team" {
		t.Fatalf("a participant named like 'conflict' must survive Collect, got %+v", envs)
	}
}

func hasWarning(ws []string, substr string) bool {
	for _, w := range ws {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}
