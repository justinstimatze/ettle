package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/justinstimatze/ettle/internal/transport"
)

// `ettle teams` answers the one setup question the tool could not: which team owns
// the room's project.
//
// LINEAR_TEAM_ID is a uuid, Linear shows that uuid on no screen, and the documented
// way to get it was a curl with the key interpolated into the shell. That instruction
// is broken for anyone following ettle's own convention — keys live in
// <config>/ettle/env so the hooks can read them, which means they are NOT exported,
// so the header comes out empty and Linear answers 401. Reaching for a shell variable
// to fix it is how a key ends up in shell history.
//
// So ettle asks on your behalf, with the key it already loads, and prints the ids
// next to the keys and names you can actually see in the app.
func runTeams(args []string) error {
	fs := flag.NewFlagSet("teams", flag.ContinueOnError)
	profile := fs.String("profile", "", "which key set to read (default: the profile recorded for this directory, or ETTLE_PROFILE)")
	asJSON := fs.Bool("json", false, "emit JSON instead of a table — for an agent driving the setup")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// Load this project's key profile. Without it the command reads the GLOBAL key and
	// lists the wrong workspace's teams — in a project with `profile = work`, which is
	// the only situation the command matters. Worse than an error if both workspaces
	// happen to have an ENG: no error, wrong team.
	loadProjectProfile(*profile)
	key := strings.TrimSpace(os.Getenv("LINEAR_API_KEY"))
	if key == "" {
		return fmt.Errorf("no LINEAR_API_KEY — put a personal member key in %s (see docs/LINEAR_SETUP.md)", userEnvPath())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	teams, err := transport.LinearTeams(ctx, key, buildVersion())
	if err != nil {
		return err
	}
	if len(teams) == 0 {
		return fmt.Errorf("this key's workspace has no teams visible to it")
	}

	// Name the workspace too. A key is scoped to exactly one, and "which workspace am
	// I actually talking to" is the question underneath most setup confusion here.
	ws, wsErr := transport.LinearWorkspace(ctx, key, buildVersion())

	if *asJSON {
		return emitJSON(map[string]any{"workspace": ws.Name, "teams": teams})
	}
	if wsErr == nil && ws.Name != "" {
		fmt.Printf("\n  teams in %s\n\n", ws.Name)
	} else {
		fmt.Print("\n  teams\n\n")
	}
	for _, t := range teams {
		fmt.Printf("    %-10s %s\n      %s\n", t.Key, t.Name, t.ID)
	}
	fmt.Printf("\n  Pass either form to init — the key is easier and it resolves itself:\n")
	fmt.Printf("    ettle init <room> --team %s\n\n", teams[0].Key)
	return nil
}

// emitJSON is the machine mode every setup command offers, so an agent driving the
// setup branches on values rather than parsing a table.
func emitJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
