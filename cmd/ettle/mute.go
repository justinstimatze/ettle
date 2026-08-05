package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/justinstimatze/ettle/internal/mcpserver"
	"github.com/justinstimatze/ettle/internal/tanglestate"
)

// `ettle mute` is the off switch for a tangle that shouldn't keep coming back.
//
// It exists because the surface most people install is hooks-only: `ettle init
// --install-hooks` wires the SessionStart injector and nothing else, so before this
// command the ONLY writer of the mute store was the MCP server's `ettle_respond`.
// Someone on the default install who got a wrong tangle had no way to stop it — it
// re-injected at the top of every session until the underlying atoms happened to
// change. A wrong thing you cannot silence is the fastest reason to turn a tool off,
// and this is the tool whose whole premise is interrupting you at the right moment.
//
// Muting is per-room and reversible (`--clear`), and it suppresses the tangle on both
// surfaces — horizon stops surfacing it and escalate won't post it — because both read
// the same store through the same key.

func runMute(args []string) error {
	fs := flag.NewFlagSet("mute", flag.ContinueOnError)
	room := fs.String("room", "", "the room whose tangles to mute (default: this project's `.ettle-room`)")
	transportName := fs.String("transport", "", "transport spec, when not using --room")
	me := fs.String("me", "", "who is judging (default: the room's identity, else $USER)")
	clear := fs.Bool("clear", false, "unmute instead of mute — the named tangle, or all of them with no name")
	wrong := fs.Bool("wrong", false, "the tangle is a false alarm: ettle was wrong to raise it")
	handled := fs.Bool("handled", false, "the tangle was real and is now dealt with")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	*room, *transportName = applyRoomFile(*room, *transportName)
	stateKey := roomStateKey(*room, *transportName)
	if stateKey == "" {
		return fmt.Errorf("no room: run `ettle init` in this project, or name one with --room / --transport")
	}

	key := muteKey(rest)

	switch {
	case *clear && key == "":
		return clearAllMutes(stateKey)
	case *clear:
		gone, err := tanglestate.Remove(tanglestate.Muted, stateKey, key)
		if err != nil {
			return err
		}
		if !gone {
			fmt.Printf("%s was not muted in %s — nothing changed.\n", key, stateKey)
			return nil
		}
		fmt.Printf("unmuted %s — it can surface again in %s.\n", key, stateKey)
		return nil
	case key == "":
		return listMutes(stateKey)
	}

	verdict, err := muteVerdict(*wrong, *handled)
	if err != nil {
		return err
	}
	if err := tanglestate.Add(tanglestate.Muted, stateKey, key); err != nil {
		return err
	}
	fmt.Printf("muted %s in %s — horizon will stop surfacing it and escalate won't post it.\n", key, stateKey)

	// The mute is the visible half; the verdict is the half the project actually needs.
	// A silence recorded without WHY teaches the calibration loop nothing, so this
	// writes the same label ettle_respond would, into the same log.
	lbl := mcpserver.Label{Key: key, Verdict: verdict, By: captureIdentity(*me, *room, *transportName)}
	if err := mcpserver.RecordLabel(lbl); err != nil {
		// The mute already landed and is what the human asked for; losing the label is
		// worth saying out loud and not worth failing over.
		fmt.Fprintf(os.Stderr, "ettle: muted, but the verdict was not recorded: %v\n", err)
	} else {
		fmt.Printf("recorded %q by %s in %s — the calibration signal, kept.\n", verdict, lbl.By, mcpserver.LabelsPath())
	}
	fmt.Println("undo with `ettle mute --clear " + key + "`.")
	return nil
}

// muteVerdict refuses to guess between the two reasons for silencing a tangle. They
// are opposite calibration signals: `not_real` says the detector should not have
// raised it, `handled` says it was right and the work is done. Recording one as the
// other poisons the only ground truth the calibration loop will have, and defaulting
// would do that silently every time someone typed the short form.
func muteVerdict(wrong, handled bool) (string, error) {
	switch {
	case wrong && handled:
		return "", fmt.Errorf("--wrong and --handled say opposite things about the same tangle; pick one")
	case wrong:
		return "not_real", nil
	case handled:
		return "handled", nil
	default:
		return "", fmt.Errorf("say why, so the verdict is worth something:\n" +
			"  --wrong     ettle should not have raised this (a false alarm)\n" +
			"  --handled   it was real and you have dealt with it\n" +
			"both stop it resurfacing; they are opposite signals to the calibration loop")
	}
}

// muteKey accepts either the exact key a horizon shows (`duplication|ivo+mara`) or the
// loose form a human reads off the same line (`duplication ivo mara`, or with commas).
// Both normalize through tanglestate.Key, so a mute typed by hand is the same fact as
// one recorded by the MCP server.
func muteKey(args []string) string {
	fields := make([]string, 0, len(args))
	for _, a := range args {
		for _, f := range strings.FieldsFunc(a, func(r rune) bool { return r == ',' || r == ' ' }) {
			if f = strings.TrimSpace(f); f != "" {
				fields = append(fields, f)
			}
		}
	}
	if len(fields) == 0 {
		return ""
	}
	// Exact-key form: one argument already containing the separator.
	if len(fields) == 1 && strings.Contains(fields[0], "|") {
		kind, parties, _ := strings.Cut(fields[0], "|")
		return tanglestate.Key(strings.TrimSpace(kind), strings.Split(parties, "+"))
	}
	return tanglestate.Key(fields[0], fields[1:])
}

func listMutes(stateKey string) error {
	set, err := tanglestate.Load(tanglestate.Muted, stateKey)
	if err != nil {
		return err
	}
	if len(set) == 0 {
		fmt.Printf("nothing muted in %s.\n\n", stateKey)
		fmt.Println("  ettle mute --wrong <kind> <person>...     ettle should not have raised it")
		fmt.Println("  ettle mute --handled <kind> <person>...   it was real and you dealt with it")
		fmt.Println("  ettle mute --wrong duplication ivo mara   — as it reads off the horizon line")
		return nil
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("muted in %s — these do not surface and do not escalate:\n", stateKey)
	for _, k := range keys {
		fmt.Printf("  %s\n", k)
	}
	fmt.Println("\nunmute one with `ettle mute --clear <key>`, or all with `ettle mute --clear`.")
	return nil
}

func clearAllMutes(stateKey string) error {
	set, err := tanglestate.Load(tanglestate.Muted, stateKey)
	if err != nil {
		return err
	}
	if len(set) == 0 {
		fmt.Printf("nothing muted in %s — nothing changed.\n", stateKey)
		return nil
	}
	n := len(set)
	if err := tanglestate.Save(tanglestate.Muted, stateKey, map[string]bool{}); err != nil {
		return err
	}
	fmt.Printf("unmuted %s in %s — every one can surface again.\n", plural(n, "tangle", "tangles"), stateKey)
	return nil
}
