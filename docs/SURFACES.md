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
   It is an **opt-in escalation**, not a default: the move you make to reach a
   teammate who isn't running ettle. Two guards keep it from being noise — it
   posts to **one dedicated coordination issue per room, never onto feature
   tickets**, and the calibration gate means only knots above a confidence bar
   emit at all (calibration-before-speed). **Unbuilt.** Needs the OAuth
   app-actor token; the emit mechanics (`agentSessionCreateOnIssue` +
   `agentActivityCreate`) are proven from a live probe but not yet in the code.

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
| **SessionStart** | pull teammates' replies **and** inject the current horizon into context, so the person sees relevant knots the instant a session opens | pull **built**; horizon-injection **unbuilt** |
| **PostToolUse(`mcp__linear`)** | pull, so touching Linear catches you up | **built** (`ettle pull-hook`, `hooks/settings.example.json`) |
| **Stop** | distill *this* session's transcript into the person's own atoms and publish them to the bus | **unbuilt** |

## The honest gap: auto-capture

Today a session's atoms reach the bus only when Claude is *told* to call the
`ettle_emit` MCP tool — that is "instructing it," which is exactly what
set-and-forget forbids. The parse half exists: `ettle capture`
(`cmd/ettle/main.go:1705`, `internal/capture`) reads a Claude Code transcript and
produces an L1 digest. What is missing is the wiring — digest → `Detector.Distill`
→ `bus.Publish` — behind a **Stop hook** that fires automatically, non-blocking,
debounced, and calibration-gated so a trivial session does not spam atoms. It is
the same detached-spawn shape as the shipped `ettle pull-hook`, pointed at the
transcript instead of at Linear.

That one hook is what makes "works in every session without being told" literally
true. Until it lands, the receive half and the bus are in place but the person's
*own* contribution still depends on an instruction.

## What this commits us to (and what it defers)

- **Default install never posts to an issue.** Whisper + bus only.
- **Escalation-emit is opt-in and lands on a dedicated coordination issue**, gated
  by calibration — deferred, and the paired follow-on to auto-capture.
- **No auto-resolution, ever** — a named invariant, not a roadmap item.
- **Consent-first holds:** the non-adopter participates from outside via Linear
  replies and opts in once they've felt the value; adoption is never pushed
  (ADOPTION.md). One-sided is the wedge; two-sided is where it earns "substrate."
