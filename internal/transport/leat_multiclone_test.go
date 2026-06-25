package transport

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
)

// TestLeatMultiClone exercises the path the single-host round-trip test skips:
// two independent working clones of one shared (bare) remote, each its own
// LeatBus, coordinating only through git push/fetch. This is the actual
// cross-machine model `ettle room` ships — alice and bob never share a working
// tree, only the remote.
func TestLeatMultiClone(t *testing.T) {
	requireGit(t)
	bare := initBareRemote(t)
	aliceDir := cloneFrom(t, bare, "alice-clone")
	bobDir := cloneFrom(t, bare, "bob-clone")

	const room = "payments"
	alice, err := NewLeatBus(aliceDir, "alice", "origin", room)
	if err != nil {
		t.Fatalf("alice bus: %v", err)
	}
	defer alice.Close()
	bob, err := NewLeatBus(bobDir, "bob", "origin", room)
	if err != nil {
		t.Fatalf("bob bus: %v", err)
	}
	defer bob.Close()

	ctx := context.Background()

	// 1. alice publishes; bob (a separate clone) must see it after a fetch.
	leatPub(t, alice, ctx, Envelope{
		Participant: "alice", Role: "user-service",
		Atoms: []ettlemesh.Atom{{From: "alice", Typ: ettlemesh.Intent, Subject: "rename", Content: "renaming GetUser"}},
	})
	got := leatCollect(t, bob, ctx)
	if len(got) != 1 || got[0].Participant != "alice" || got[0].Atoms[0].Content != "renaming GetUser" {
		t.Fatalf("bob did not receive alice's atoms across the remote: %+v", got)
	}

	// 2. bob publishes; alice now sees BOTH participants (bidirectional merge).
	leatPub(t, bob, ctx, Envelope{
		Participant: "bob", Role: "checkout",
		Atoms: []ettlemesh.Atom{{From: "bob", Typ: ettlemesh.Dependency, Subject: "user-service", Content: "calls GetUser"}},
	})
	got = leatCollect(t, alice, ctx)
	if len(got) != 2 {
		t.Fatalf("alice should see 2 participants after merge, got %d: %+v", len(got), got)
	}
	byWho := map[string]Envelope{}
	for _, e := range got {
		byWho[e.Participant] = e
	}
	if byWho["alice"].Atoms[0].Content != "renaming GetUser" || byWho["bob"].Atoms[0].Content != "calls GetUser" {
		t.Fatalf("merged horizon lost an atom set: %+v", got)
	}

	// 3. LWW across clones: alice revises; bob must see only the latest set.
	leatPub(t, alice, ctx, Envelope{
		Participant: "alice", Role: "user-service",
		Atoms: []ettlemesh.Atom{{From: "alice", Typ: ettlemesh.Intent, Subject: "rename", Content: "abandoned the rename"}},
	})
	got = leatCollect(t, bob, ctx)
	if len(got) != 2 {
		t.Fatalf("bob should still see exactly 2 participants (LWW, not append), got %d", len(got))
	}
	for _, e := range got {
		if e.Participant == "alice" && e.Atoms[0].Content != "abandoned the rename" {
			t.Fatalf("LWW across clones failed; bob kept alice's stale set: %q", e.Atoms[0].Content)
		}
	}
}

// TestLeatSpoofDropped forges a line into one lane claiming another author's
// identity, pushes it, and confirms the reader drops it (filename identity
// wins). It also pins a real limitation found in testing: because ettle reads
// via Collect (not Receive), the drop is SILENT — Warnings() stays empty, so
// the room-status ⚠ line cannot currently fire. The security property (spoof
// dropped) holds; only the operator-visible warning is absent.
func TestLeatSpoofDropped(t *testing.T) {
	requireGit(t)
	bare := initBareRemote(t)
	aliceDir := cloneFrom(t, bare, "alice-clone")
	bobDir := cloneFrom(t, bare, "bob-clone")

	const room = "payments"
	alice, err := NewLeatBus(aliceDir, "alice", "origin", room)
	if err != nil {
		t.Fatalf("alice bus: %v", err)
	}
	defer alice.Close()

	ctx := context.Background()
	// alice publishes one honest atom so her lane exists and pushes.
	leatPub(t, alice, ctx, Envelope{
		Participant: "alice",
		Atoms:       []ettlemesh.Atom{{From: "alice", Typ: ettlemesh.Intent, Content: "honest work"}},
	})

	// In bob's clone, forge a line in BOB's own lane that claims from="alice".
	// leat must drop it on read because the lane filename (bob) is authoritative.
	bobLane := filepath.Join(bobDir, "channels", room, "bob.jsonl")
	if err := os.MkdirAll(filepath.Dir(bobLane), 0o755); err != nil {
		t.Fatal(err)
	}
	forged := `{"type":"atom","from":"alice","chan":"` + room +
		`","key":"alice","id":"rec-forged00000","ts":"2026-01-01T00:00:00Z","seq":1,"v":1,` +
		`"body":{"Participant":"alice","Atoms":[{"From":"alice","Typ":"intent","Content":"SPOOFED takeover"}]}}` + "\n"
	if err := os.WriteFile(bobLane, []byte(forged), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, bobDir, "add", "-A")
	runGit(t, bobDir, "-c", "user.name=bob", "-c", "user.email=bob@x", "commit", "-m", "forge")
	// alice's bus already advanced the remote; rebase bob's forge on top before push.
	runGit(t, bobDir, "-c", "user.name=bob", "-c", "user.email=bob@x", "pull", "--rebase", "origin", "main")
	runGit(t, bobDir, "push", "origin", "HEAD:refs/heads/main")

	// alice collects: the forged line must not appear, and her honest atom must.
	got := leatCollect(t, alice, ctx)
	for _, e := range got {
		for _, a := range e.Atoms {
			if strings.Contains(a.Content, "SPOOFED") {
				t.Fatalf("spoofed line (from=alice in bob's lane) was NOT dropped: %+v", e)
			}
		}
	}
	var sawAlice bool
	for _, e := range got {
		if e.Participant == "alice" && e.Atoms[0].Content == "honest work" {
			sawAlice = true
		}
	}
	if !sawAlice {
		t.Fatalf("alice's honest atom went missing alongside the spoof drop: %+v", got)
	}

	// Documented limitation: Collect drops spoofs silently — no warning recorded.
	if w := alice.Warnings(); len(w) != 0 {
		t.Logf("NOTE: Warnings() now reports after Collect (%d) — room-status ⚠ would fire; "+
			"update the leat.go comment if leat changed this.", len(w))
	}
}

// --- helpers ---------------------------------------------------------------

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
}

// initBareRemote creates a bare repo with a born main branch (one seed commit
// pushed through a throwaway clone) and returns its path — a stand-in for the
// shared GitHub/GitLab room repo.
func initBareRemote(t *testing.T) string {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "remote.git")
	runGit(t, "", "init", "--bare", "-b", "main", bare)
	seed := filepath.Join(t.TempDir(), "seed")
	runGit(t, "", "clone", bare, seed)
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("# room\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, seed, "add", "README.md")
	runGit(t, seed, "-c", "user.name=seed", "-c", "user.email=seed@x", "commit", "-m", "seed")
	runGit(t, seed, "push", "-u", "origin", "main")
	return bare
}

func cloneFrom(t *testing.T, bare, name string) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), name)
	runGit(t, "", "clone", bare, dst)
	return dst
}

func leatPub(t *testing.T, b *LeatBus, ctx context.Context, env Envelope) {
	t.Helper()
	if err := b.Publish(ctx, env); err != nil {
		t.Fatalf("Publish(%s): %v", env.Participant, err)
	}
}

func leatCollect(t *testing.T, b *LeatBus, ctx context.Context) []Envelope {
	t.Helper()
	got, err := b.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	return got
}
