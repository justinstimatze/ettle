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
// leat room name, the key profile to read, plus where it was found.
//
// LEGACY. Nothing writes this file any more — the mapping lives per-machine in
// <config>/ettle/rooms.json (see roomstore.go). It is still READ for one release so an
// existing install keeps working across the upgrade, and `ettle init` migrates it.
//
// It moved because "safe to commit", which this file used to claim, was wrong: a room
// pointer in a repo enrols whoever clones it into a room they never chose, and
// ADOPTION.md says state enters the shared layer only from a person's own act.
type roomFile struct {
	Spec    string
	Profile string
	Path    string
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
		switch key := strings.ToLower(strings.TrimSpace(k)); key {
		case "room", "transport":
			rf.Spec = strings.TrimSpace(v)
		case "profile":
			rf.Profile = strings.TrimSpace(v)
		}
	}
	return rf
}

// activeProfile decides which key profile a command should read: an explicit
// ETTLE_PROFILE wins over the committed line, because the room file is SHARED and a
// teammate may name their profile something else on their own machine.
func activeProfile(rf roomFile) string {
	if p := strings.TrimSpace(os.Getenv(profileEnvVar)); p != "" {
		return p
	}
	return rf.Profile
}

// profileEnvVar is the per-machine override for the room file's committed profile line.
const profileEnvVar = "ETTLE_PROFILE"

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
func currentRoomFile() (roomFile, bool) { return currentRoom() }

// loadProjectProfile loads THIS directory's key profile without touching the room
// target, for the commands that resolve their target some other way — `ettle capture`,
// whose bare form is a free local preview that applyRoomFile would silently turn into a
// paid publish, and `ettle teams`, which has no room yet. `override` is a --profile flag
// and wins when set.
//
// Splitting the keys off from the room is the whole point: applyRoomFile does both, so a
// command that must not adopt the room used to get neither, and its failure was a
// missing LINEAR_API_KEY in exactly the projects that name a profile.
func loadProjectProfile(override string) {
	if p := strings.TrimSpace(override); p != "" {
		loadProfileEnv(p)
		return
	}
	rf, _ := currentRoomFile()
	loadProfileEnv(activeProfile(rf))
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
//
// It also loads the project's key profile, and does so BEFORE the explicit-flags
// early return: a typed `--room` still belongs to this project and still wants this
// project's keys. Putting the load here rather than at the ~8 call sites is the same
// anti-drift reasoning loadAndDetect uses — a funnel someone can forget is a funnel
// someone will forget, and the failure would be silent (the wrong workspace's key,
// no error).
func applyRoomFile(room, transportName string) (string, string) {
	rf, ok := currentRoomFile()
	loadProfileEnv(activeProfile(rf))
	if room != "" || transportName != "" {
		return room, transportName
	}
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
	rf, _ := currentRoomFile()
	// Load before the explicit-room return, for the same reason applyRoomFile does:
	// pull and escalate read LINEAR_API_KEY straight from the environment moments
	// later, and a typed --room does not make this a different project.
	loadProfileEnv(activeProfile(rf))
	if strings.TrimSpace(room) != "" {
		return strings.TrimSpace(room)
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

// readIdentity returns the stored record for a spec, or a zero value when there is
// none. Every writer goes through this first so it can preserve the fields it does
// not own — the file has two independent authors (identity and workspace) and a
// writer that marshals only its own half silently erases the other's.
func readIdentity(spec string) (identityFile, string, error) {
	path, err := identityPath(spec)
	if err != nil {
		return identityFile{}, "", err
	}
	var v identityFile
	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &v)
	}
	return v, path, nil
}

// writeIdentity persists a whole record. Marshaling the STRUCT rather than a literal
// map is the whole point: a map of just the caller's fields is how the workspace got
// dropped, and that failure is invisible — the guard it feeds simply never fires.
func writeIdentity(path string, v identityFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// saveIdentity records who you are in a room, so no later command needs --me. It
// preserves any recorded workspace.
func saveIdentity(spec, me string) error {
	if strings.TrimSpace(spec) == "" || strings.TrimSpace(me) == "" {
		return nil
	}
	v, path, err := readIdentity(spec)
	if err != nil {
		return err
	}
	v.Me = strings.TrimSpace(me)
	// The spec rides along because the filename is sanitized and therefore lossy
	// ("linear://crew" and "linear__crew" collapse to the same name). Without it
	// `ettle room list` could not name the rooms this machine belongs to.
	v.Room = strings.TrimSpace(spec)
	return writeIdentity(path, v)
}

// saveOrg records which Linear workspace a room actually lives in, so a later run
// with a different workspace's key can be refused instead of quietly creating a
// second project of the same name somewhere nobody will look. It preserves the
// identity — the mirror image of saveIdentity, and for the same reason.
//
// Both id and name are kept: the comparison needs the id, and the refusal has to be
// able to say WHICH workspace, which an id cannot do for a human.
func saveOrg(spec, id, name string) error {
	if strings.TrimSpace(spec) == "" || strings.TrimSpace(id) == "" {
		return nil
	}
	v, path, err := readIdentity(spec)
	if err != nil {
		return err
	}
	v.Org = &orgRef{ID: strings.TrimSpace(id), Name: strings.TrimSpace(name)}
	if strings.TrimSpace(v.Room) == "" {
		v.Room = strings.TrimSpace(spec)
	}
	return writeIdentity(path, v)
}

// loadOrg returns the workspace recorded for a room, or a zero orgRef when none is —
// which is the "no expectation" case every pre-existing install starts in, and why
// adding the guard cannot break a setup that already works.
func loadOrg(spec string) orgRef {
	v, _, err := readIdentity(spec)
	if err != nil || v.Org == nil {
		return orgRef{}
	}
	return *v.Org
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

// identityFile is the per-machine record: who you are in a room, which room, and
// which Linear workspace that room was found in.
type identityFile struct {
	Me   string  `json:"me"`
	Room string  `json:"room,omitempty"`
	Org  *orgRef `json:"org,omitempty"`
}

// orgRef names a Linear workspace. The id is what gets compared; the name is what a
// person can act on when the comparison fails.
type orgRef struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// knownRoom is one entry in the machine's room registry, as `ettle room list` shows it.
type knownRoom struct {
	Spec string
	Me   string
	Org  *orgRef
}

// otherWorkspaces names the Linear workspaces this machine has rooms in besides the
// one given. A non-empty result is positive evidence that this person works across
// more than one workspace — the case where picking the wrong key silently builds a
// room nobody else can see, and the case a first `ettle init` cannot otherwise catch,
// because there is no prior record for that room to check against.
func otherWorkspaces(exceptID string) []string {
	seen := map[string]bool{strings.TrimSpace(exceptID): true}
	var out []string
	for _, r := range knownRooms() {
		if r.Org == nil || seen[r.Org.ID] {
			continue
		}
		seen[r.Org.ID] = true
		name := strings.TrimSpace(r.Org.Name)
		if name == "" {
			name = "id " + r.Org.ID
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
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
		out = append(out, knownRoom{Spec: strings.TrimSpace(v.Room), Me: strings.TrimSpace(v.Me), Org: v.Org})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Spec < out[j].Spec })
	return out
}
