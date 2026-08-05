# Surfaces — where ettle meets Claude Code and Linear

This is the reading guide for *how a person actually touches ettle*: what they
install, what they configure once, and which of ettle's outputs land in which
place. It exists because "put the knot in Linear" is the obvious design and the
wrong default, and the reasons are easy to lose.

The governing requirement, stated the way a user states it: **install one thing,
configure it once, and have it work in every session without being told to.** No
per-session `ettle` command, no "remember to run pull." Everything below is
measured against that.

## Whisper-first: the default surface is the person's own session

A detected coordination knot surfaces **privately, to the person it's relevant
to, inside their own Claude Code session** — the horizon they already hold. It
does not post anywhere shared by default. Alice sees "heads up, this contradicts
Bob's stateless note" in her session; *she* decides whether it's worth raising
with Bob.

This is the contextual-privacy invariant and the humans-stay-the-point invariant
doing their job (CONCEPT.md §Design invariants). The mesh routes each person the
tangles relevant to them; it does not broadcast a team feed, and it does not
resolve the conflict. **Auto-resolution is off the table on purpose** — no
machine-speed feedback loop, humans stay the deciders. The value of two people
running ettle is *earlier detection and each-sees-their-own-slice*, so they
resolve it in one exchange today instead of at a broken deploy Thursday. It is
never that the tool decides for them.

## The three Linear roles (they are not the same surface)

Linear shows up in three distinct ways. Conflating them is what makes "ettle
pollutes my tickets" sound true when it isn't.

1. **`LinearBus` — the atom wire.** `linear://<room>`
   (`internal/transport/linear.go`) writes each participant's atoms as a Linear
   **project document** titled `ettle/<slug>` — docs-as-bus. It is storage for
   typed atoms, never a comment on a work ticket, and it carries *only* atoms
   (the privacy boundary; raw prose never crosses). Both Alice and Bob point
   their ettle at the same `linear://<room>`; each `Collect`s the whole set of
   docs and reconciles its own private horizon. No server, no issue touched.
   **Built.**

2. **`ettle pull` — the non-adopter on-ramp.** A teammate who never installs
   ettle replies in Linear's native agent UI; `ettle pull` reads that reply
   (`internal/transport/linearagent.go`), distills it *locally* under that
   teammate's identity, and publishes their atoms to the bus. This is how a
   non-participant contributes a voice to reconcile without adopting anything.
   **Built** (member key only — reading agent activities needs no OAuth app
   token). See `hooks/README.md`.

3. **Escalation-emit — surfacing a knot onto an issue.** The *only* role that
   writes onto Linear, and therefore the only one the pollution worry is about.
   It is an **opt-in escalation**, not a default: `ettle escalate --room <room>`
   is the deliberate move you make to reach a teammate who isn't running ettle.
   Two guards keep it from being noise — it posts to **one dedicated coordination
   issue per room ("ettle coordination" in `ettle-<room>`), never onto feature
   tickets**, and it emits only **firm cross-person knots** (firm *is* the
   calibration gate — a knot below the recurrence bar never posts). Idempotent: a
   knot already posted is skipped. **Built** (`ettle escalate`,
   `internal/transport/linearescalate.go`): it authenticates as the OAuth
   **app actor** (`LINEAR_AGENT_TOKEN`, Bearer — the member key can *read* agent
   activities but not post them), does `agentSessionCreateOnIssue` +
   `agentActivityCreate`, and the reply flow closes back via `ettle pull`.

### The same three roles on GitHub

A team on GitHub rather than Linear gets role 1 in the same shape: the room is a
repository **Discussion** titled `ettle/<room>`, each participant owns one comment
carrying their envelope, and identity rides a `<!-- ettle:<name> -->` marker
(`internal/transport/github.go`, `github://<owner>/<repo>[/<room>]`). It needs no
new secret — the credential `gh auth login` already stored is enough — and that is
the whole reason it exists next to the git-repo bus, which needs a separate repo
created, cloned, and seeded first. **Built.**

Two differences from Linear, both deliberate. **Private repositories only, enforced
at construction and with no override flag.** A public repo's Discussions are readable
by anyone on the internet; a Linear project is workspace-scoped and a private repo's
Discussion is collaborator-scoped, which are comparable audiences, so this is a
difference in kind rather than degree, and the contextual-privacy boundary is a
design invariant rather than a default. The residual risk a check cannot close: a
repo that is private today can be made public tomorrow and the history goes with it,
so the guard runs on every construction and fails the next publish loudly. **And
roles 2 and 3 have no GitHub equivalent yet** — pull and escalate ride Linear's agent
activities, which is a surface GitHub has no counterpart for. A GitHub team gets the
bus and the whisper; reaching a non-adopter through a comment thread is unbuilt. That
is why `ettle init` installs only three hooks for a GitHub room: wiring `pull-hook`
and a `PostToolUse` matcher on a Linear MCP server the team doesn't run would be two
hooks that can only ever no-op.

Naming a room is friction the GitHub path doesn't need, so it doesn't have it: run
`ettle init` with no argument inside a checkout and the room is derived from the
`origin` remote. That is worth more than the keystrokes — the failure it removes is
two teammates typing different names and each sitting alone in a room they think the
other is in.

**The payoff of the split:** if both people install ettle, role 3 is never
needed. Role 1 (bus) + the whisper (in-session horizon) is the entire loop, all
through project documents, and the default install touches no issue. Emit-onto-an
-issue exists *only* as the bridge to someone who won't install the thing.

## Set-and-forget: what "install once" actually is

The thing a person installs:

- the `ettle` binary on PATH,
- a **hook bundle merged into global `~/.claude/settings.json` once** — the
  mechanism that makes it session-agnostic, because hooks there fire in every
  session and every project with no per-session instruction,
- config once: the keys (`LINEAR_API_KEY` + `ANTHROPIC_API_KEY`) in the shell
  profile — genuinely once — and the room carried with the project in a
  `.ettle-room` file at its root, so it is never re-specified.

`ettle init <room>` does all three but the binary, and the room file is what makes
the global hook config coherent: because the hook commands name no room, the *same*
four lines serve every repo on the machine, resolve the right room in each, and stay
silent no-ops in the ones that aren't ettle projects. The file holds only the room —
a fact about the project, safe to commit. Identity is a fact about the person and is
kept per-machine, because a committed `me = alice` would publish Bob's atoms as
Alice's, which is exactly the misattribution the transport works to prevent.

Three hooks close the instruction-free loop:

| Hook | Job | Status |
|------|-----|--------|
| **SessionStart** | pull teammates' replies **and** inject the current horizon into context, so the person sees relevant knots the instant a session opens | **built** (`ettle horizon-hook` injects a cached reconcile; `ettle pull-hook` pulls) |
| **PostToolUse(`mcp__linear`)** | pull, so touching Linear catches you up | **built** (`ettle pull-hook`, `hooks/settings.example.json`) |
| **SessionEnd / Stop** | distill *this* session's transcript into the person's own atoms and publish them to the bus | **built** (`ettle capture-hook` → `ettle capture --room`) |

Getting them installed is itself a step, so it is one command: `ettle init <room>
--install-hooks` merges the four into `~/.claude/settings.json`, idempotently (a
re-run adds nothing), never joining a group someone else's hooks live in, and backing
up the previous file first. Keys are checked and reported rather than assumed —
[LINEAR_SETUP.md](LINEAR_SETUP.md) is what to do about each ✗.

## Auto-capture (the send half of set-and-forget)

Before, a session's atoms reached the bus only when Claude was *told* to call the
`ettle_emit` MCP tool — "instructing it," which is exactly what set-and-forget
forbids. Now `ettle capture --room <room>` (`cmd/ettle/capture.go`) distills a
Claude Code transcript locally — reusing the parse in `internal/capture` and
`Detector.Distill` — and publishes the atoms as you; `ettle capture-hook` fires it
from a **SessionEnd** hook (once per session) and optionally **Stop** (each turn,
debounced), detached so it never blocks the agent. Only typed atoms cross; the raw
transcript stays local. A session that distills to no atoms publishes nothing, so
an empty session never wipes your existing atoms off the bus.

With capture, pull, and horizon-injection all wired, the loop closes with no
command to run: your sessions put you on the bus (capture), teammates come in over
Linear (pull), and the knots relevant to you appear in your next session
unprompted (horizon-injection). Because reconcile is a model call, the injection
never runs it inline — `ettle horizon-hook` injects a *cached* reconcile instantly
at SessionStart and spawns a detached refresh for next time, so session start stays
free and instant while the horizon stays warm. The injected block is written as an
instruction to its actual reader — the agent — and carries the same knot state as the
MCP `ettle_horizon`: muted knots suppressed, un-shared cross-person knots flagged so
the agent offers to escalate exactly what a teammate can't see. Whisper-first holds
end to end: the knot surfaces privately in your own session, and nothing is posted
anywhere.

## Operating it from inside a session (the MCP tools)

The hooks are the passive, set-and-forget layer — they run without anyone driving
them. The *active* layer is the agent operating ettle mid-session through MCP tools
(`ettle mcp --transport linear://<room>`), because an agent uses tools far more
reliably than it remembers to shell out to a CLI. Three affordances make the agent an
effective operator rather than a forgetful one:

- **`ettle_horizon`** tags each surfaced knot `escalated: true/false` and suppresses
  the ones the human has muted (with an honest `muted` count) — so the agent offers
  to escalate only what a teammate can't already see, and stops re-raising what's
  resolved.
- **`ettle_escalate`** posts one knot (by `key`) to the coordination issue — the
  agent offers it to the human, and on yes escalates that knot inline, no CLI recall.
- **`ettle_respond`** with `not_real`/`handled` **mutes** the knot so it stops
  re-surfacing. That's what lets the agent make a knot go away when the human says
  it's settled, instead of nagging about it every session.

The shared `internal/knotstate` package is why these compose with the CLI: it keys a
knot the same way and holds the per-room escalated/muted sets, so an escalation or a
mute from either surface is honored by both.

## What this commits us to (and what it defers)

- **Default install never posts to an issue.** Whisper + bus only.
- **Escalation-emit is opt-in and lands on a dedicated coordination issue**, gated
  to firm cross-person knots — built (`ettle escalate`), and only ever the bridge to
  a teammate who won't install ettle. The default install still never posts.
- **No auto-resolution, ever** — a named invariant, not a roadmap item.
- **Consent-first holds:** the non-adopter participates from outside via Linear
  replies and opts in once they've felt the value; adoption is never pushed
  (ADOPTION.md). One-sided is the wedge; two-sided is where it earns "substrate."
