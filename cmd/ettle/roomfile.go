package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/justinstimatze/ettle/internal/transport"
)

// The per-project room pointer.
//
// The set-and-forget promise is a hook bundle in the GLOBAL ~/.claude/settings.json,
// which fires in every session of every project. That config therefore cannot name a
// room — the room has to travel with the project. `.ettle-room` in the project root is
// that pointer: `ettle init` writes it, every room-taking command falls back to it when
// no --room/--transport was passed, and a project without one leaves the hooks as silent
// no-ops instead of errors.

const roomFileName = ".ettle-room"

// roomFile is the parsed pointer: a transport spec ("linear://crew") or a configured
// leat room name, plus where it was found.
//
// The room is the only thing in here, deliberately — it is a fact about the PROJECT
// and is safe to commit. Identity is a fact about the PERSON and lives per-machine
// (see loadIdentity): a committed `me = alice` would publish Bob's atoms as Alice's,
// which is exactly the misattribution the transport works to prevent.
type roomFile struct {
	Spec string
	Path string
}

// parseRoomFile reads the tiny key=value format, tolerating a hand-authored file that
// is just the spec on one line. Unknown keys are ignored rather than an error — this
// is a pointer, not a config surface, and failing a hook over a typo would be worse
// than ignoring it.
func parseRoomFile(text string) roomFile {
	var rf roomFile
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			if rf.Spec == "" {
				rf.Spec = line // bare form: the whole line is the spec
			}
			continue
		}
		if key := strings.ToLower(strings.TrimSpace(k)); key == "room" || key == "transport" {
			rf.Spec = strings.TrimSpace(v)
		}
	}
	return rf
}

// findRoomFile walks up from dir to the filesystem root looking for `.ettle-room`,
// so a session started in a subdirectory still finds the project's room.
func findRoomFile(dir string) (roomFile, bool) {
	d, err := filepath.Abs(dir)
	if err != nil {
		return roomFile{}, false
	}
	for {
		path := filepath.Join(d, roomFileName)
		if data, err := os.ReadFile(path); err == nil {
			rf := parseRoomFile(string(data))
			if rf.Spec == "" {
				return roomFile{}, false // present but says nothing — same as absent
			}
			rf.Path = path
			return rf, true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return roomFile{}, false
		}
		d = parent
	}
}

// currentRoomFile is findRoomFile anchored at the working directory — what every
// command's fallback actually calls.
func currentRoomFile() (roomFile, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return roomFile{}, false
	}
	return findRoomFile(cwd)
}

// splitRoomSpec maps a spec onto the (--room, --transport) pair the commands already
// take: anything with a scheme is a transport, anything else is a configured leat room.
func splitRoomSpec(spec string) (room, transportName string) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", ""
	}
	if strings.Contains(spec, "://") {
		return "", spec
	}
	return spec, ""
}

// applyRoomFile fills an empty (--room, --transport) pair from the nearest
// `.ettle-room`. Explicit flags always win, so a project pointer never overrides
// what someone typed.
func applyRoomFile(room, transportName string) (string, string) {
	if room != "" || transportName != "" {
		return room, transportName
	}
	rf, ok := currentRoomFile()
	if !ok {
		return "", ""
	}
	return splitRoomSpec(rf.Spec)
}

// linearRoomFor resolves the Linear room name for the Linear-only commands (pull,
// escalate), which take the bare room rather than a transport spec. Only a
// `linear://` pointer answers: a leat room in `.ettle-room` means this project's bus
// is not Linear, and silently treating its name as a Linear project would be worse
// than reporting nothing.
func linearRoomFor(room string) string {
	if strings.TrimSpace(room) != "" {
		return strings.TrimSpace(room)
	}
	rf, ok := currentRoomFile()
	if !ok {
		return ""
	}
	if r, ok := strings.CutPrefix(rf.Spec, "linear://"); ok {
		return strings.TrimSpace(r)
	}
	return ""
}

// identityPath is where a room's identity is remembered for THIS machine — never in
// the project file, which teammates share.
func identityPath(spec string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ettle", "identity", transport.SanitizeID(spec)+".json"), nil
}

// saveIdentity records who you are in a room, so no later command needs --me.
func saveIdentity(spec, me string) error {
	if strings.TrimSpace(spec) == "" || strings.TrimSpace(me) == "" {
		return nil
	}
	path, err := identityPath(spec)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// The spec rides along because the filename is sanitized and therefore lossy
	// ("linear://crew" and "linear__crew" collapse to the same name). Without it
	// `ettle room list` could not name the rooms this machine belongs to.
	data, err := json.Marshal(map[string]string{"me": strings.TrimSpace(me), "room": strings.TrimSpace(spec)})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// loadIdentity returns the saved identity for a room spec, "" when none. Falls back
// through the project pointer, so a command run with no flags at all still knows who
// you are in this project's room.
func loadIdentity(spec string) string {
	if strings.TrimSpace(spec) == "" {
		rf, ok := currentRoomFile()
		if !ok {
			return ""
		}
		spec = rf.Spec
	}
	path, err := identityPath(spec)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var v identityFile
	if json.Unmarshal(data, &v) != nil {
		return ""
	}
	// Backfill the spec on files written before it was recorded, so `room list` can
	// name them. Best-effort: an unwritable config dir is not worth failing a hook over.
	if strings.TrimSpace(v.Room) == "" {
		v.Room = spec
		if out, err := json.Marshal(v); err == nil {
			_ = os.WriteFile(path, out, 0o644)
		}
	}
	return strings.TrimSpace(v.Me)
}

// identityFile is the per-machine record: who you are in a room, and which room.
type identityFile struct {
	Me   string `json:"me"`
	Room string `json:"room,omitempty"`
}

// knownRoom is one entry in the machine's room registry, as `ettle room list` shows it.
type knownRoom struct {
	Spec string
	Me   string
}

// knownRooms lists every room this machine has an identity for, on any transport.
// This is the answer to "which rooms am I in" — the leat room directory only knows
// about the git-repo bus, and most people are on Linear or GitHub instead.
//
// Files that predate the recorded spec are skipped rather than guessed at: the
// filename is sanitized, so reconstructing "linear://crew" from "linear___crew" would
// be inventing a room that may not exist. loadIdentity backfills them on first use.
func knownRooms() []knownRoom {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(dir, "ettle", "identity"))
	if err != nil {
		return nil
	}
	var out []knownRoom
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, "ettle", "identity", e.Name()))
		if err != nil {
			continue
		}
		var v identityFile
		if json.Unmarshal(data, &v) != nil || strings.TrimSpace(v.Room) == "" {
			continue
		}
		out = append(out, knownRoom{Spec: strings.TrimSpace(v.Room), Me: strings.TrimSpace(v.Me)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Spec < out[j].Spec })
	return out
}

// renderRoomFile is what `ettle init` writes — self-describing, because the next
// person to open it will not have read the docs.
func renderRoomFile(spec string) string {
	return "# ettle — this project's coordination room. Written by `ettle init`.\n" +
		"# The Claude Code hooks read it, so the hook config names no room and one\n" +
		"# global settings.json works across every project. Safe to commit: it says\n" +
		"# which room, not who you are (your identity is per-machine).\n" +
		"room = " + spec + "\n"
}
