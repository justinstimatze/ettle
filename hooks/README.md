# Pull teammates' Linear replies automatically

`ettle pull` ingests replies that teammates post in Linear's native agent UI and
turns them into that teammate's atoms in the tangle (see the main
[README](../README.md#status)). This hook runs it for you, so it isn't a thing to
remember: it fires when a session starts and after the session uses a Linear MCP
tool, and by then any new teammate replies are already distilled into the room.

## Wire it up

Merge [`settings.example.json`](settings.example.json) into your Claude Code
settings — `~/.claude/settings.json` for every project, or a repo's
`.claude/settings.json` for just that one — then:

- Replace `YOUR_ROOM` with your Linear room (the `<room>` in `linear://<room>`).
- Adjust the `PostToolUse` `matcher` to your Linear MCP server's tool prefix. It
  is a regex over the tool name; `mcp__linear` matches a server named `linear`
  (`mcp__linear__*`). If yours is named differently, match that.

The session needs `LINEAR_API_KEY` (a member key — reading agent replies needs no
OAuth app token) and `ANTHROPIC_API_KEY` (replies distill locally) in its
environment, the same as `ettle pull` itself.

## What the hook does

`ettle pull-hook --room <room>`:

- **Never blocks the agent.** It spawns `ettle pull` detached and returns
  immediately — a distill happening in the background never stalls a tool call.
- **Debounces.** A burst of Linear tool calls collapses to one pull (default 30s;
  `--debounce 60s` to widen). A pull with nothing new is one cheap query anyway,
  so firing often is fine.
- **Is incremental.** Pull keeps a per-room cursor, so each run only fetches
  replies newer than the last.

Prefer the smallest wiring? A single line works without the subcommand — just
background `ettle pull` directly in your hook command:
`nohup ettle pull --room YOUR_ROOM >/dev/null 2>&1 &`. You lose only the debounce.
