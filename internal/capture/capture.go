// Package capture is ettle's L1 layer: it turns a person's LIVE agent session
// (a Claude Code transcript) into the raw material the detector distills into
// typed atoms — replacing the hand-written standup note with what the person
// actually reasoned about and did.
//
// This is the load-bearing piece. ettle's thesis is "model people from their
// reasoning-in-progress, not from after-the-fact artifacts." A markdown note IS
// an after-the-fact artifact; a session transcript is the reasoning-in-progress.
// Capture extracts two signals: the person's STATED INTENT (their prompts —
// what they're trying to do
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
	"regexp"
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
	s, _, err := ReadFrom(path, 0)
	return s, err
}

// ReadFrom is Read starting at line `from`, returning the session and the total line
// count of the file. It exists so a long-running session is not re-digested in full
// every couple of minutes: a capture distills only the turns added since the last one,
// and `read` is the offset to hand back next time.
//
// A `from` past the end means the transcript was replaced or truncated under us — a
// fresh session reusing a path, say — so the whole file is read rather than silently
// producing nothing.
func ReadFrom(path string, from int) (Session, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return Session{}, 0, err
	}
	defer f.Close()

	var s Session
	editSeen, cmdSeen := map[string]bool{}, map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 256*1024), 8*1024*1024) // transcript lines can be large
	total := 0
	for sc.Scan() {
		total++
		if total <= from {
			continue // already distilled in an earlier capture
		}
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
		return Session{}, 0, err
	}
	if from > 0 && from >= total {
		// The file shrank or was replaced — a fresh session reusing the path, say.
		// Starting over beats silently publishing nothing.
		s2, n2, err := ReadFrom(path, 0)
		return s2, n2, err
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
	return s, total, nil
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
		// A background task finishing is delivered as a user turn, but it is the
		// harness talking, not the person. Left in, it reads as their stated intent
		// and drags a whole dispatch payload into the digest.
		"<task-notification>",
		// The compaction preamble is a summary of a conversation, addressed to the
		// model. Distilling it re-states old context as if it were new intent.
		"This session is being continued from a previous conversation",
	}
	for _, n := range noise {
		if strings.HasPrefix(s, n) {
			return ""
		}
	}
	// A bare slash command (`/compact`, `/check-plan foo`) is an instruction to the
	// harness, not a statement about the work. Matched on the FIRST TOKEN only, and
	// only when it has no second slash, so a message that opens with a path
	// ("/etc/hosts is wrong") is still the person talking.
	if strings.HasPrefix(s, "/") {
		if first, _, _ := strings.Cut(s, " "); isSlashCommand(first) {
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

// slashCommandRe matches a bare harness command like `/compact` or `/check-plan`:
// one leading slash then word characters, `-` or `:`. A path fails it on the second
// slash, so a message opening with "/etc/hosts" is still the person talking.
var slashCommandRe = regexp.MustCompile(`^/[\w:-]+$`)

func isSlashCommand(tok string) bool {
	return slashCommandRe.MatchString(strings.TrimSpace(tok))
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
