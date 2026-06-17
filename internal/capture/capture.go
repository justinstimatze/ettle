// Package capture is ettle's L1 layer: it turns a person's LIVE agent session
// (a Claude Code transcript) into the raw material the detector distills into
// typed atoms — replacing the hand-written standup note with what the person
// actually reasoned about and did.
//
// This is the load-bearing piece. ettle's thesis is "model people from their
// reasoning-in-progress, not from after-the-fact artifacts." A markdown note IS
// an after-the-fact artifact; a session transcript is the reasoning-in-progress.
// Capture extracts two signals, following the same rule inkling's observe layer
// uses: the person's STATED INTENT (their prompts — what they're trying to do
// and the decisions they voice) and the WORK THEY COMMITTED (file edits and
// shell commands — Edit/Write/Bash, the actions that passed a human's
// permission; Read/Grep/etc. are the agent exploring, not the human deciding).
//
// Privacy boundary, unchanged: the Digest produced here stays LOCAL — it is the
// note-equivalent. Only the typed atoms the detector distills from it ever
// cross. Capture is deliberately lossy (prompts truncated, exploration dropped):
// it is a digest, not a transcript dump.
package capture

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Session is the extracted L1 signal from one person's transcript.
type Session struct {
	Branch  string
	Prompts []string // the person's stated intent / decisions, most recent last
	Edits   []string // distinct files they (via the agent) edited
	Cmds    []string // distinct shell verbs they ran
}

const (
	maxPrompts   = 10  // keep the most recent reasoning; older context has decayed
	maxPromptLen = 320 // a digest, not a transcript — truncate long prompts
	maxEdits     = 24
	maxCmds      = 16
)

// transcript line (only the fields we read).
type tline struct {
	Type        string          `json:"type"`
	IsSidechain bool            `json:"isSidechain"`
	GitBranch   string          `json:"gitBranch"`
	Message     json.RawMessage `json:"message"`
}

type msg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type block struct {
	Type  string         `json:"type"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

// Read parses a Claude Code transcript JSONL into a Session. It skips sidechain
// (subagent) lines, tool-result messages, and harness noise, keeping the
// human's prompts and the agent's committed actions.
func Read(path string) (Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return Session{}, err
	}
	defer f.Close()

	var s Session
	editSeen, cmdSeen := map[string]bool{}, map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 256*1024), 8*1024*1024) // transcript lines can be large
	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var t tline
		if err := json.Unmarshal(line, &t); err != nil {
			continue // tolerate malformed/non-message lines
		}
		if t.IsSidechain {
			continue // a subagent's work, not this person's
		}
		if t.GitBranch != "" {
			s.Branch = t.GitBranch
		}
		var m msg
		if len(t.Message) == 0 || json.Unmarshal(t.Message, &m) != nil {
			continue
		}
		switch t.Type {
		case "user":
			// Real human prompts are string content. Array content is a
			// tool_result (the agent's output coming back), not the human.
			var content string
			if json.Unmarshal(m.Content, &content) != nil {
				continue
			}
			if p := cleanPrompt(content); p != "" {
				s.Prompts = append(s.Prompts, p)
			}
		case "assistant":
			var blocks []block
			if json.Unmarshal(m.Content, &blocks) != nil {
				continue
			}
			for _, b := range blocks {
				if b.Type != "tool_use" {
					continue
				}
				switch b.Name {
				case "Edit", "Write", "MultiEdit", "NotebookEdit":
					if p := baseName(filePath(b.Input)); p != "" && !editSeen[p] {
						editSeen[p] = true
						s.Edits = append(s.Edits, p)
					}
				case "Bash":
					if v := bashVerb(b.Input); v != "" && !cmdSeen[v] {
						cmdSeen[v] = true
						s.Cmds = append(s.Cmds, v)
					}
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		return Session{}, err
	}
	// Keep the most recent prompts (the live reasoning); truncate each.
	if len(s.Prompts) > maxPrompts {
		s.Prompts = s.Prompts[len(s.Prompts)-maxPrompts:]
	}
	if len(s.Edits) > maxEdits {
		s.Edits = s.Edits[:maxEdits]
	}
	if len(s.Cmds) > maxCmds {
		s.Cmds = s.Cmds[:maxCmds]
	}
	return s, nil
}

// Digest renders the session as a compact note the detector can distill — the
// same shape a hand-written standup note would have. Stays local; only the
// distilled atoms cross.
func (s Session) Digest() string {
	var b strings.Builder
	if s.Branch != "" {
		fmt.Fprintf(&b, "Working session on branch %q.\n\n", s.Branch)
	}
	if len(s.Prompts) > 0 {
		b.WriteString("Stated intent and decisions (from their own prompts):\n")
		for _, p := range s.Prompts {
			fmt.Fprintf(&b, "- %s\n", p)
		}
		b.WriteString("\n")
	}
	if len(s.Edits) > 0 {
		fmt.Fprintf(&b, "Files they worked on: %s\n", strings.Join(s.Edits, ", "))
	}
	if len(s.Cmds) > 0 {
		fmt.Fprintf(&b, "Commands they ran: %s\n", strings.Join(s.Cmds, ", "))
	}
	return strings.TrimSpace(b.String())
}

// Empty reports whether the session yielded no usable L1 signal.
func (s Session) Empty() bool {
	return len(s.Prompts) == 0 && len(s.Edits) == 0 && len(s.Cmds) == 0
}

// cleanPrompt drops harness-injected noise (slash-command wrappers, local
// command stdout, interrupt markers, system reminders) and truncates. Returns
// "" for a prompt that is only noise.
func cleanPrompt(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	noise := []string{
		"<local-command", "<command-name>", "<command-message>", "<command-args>",
		"<bash-", "Caveat:", "[Request interrupted", "<system-reminder>",
	}
	for _, n := range noise {
		if strings.HasPrefix(s, n) {
			return ""
		}
	}
	// Strip any inline system-reminder block, then re-trim.
	if i := strings.Index(s, "<system-reminder>"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if s == "" {
		return ""
	}
	s = strings.Join(strings.Fields(s), " ") // collapse whitespace
	if len(s) > maxPromptLen {
		s = s[:maxPromptLen] + "…"
	}
	return s
}

func filePath(in map[string]any) string {
	for _, k := range []string{"file_path", "filePath", "path", "notebook_path"} {
		if v, ok := in[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func baseName(p string) string {
	if p == "" {
		return ""
	}
	if i := strings.LastIndexAny(p, "/\\"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// bashVerb returns the first effective command token, skipping env wrappers
// (sudo, env, FOO=bar) and a leading `cd dir &&`. Pipelines/&&-chains keep the
// first verb, which is the right call for the canonical case.
func bashVerb(in map[string]any) string {
	cmd, _ := in["command"].(string)
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ""
	}
	fields := strings.Fields(cmd)
	for i := 0; i < len(fields); i++ {
		tok := fields[i]
		switch {
		case tok == "sudo" || tok == "env":
			continue
		case strings.Contains(tok, "="): // FOO=bar wrapper
			continue
		case tok == "cd":
			// skip "cd dir &&"
			for i < len(fields) && fields[i] != "&&" {
				i++
			}
			continue
		}
		return tok
	}
	return ""
}
