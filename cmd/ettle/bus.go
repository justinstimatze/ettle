package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/justinstimatze/ettle/internal/transport"
)

// dirBusFor handles the build-tag-INDEPENDENT transport schemes (file:// and
// leat://) so they are defined exactly once and can't drift between the nats and
// non-nats busFor copies (the dual-path bug class). Returns (bus, handled, err):
// when handled is false the caller falls through to its own inproc/nats switch.
func dirBusFor(name string) (transport.Transport, bool, error) {
	if path, ok := strings.CutPrefix(name, "file://"); ok {
		b, err := transport.NewDirBus(path)
		return b, true, err
	}
	if repoDir, ok := strings.CutPrefix(name, "leat://"); ok {
		b, err := leatBusFor(repoDir)
		return b, true, err
	}
	if room, ok := strings.CutPrefix(name, "linear://"); ok {
		b, err := linearBusFor(room)
		return b, true, err
	}
	return nil, false, nil
}

// linearBusFor builds a Linear-backed transport from linear://<room> plus the
// environment: LINEAR_API_KEY (a Linear personal API key, required) and
// LINEAR_TEAM_ID (the team that owns the room's project, required only the first
// time — to create it; ignored once it exists). The room maps to a project named
// "ettle-<room>". Meant for a team already on Linear + Claude Code, so the bus is
// a Linear project they already have rather than a git repo to stand up.
func linearBusFor(room string) (transport.Transport, error) {
	key := strings.TrimSpace(os.Getenv("LINEAR_API_KEY"))
	if key == "" {
		return nil, fmt.Errorf("linear transport needs LINEAR_API_KEY (a Linear personal API key)")
	}
	team := strings.TrimSpace(os.Getenv("LINEAR_TEAM_ID"))
	return transport.NewLinearBus(key, room, team, buildVersion())
}

// leatBusFor builds a leat git-bus transport from the repo path plus the
// environment: LEAT_AGENT (this agent's stable id == its lane filename, required),
// LEAT_REMOTE (a git remote to push/fetch, e.g. "origin"; empty = local-only),
// and ETTLE_TEAM (the room channel, default "default").
func leatBusFor(repoDir string) (transport.Transport, error) {
	agent := strings.TrimSpace(os.Getenv("LEAT_AGENT"))
	if agent == "" {
		return nil, fmt.Errorf("leat transport needs LEAT_AGENT (this agent's stable id == its lane filename)")
	}
	room := strings.TrimSpace(os.Getenv("ETTLE_TEAM"))
	if room == "" {
		room = "default"
	}
	return transport.NewLeatBus(repoDir, agent, strings.TrimSpace(os.Getenv("LEAT_REMOTE")), room)
}
