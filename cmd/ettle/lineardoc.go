package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/justinstimatze/ettle/internal/transport"
)

// `ettle linear-doc` is the narrow escape hatch a SIBLING project uses to write
// one Document per participant onto a Linear-backed room, without importing
// ettle as a Go library. pennon (github.com/justinstimatze/pennon) asked for
// this over dispatch, 2026-09-10: it wants the transport MECHANISM — one
// Document per participant, `documentUpdate` is REPLACE-CURRENT rather than a
// merge, tested and documented in internal/transport/linear.go — not ettle's
// crux/gemot/calque scope, and not a Go import that would pin pennon's own
// dependency closure against a v0.x, self-described "design stage" module.
// pennon's own read, after the alternative was pressure-tested rather than
// just proposed: "a subprocess boundary can't leak crux/gemot/calque by
// accident the way an import always could."
//
// Content is written verbatim — no ettle atom envelope wrapping (see
// transport.LinearBus.PublishDocument) — so a caller with its own document
// schema never has to match ettle's own.
func runLinearDoc(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf(`usage: ettle linear-doc upsert --room <name> --title <name> --content <text|->

  ettle linear-doc upsert    write one Document, titled --title, into the Linear
                              project for --room ("ettle-<room>"). Creates the
                              project on first use (needs LINEAR_TEAM_ID or
                              --team); every run after that finds it and ignores
                              --team. REPLACE-CURRENT: this call always
                              overwrites whatever the document held before, it
                              never merges into it.

This is the generic escape hatch for a SEPARATE project that wants ettle's
Linear Document transport without importing ettle as a Go library. ettle's own
atoms are titled with an "ettle/" prefix and never collide with a caller here
— --title must not start with "ettle/", for the same reason.`)
	}
	switch args[0] {
	case "upsert":
		return linearDocUpsert(args[1:])
	default:
		return fmt.Errorf("ettle linear-doc: unknown subcommand %q (want: upsert)", args[0])
	}
}

// linearDocResult is the --json shape, so a caller parses one thing regardless
// of outcome rather than branching on exit code plus a differently-shaped
// success line.
type linearDocResult struct {
	Success bool   `json:"success"`
	Room    string `json:"room,omitempty"`
	Title   string `json:"title,omitempty"`
	Error   string `json:"error,omitempty"`
}

func linearDocUpsert(args []string) error {
	fs := flag.NewFlagSet("linear-doc upsert", flag.ContinueOnError)
	room := fs.String("room", "", "the Linear room (maps to project \"ettle-<room>\") — required")
	title := fs.String("title", "", "the document's title, e.g. a participant name — required, must not start with \"ettle/\"")
	content := fs.String("content", "", "the document's new content, verbatim; - reads stdin — required")
	team := fs.String("team", "", "which Linear team owns the room's project, as a team key or name — only needed the first time (default: LINEAR_TEAM_ID)")
	profile := fs.String("profile", "", "which key set to read, from <config>/ettle/env.d/<name> (default: the profile recorded for this directory, or ETTLE_PROFILE)")
	asJSON := fs.Bool("json", false, `emit {"success":bool,"room":...,"title":...,"error":...} on stdout instead of prose — for a caller checking the result programmatically rather than scraping human-readable output`)
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Load this directory's key profile WITHOUT adopting a room, same reasoning as
	// `ettle capture`'s bare-form preview: --room here is an explicit flag from a
	// caller that may not even be an ettle project, so nothing about this command
	// should depend on what directory it happens to run in beyond which keys to use.
	loadProjectProfile(*profile)

	err := doLinearDocUpsert(*room, *title, *content, *team)
	if *asJSON {
		res := linearDocResult{Success: err == nil, Room: *room, Title: *title}
		if err != nil {
			res.Error = err.Error()
		}
		return emitJSON(res)
	}
	if err != nil {
		return err
	}
	fmt.Printf("ettle: wrote %q into ettle-%s.\n", *title, *room)
	return nil
}

func doLinearDocUpsert(room, title, content, team string) error {
	if strings.TrimSpace(room) == "" {
		return fmt.Errorf("--room is required")
	}
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("--title is required")
	}
	if strings.HasPrefix(title, "ettle/") {
		return fmt.Errorf("--title %q must not start with \"ettle/\" — that prefix is reserved for ettle's own atoms", title)
	}
	if content == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read --content from stdin: %w", err)
		}
		content = string(data)
	}
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("--content is required (pass - to read stdin)")
	}

	if err := applyTeamFlag("linear://"+room, team); err != nil {
		return err
	}
	key := strings.TrimSpace(os.Getenv("LINEAR_API_KEY"))
	if key == "" {
		return fmt.Errorf("no LINEAR_API_KEY — put a personal member key in %s (see docs/LINEAR_SETUP.md)", userEnvPath())
	}
	teamID := strings.TrimSpace(os.Getenv("LINEAR_TEAM_ID"))

	bus, err := transport.NewLinearBus(key, room, teamID, buildVersion(), linearWorkspaceFor(room))
	if err != nil {
		return err
	}
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return bus.PublishDocument(ctx, title, content)
}
