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
  profile — genuinely once — and the room carried with the project via a
  `.ettle` file in the repo root, so it is never re-specified.

Three hooks close the instruction-free loop:

| Hook | Job | Status |
|------|-----|--------|
| **SessionStart** | pull teammates' replies **and** inject the current horizon into context, so the person sees relevant knots the instant a session opens | **built** (`ettle horizon-hook` injects a cached reconcile; `ettle pull-hook` pulls) |
| **PostToolUse(`mcp__linear`)** | pull, so touching Linear catches you up | **built** (`ettle pull-hook`, `hooks/settings.example.json`) |
| **SessionEnd / Stop** | distill *this* session's transcript into the person's own atoms and publish them to the bus | **built** (`ettle capture-hook` → `ettle capture --room`) |

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
free and instant while the horizon stays warm. Whisper-first holds end to end: the
knot surfaces privately in your own session, and nothing is posted anywhere.

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
