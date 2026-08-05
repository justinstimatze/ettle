# Set-and-forget: hook ettle into every Claude Code session

The point is that ettle is not a thing to remember. You install it, configure it
once, and it works in every session without a command to run. Two hooks do that,
and both are non-blocking — they never stall the agent:

- **`ettle capture-hook`** distills *this* session's own reasoning and publishes
  it as your atoms — the SEND half. Raw prose never crosses; only typed atoms do.
- **`ettle pull-hook`** ingests replies teammates post in Linear's native agent
  UI — the RECEIVE half (see the main [README](../README.md#status)).

Together: your sessions put you on the bus, teammates come in over Linear, and
the coordination surfaces in your own horizon. Nothing hosted, no daemon.

## Wire it up

Merge [`settings.example.json`](settings.example.json) into your Claude Code
settings — `~/.claude/settings.json` for every project, or a repo's
`.claude/settings.json` for just that one — then:

- Replace `YOUR_ROOM` (in all four hooks) with your room — a leat room name from
  `ettle room join`, or a `linear://<room>`.
- Adjust the `PostToolUse` `matcher` to your Linear MCP server's tool prefix. It
  is a regex over the tool name; `mcp__linear` matches a server named `linear`
  (`mcp__linear__*`). If yours is named differently, match that. (If you don't use
  Linear at all, drop the two `pull-hook` entries — `capture-hook` alone still
  puts you on the bus.)

The session needs `ANTHROPIC_API_KEY` (capture and pull both distill locally) and,
for the Linear receive half, `LINEAR_API_KEY` (a member key — reading agent replies
needs no OAuth app token) in its environment.

## What the hooks do

Both spawn their work **detached** and return immediately, so a distill in the
background never stalls a tool call or session exit.

`ettle capture-hook --room <room>`:

- **Fires on SessionEnd** (once, when the session closes) and, if you want
  mid-session freshness, on **Stop** (each turn). It reads the transcript path off
  the hook payload, distills the session, and publishes your atoms as you.
- **Debounces** (default 2m; `--debounce 5m` to widen) — so wiring it to Stop,
  which fires every turn, collapses a long session to the occasional distill
  instead of one per turn.
- **Publishes as you.** Identity is `--me`, else the room's configured agent, else
  `$USER`. A session that distills to no atoms publishes nothing (an empty
  envelope would erase your atoms).

`ettle pull-hook --room <room>`:

- **Fires on SessionStart and after a Linear MCP tool call**, so any new teammate
  replies are already in the room by the time you look.
- **Debounces** (default 30s; `--debounce 60s` to widen). Its own marker, separate
  from capture's — a recent pull never suppresses a capture on the same room.
- **Is incremental.** It keeps a per-room cursor, so each run fetches only replies
  newer than the last.

Prefer the smallest wiring? Each works as a single backgrounded line without the
`-hook` subcommand — you lose only the debounce:

- `nohup ettle capture --room YOUR_ROOM "$transcript_path" >/dev/null 2>&1 &`
- `nohup ettle pull --room YOUR_ROOM >/dev/null 2>&1 &`
