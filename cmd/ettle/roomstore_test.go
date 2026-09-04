package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The room moved out of the repo because a committed pointer enrols whoever clones it:
// open a session, and the SessionEnd hook publishes your distilled work into a room you
// never chose. ADOPTION.md forbids exactly that — state enters the shared layer only
// from a person's own act.

func TestRoomStoreCoversEveryDirectoryBeneathIt(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	if err := saveRoomForDir(root, "linear://dumpling", "dayjob"); err != nil {
		t.Fatal(err)
	}
	// One entry has to serve every worktree under a shared parent — that is the whole
	// reason the parent is the unit rather than the repo.
	for _, sub := range []string{".", "jupiter", "mars", filepath.Join("aipotluck.org", ".claude", "worktrees", "agent-x")} {
		dir := filepath.Join(root, sub)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		rf, ok := resolveRoom(dir)
		if !ok || rf.Spec != "linear://dumpling" || rf.Profile != "dayjob" {
			t.Errorf("%s resolved to %+v (ok=%v); want the parent's room and profile", sub, rf, ok)
		}
	}
	if _, ok := resolveRoom(t.TempDir()); ok {
		t.Error("an unrelated directory must resolve to no room, or every project on the machine joins one")
	}
}

func TestDeeperEntryWinsOverTheTreeItSitsIn(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	inner := filepath.Join(root, "sub", "project")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := saveRoomForDir(root, "linear://outer", ""); err != nil {
		t.Fatal(err)
	}
	if err := saveRoomForDir(inner, "linear://inner", "side"); err != nil {
		t.Fatal(err)
	}
	// Nearest-wins, matching the walk-up rule the file had.
	if rf, _ := resolveRoom(inner); rf.Spec != "linear://inner" || rf.Profile != "side" {
		t.Errorf("the deeper entry should win, got %+v", rf)
	}
	if rf, _ := resolveRoom(root); rf.Spec != "linear://outer" {
		t.Errorf("the outer directory keeps its own room, got %+v", rf)
	}
	// A sibling prefix must not match by string alone — /a/bcd is not under /a/b.
	if err := saveRoomForDir(root+"-other", "linear://other", ""); err != nil {
		t.Fatal(err)
	}
	if rf, _ := resolveRoom(inner); rf.Spec != "linear://inner" {
		t.Errorf("a lexical prefix leaked across directories, got %+v", rf)
	}
}

func TestLegacyRoomFileStillResolvesForOneRelease(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, roomFileName),
		[]byte("room = linear://crew\nprofile = work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Upgrading must not silently disconnect an existing install from its room.
	rf, ok := resolveRoom(dir)
	if !ok || rf.Spec != "linear://crew" || rf.Profile != "work" {
		t.Fatalf("a pre-existing .ettle-room should still work, got %+v (ok=%v)", rf, ok)
	}
	// But the store wins once it has an entry, so `ettle init` migrating is enough.
	if err := saveRoomForDir(dir, "linear://dumpling", "dayjob"); err != nil {
		t.Fatal(err)
	}
	if rf, _ := resolveRoom(dir); rf.Spec != "linear://dumpling" {
		t.Errorf("the store should take precedence after migration, got %+v", rf)
	}
}
