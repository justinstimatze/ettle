package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseRoomFileForms(t *testing.T) {
	full := parseRoomFile("# ettle\n\nroom = linear://crew\nunknown = x\n")
	if full.Spec != "linear://crew" {
		t.Errorf("key form: got %q", full.Spec)
	}
	bare := parseRoomFile("linear://crew\n")
	if bare.Spec != "linear://crew" {
		t.Errorf("bare form should parse: got %q", bare.Spec)
	}
	// Identity is deliberately NOT a field here — a committed `me` would publish a
	// teammate's atoms under someone else's name.
	if only := parseRoomFile("me = alice\n"); only.Spec != "" {
		t.Errorf("a file with no room should yield no spec, got %q", only.Spec)
	}
	if empty := parseRoomFile("# just a comment\n\n"); empty.Spec != "" {
		t.Errorf("comments-only should yield no spec, got %q", empty.Spec)
	}
}

func TestFindRoomFileWalksUp(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := findRoomFile(deep); ok {
		t.Fatal("no pointer anywhere should find nothing")
	}
	if err := os.WriteFile(filepath.Join(root, roomFileName), []byte(renderRoomFile("linear://crew")), 0o644); err != nil {
		t.Fatal(err)
	}
	rf, ok := findRoomFile(deep)
	if !ok || rf.Spec != "linear://crew" {
		t.Fatalf("should find the pointer from a subdirectory: ok=%v spec=%q", ok, rf.Spec)
	}
	if rf.Path != filepath.Join(root, roomFileName) {
		t.Errorf("path should point at where it was found: %q", rf.Path)
	}
	// A pointer that names nothing reads as absent, not as an empty room.
	if err := os.WriteFile(filepath.Join(root, roomFileName), []byte("# nothing here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := findRoomFile(deep); ok {
		t.Error("a pointer with no room should read as absent")
	}
}

func TestSplitRoomSpec(t *testing.T) {
	if r, tr := splitRoomSpec("linear://crew"); r != "" || tr != "linear://crew" {
		t.Errorf("a scheme is a transport: room=%q transport=%q", r, tr)
	}
	if r, tr := splitRoomSpec("crew"); r != "crew" || tr != "" {
		t.Errorf("a bare name is a leat room: room=%q transport=%q", r, tr)
	}
	if r, tr := splitRoomSpec(""); r != "" || tr != "" {
		t.Errorf("empty stays empty: %q %q", r, tr)
	}
}

func TestApplyRoomFileFlagsWin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, roomFileName), []byte(renderRoomFile("linear://crew")), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	if r, tr := applyRoomFile("", ""); r != "" || tr != "linear://crew" {
		t.Errorf("empty flags should take the pointer: room=%q transport=%q", r, tr)
	}
	if r, tr := applyRoomFile("other", ""); r != "other" || tr != "" {
		t.Errorf("an explicit --room must win over the pointer: room=%q transport=%q", r, tr)
	}
	if r, tr := applyRoomFile("", "file://x"); r != "" || tr != "file://x" {
		t.Errorf("an explicit --transport must win: room=%q transport=%q", r, tr)
	}
}

func TestLinearRoomForOnlyAnswersForLinear(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if got := linearRoomFor(""); got != "" {
		t.Errorf("no pointer should yield no room, got %q", got)
	}
	if got := linearRoomFor("explicit"); got != "explicit" {
		t.Errorf("an explicit room wins: %q", got)
	}

	if err := os.WriteFile(filepath.Join(dir, roomFileName), []byte(renderRoomFile("linear://crew")), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := linearRoomFor(""); got != "crew" {
		t.Errorf("a linear pointer should yield the bare room: %q", got)
	}
	// A leat room means this project's bus is not Linear — better to report nothing
	// than to treat its name as a Linear project that doesn't exist.
	if err := os.WriteFile(filepath.Join(dir, roomFileName), []byte(renderRoomFile("standup-room")), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := linearRoomFor(""); got != "" {
		t.Errorf("a non-Linear pointer must not answer a Linear question: %q", got)
	}
}

func TestIdentityRoundTripIsPerRoom(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())

	if got := loadIdentity("linear://crew"); got != "" {
		t.Fatalf("cold store should be empty, got %q", got)
	}
	if err := saveIdentity("linear://crew", "justin"); err != nil {
		t.Fatal(err)
	}
	if got := loadIdentity("linear://crew"); got != "justin" {
		t.Errorf("round-trip mismatch: %q", got)
	}
	if got := loadIdentity("linear://other"); got != "" {
		t.Errorf("a different room should be independent: %q", got)
	}
	// An empty spec resolves through the project pointer.
	cwd, _ := os.Getwd()
	if err := os.WriteFile(filepath.Join(cwd, roomFileName), []byte(renderRoomFile("linear://crew")), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadIdentity(""); got != "justin" {
		t.Errorf("empty spec should resolve via the pointer: %q", got)
	}
}
