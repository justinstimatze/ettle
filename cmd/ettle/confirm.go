package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/justinstimatze/ettle/internal/mcpserver"
	"github.com/justinstimatze/ettle/internal/tanglestate"
)

// `ettle confirm` is the positive half of `ettle mute`, and it exists for the same
// reason that one does: the default install is hooks-only, so before this the ONLY
// way to record a `real` verdict was the MCP server's ettle_respond. Someone on that
// install could tell ettle it was wrong and had no way to tell it that it was right.
//
// That gap is not neutral. It is a sampling bias in the only ground truth the
// calibration loop will ever have — `ettle calibrate` can bound the false-alarm rate
// above a cut point and cannot estimate the hit rate, because the surfaces paid a
// human for one answer and not the other. Confirming buys the symmetric thing muting
// buys: the tangle stays on the horizon, because it is a live conflict, and stops
// being asked about.

func runConfirm(args []string) error {
	fs := flag.NewFlagSet("confirm", flag.ContinueOnError)
	room := fs.String("room", "", "the room whose tangles to confirm (default: the room recorded for this directory)")
	transportName := fs.String("transport", "", "transport spec, when not using --room")
	me := fs.String("me", "", "who is judging (default: the room's identity, else $USER)")
	clear := fs.Bool("clear", false, "withdraw a confirmation — the named tangle, or all of them with no name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	*room, *transportName = applyRoomFile(*room, *transportName)
	stateKey := roomStateKey(*room, *transportName)
	if stateKey == "" {
		return fmt.Errorf("no room: run `ettle init` in this project, or name one with --room / --transport")
	}

	key := muteKey(fs.Args()) // same loose-or-exact parsing; a key is a key
	switch {
	case *clear && key == "":
		return clearAllConfirmations(stateKey)
	case *clear:
		gone, err := tanglestate.Remove(tanglestate.Confirmed, stateKey, key)
		if err != nil {
			return err
		}
		if !gone {
			fmt.Printf("%s was not confirmed in %s — nothing changed.\n", key, stateKey)
			return nil
		}
		fmt.Printf("withdrew the confirmation on %s — ettle will ask about it again.\n", key)
		return nil
	case key == "":
		return listConfirmations(stateKey)
	}

	if err := tanglestate.Add(tanglestate.Confirmed, stateKey, key); err != nil {
		return err
	}
	fmt.Printf("confirmed %s in %s — it stays on the horizon and stops being asked about.\n", key, stateKey)

	// The verdict is the half the project needs, and it goes in the same log
	// ettle_respond writes so calibration data doesn't split by which surface someone
	// happened to reach for. Losing it is worth saying and not worth failing over —
	// the confirmation already landed and is what the human asked for.
	lbl := mcpserver.Label{Key: key, Verdict: "real", By: captureIdentity(*me, *room, *transportName)}
	enrichFromSurfaced(&lbl, stateKey, key)
	if err := mcpserver.RecordLabel(lbl); err != nil {
		fmt.Fprintf(os.Stderr, "ettle: confirmed, but the verdict was not recorded: %v\n", err)
	} else {
		fmt.Printf("recorded %q by %s in %s — the calibration signal, kept.\n", lbl.Verdict, lbl.By, mcpserver.LabelsPath())
	}
	fmt.Println("undo with `ettle confirm --clear " + key + "`.")
	return nil
}

func listConfirmations(stateKey string) error {
	set, err := tanglestate.Load(tanglestate.Confirmed, stateKey)
	if err != nil {
		return err
	}
	if len(set) == 0 {
		fmt.Printf("nothing confirmed in %s.\n\n", stateKey)
		fmt.Println("  ettle confirm <kind> <person>...          this one is a genuine conflict")
		fmt.Println("  ettle confirm duplication ivo mara        — as it reads off the horizon line")
		fmt.Println("\nit keeps surfacing (it's live) and stops being asked about.")
		fmt.Println("for the other direction: `ettle mute --wrong` / `--handled`.")
		return nil
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("confirmed in %s — these still surface, and ettle stops asking about them:\n", stateKey)
	for _, k := range keys {
		fmt.Printf("  %s\n", k)
	}
	fmt.Println("\nwithdraw one with `ettle confirm --clear <key>`, or all with `ettle confirm --clear`.")
	return nil
}

func clearAllConfirmations(stateKey string) error {
	set, err := tanglestate.Load(tanglestate.Confirmed, stateKey)
	if err != nil {
		return err
	}
	if len(set) == 0 {
		fmt.Printf("nothing confirmed in %s — nothing changed.\n", stateKey)
		return nil
	}
	n := len(set)
	if err := tanglestate.Save(tanglestate.Confirmed, stateKey, map[string]bool{}); err != nil {
		return err
	}
	fmt.Printf("withdrew %s in %s — ettle will ask about them again.\n", plural(n, "confirmation", "confirmations"), stateKey)
	return nil
}
