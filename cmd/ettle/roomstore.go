package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The per-machine directory→room map.
//
// This used to be `.ettle-room`, a file at the project root that the docs called
// "safe to commit". It isn't, and the reason is ettle's own consent invariant rather
// than a general worry: ADOPTION.md says state enters the shared layer only from a
// person's own act, and presence is explicit and revocable. A committed room pointer
// breaks both — clone the repo, open a session, and the SessionEnd hook publishes
// your distilled work into a room you never chose. That is enrollment by proximity,
// which that document opens by calling disqualifying.
//
// The convention agrees. Across tooling, a file that states what the PROJECT is gets
// committed, and a file recording which external account THIS CHECKOUT is wired to
// stays local — `.vercel/project.json` and `.mcp.json` are gitignored while
// `.linear.json` and `.claude/` are committed. A room is people, not code, and unlike
// any of those it causes an action on clone.
//
// So the mapping lives here, beside the identity that was already per-machine for the
// same reason: `me = alice` committed would publish Bob's atoms as Alice's.

// roomStoreEntry is one directory's room, as stored.
type roomStoreEntry struct {
	Room    string `json:"room"`
	Profile string `json:"profile,omitempty"`
}

// roomStorePath is the machine's directory→room map.
func roomStorePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "ettle", "rooms.json")
}

func loadRoomStore() map[string]roomStoreEntry {
	out := map[string]roomStoreEntry{}
	path := roomStorePath()
	if path == "" {
		return out
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(data, &out)
	return out
}

// saveRoomForDir records that `dir` (and everything under it, unless a deeper entry
// says otherwise) belongs to `spec`.
func saveRoomForDir(dir, spec, profile string) error {
	path := roomStorePath()
	if path == "" || strings.TrimSpace(dir) == "" || strings.TrimSpace(spec) == "" {
		return nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	store := loadRoomStore()
	store[abs] = roomStoreEntry{Room: strings.TrimSpace(spec), Profile: strings.TrimSpace(profile)}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// roomForDir finds the room governing `dir`: the LONGEST recorded path that is dir or
// one of its ancestors, so a subdirectory can override the tree it sits in — the same
// nearest-wins rule the old file walk had.
func roomForDir(dir string) (roomFile, bool) {
	rf, _, ok := roomForDirAt(dir)
	return rf, ok
}

// roomForDirAt is roomForDir plus WHICH directory matched, so resolveRoom can compare
// how near this answer is against a legacy file's.
func roomForDirAt(dir string) (roomFile, string, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return roomFile{}, "", false
	}
	store := loadRoomStore()
	keys := make([]string, 0, len(store))
	for k := range store {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, k := range keys {
		if abs == k || strings.HasPrefix(abs, k+string(os.PathSeparator)) {
			e := store[k]
			if strings.TrimSpace(e.Room) == "" {
				continue
			}
			return roomFile{Spec: e.Room, Profile: e.Profile, Path: roomStorePath()}, k, true
		}
	}
	return roomFile{}, "", false
}

// resolveRoom is what every command uses: the per-machine store first, then a legacy
// `.ettle-room` walking up from dir.
//
// The legacy read stays for one release so nobody's install breaks on upgrade, and
// `ettle init` migrates a file it finds into the store. Nothing writes `.ettle-room`
// any more.
func resolveRoom(dir string) (roomFile, bool) {
	stored, at, haveStore := roomForDirAt(dir)
	legacy, haveLegacy := findRoomFile(dir)
	switch {
	case haveStore && haveLegacy:
		// NEAREST wins, whichever source it came from. Taking the store unconditionally
		// was wrong and silently changed behaviour: a directory with its own legacy
		// pointer to one room sat under a parent recorded for another, and the parent
		// won — so that directory would have published into a different Linear
		// workspace than the one it was deliberately pointed at, with no error. That is
		// the failure this release exists to prevent, reintroduced by its own migration.
		if len(filepath.Dir(legacy.Path)) > len(at) {
			return legacy, true
		}
		return stored, true
	case haveStore:
		return stored, true
	default:
		return legacy, haveLegacy
	}
}

// currentRoom is resolveRoom anchored at the working directory — what the hooks hit.
func currentRoom() (roomFile, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return roomFile{}, false
	}
	return resolveRoom(cwd)
}
