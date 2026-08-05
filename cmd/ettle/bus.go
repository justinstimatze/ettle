package main

import (
	"fmt"
	"os"
	"os/exec"
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
	if spec, ok := strings.CutPrefix(name, "github://"); ok {
		b, err := githubBusFor(spec)
		return b, true, err
	}
	return nil, false, nil
}

// githubBusFor builds a GitHub-backed transport from github://<owner>/<repo>[/<room>]
// plus a token: GITHUB_TOKEN or GH_TOKEN if set, else whatever `gh auth token`
// holds — which on a developer machine is usually already there, and is the point
// of this transport over standing up a separate git-repo bus. The room maps to a
// repository Discussion titled "ettle/<room>". PRIVATE repos only; the transport
// refuses a public one (transport/github.go explains why).
func githubBusFor(spec string) (transport.Transport, error) {
	owner, repo, room, err := transport.ParseGitHubSpec(spec)
	if err != nil {
		return nil, err
	}
	tok := githubToken()
	if tok == "" {
		return nil, fmt.Errorf("github transport needs a token: set GITHUB_TOKEN, or sign in once with `gh auth login` (the `repo` scope is required)")
	}
	return transport.NewGitHubBus(tok, owner, repo, room, buildVersion())
}

// githubToken prefers an explicit env var and falls back to the gh CLI's stored
// credential, so a machine that already ran `gh auth login` needs no new secret.
func githubToken() string {
	for _, k := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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
