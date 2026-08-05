# Set-and-forget: hook ettle into every Claude Code session

The point is that ettle is not a thing to remember. You install it, configure it
once, and it works in every session without a command to run. Three hooks do that,
and all are non-blocking — they never stall the agent:

- **`ettle horizon-hook`** injects the coordination tangles relevant to *you* into
  the session at start — the SURFACE half. You see them unprompted; nothing is
  posted anywhere.
- **`ettle capture-hook`** distills *this* session's own reasoning and publishes
  it as your atoms — the SEND half. Raw prose never crosses; only typed atoms do.
- **`ettle pull-hook`** ingests replies teammates post in Linear's native agent
  UI — the RECEIVE half (see the main [README](../README.md#status)).

Together: your sessions put you on the bus, teammates come in over Linear, and the
coordination surfaces in your own session — no dashboard to check. Nothing hosted,
no daemon.

## Wire it up

`ettle init <room> --install-hooks` merges all four into `~/.claude/settings.json`
for you (backing up the previous file, and skipping anything already there). To do
it by hand, merge [`settings.example.json`](settings.example.json) yourself.

Put them in `~/.claude/settings.json`, not a project's `.claude/settings.json` —
that is what makes them serve every project at once. **The commands name no room.**
Each project's `.ettle-room` (written by `ettle init`) says which room that tree
belongs to, and every hook walks up from the working directory to find it. A project
without one makes all four hooks silent no-ops, which is why one global config can
sit over every repo on the machine, ettle or not.

One thing to adjust: the `PostToolUse` `matcher` is a regex over the tool name, and
`mcp__linear` matches a server named `linear` (`mcp__linear__*`). If yours is named
differently, match that. If you don't use Linear at all, drop the two `pull-hook`
entries — `capture-hook` alone still puts you on the bus.

The session needs `ANTHROPIC_API_KEY` (capture and pull both distill locally) and,
for the Linear receive half, `LINEAR_API_KEY` (a member key — reading agent replies
needs no OAuth app token) in its environment. `ettle init` reports which of those
you have; [`docs/LINEAR_SETUP.md`](../docs/LINEAR_SETUP.md) is how to get each.

## What the hooks do

They spawn their work **detached** and return immediately, so a distill or reconcile
in the background never stalls a tool call or session exit.

`ettle horizon-hook`:

- **Fires on SessionStart** and injects the cached horizon — the tangles relevant
  to you — into the session's context, so you see them without asking. Reconcile is
  a model call, so it does NOT run in the hook: the hook injects the *cached* block
  instantly and spawns a detached `ettle horizon --cache` to refresh it for next
  time. The cache self-warms from use; the first session on a fresh room injects
  nothing and warms the cache for the next.
- **Debounces the refresh** (default 5m; `--debounce` to widen) so session churn
  doesn't spam reconciles. Identity is `--me`, else a leat room's agent, else what
  `ettle init` saved for this room on this machine, else `$USER`.
- Run `ettle horizon` yourself anytime for the live view — it reconciles the atoms
  capture/pull already put on the bus, no note files and no flags needed.

`ettle capture-hook`:

- **Fires on SessionEnd** (once, when the session closes) and, if you want
  mid-session freshness, on **Stop** (each turn). It reads the transcript path off
  the hook payload, distills the session, and publishes your atoms as you.
- **Debounces** (default 2m; `--debounce 5m` to widen) — so wiring it to Stop,
  which fires every turn, collapses a long session to the occasional distill
  instead of one per turn.
- **Publishes as you.** Same identity chain as above — `ettle init` records who you
  are per machine, so a shared `.ettle-room` never publishes your atoms under a
  teammate's name. A session that distills to no atoms publishes nothing (an empty
  envelope would erase your atoms).

`ettle pull-hook`:

- **Fires on SessionStart and after a Linear MCP tool call**, so any new teammate
  replies are already in the room by the time you look.
- **Debounces** (default 30s; `--debounce 60s` to widen). Its own marker, separate
  from capture's — a recent pull never suppresses a capture on the same room.
- **Is incremental.** It keeps a per-room cursor, so each run fetches only replies
  newer than the last.

Prefer the smallest wiring? Each works as a single backgrounded line without the
`-hook` subcommand — you lose only the debounce:

- `nohup ettle capture --room YOUR_ROOM "$transcript_path" >/dev/null 2>&1 &`
- `nohup ettle pull >/dev/null 2>&1 &`

(`capture` is the one that still needs the room named, and it must come before the
transcript path — with no target `capture` is the digest inspector, and having a
preview silently publish instead would be a surprising thing to do.)
