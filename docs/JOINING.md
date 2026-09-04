# Joining a room someone else set up

`ettle init <room>` is the same command for the first person in a room and the tenth.
What differs for you is that everything already exists — the project, the team, the
people — so most of what [LINEAR_SETUP.md](LINEAR_SETUP.md) explains is a decision
somebody already made. This page is the joiner's short path, and the list of what you
can skip.

## First: you may not need to install anything

Three ways to be in a room, in increasing order of what they ask of you. Pick the
lowest one that gets you what you want; you can move up later without redoing anything.

| | What you install | What you need | What you get |
|---|---|---|---|
| **Reply in Linear** | nothing | nothing | Your written replies in the room are distilled under your own identity when a teammate runs `ettle pull`. You take part without a binary or a key. |
| **`ettle mcp`** | the binary | a Linear key | Your agent drives ettle through MCP tools. The agent already in your session does the distilling, so **no Anthropic key** — see [LINEAR_SETUP.md](LINEAR_SETUP.md#anthropic_api_key). |
| **The hooks** | the binary | a Linear key and an Anthropic key | Set-and-forget. Nothing after setup is a command you run. The rest of this page. |

[ADOPTION.md](ADOPTION.md) puts that first row at the centre of the design: what you
consent to is writing in the room, and running the binary is a separate choice on top
of it. If you only ever answer in Linear, the room still works.

## Who does what

Two things only the room's owner can do. Everything else is yours, in your accounts,
and stays there.

**The owner adds you to the team, if that team is private.** Linear has no per-project
visibility — a project's audience is the audience of the team that owns it, and privacy
is a setting on the team. ettle creates `ettle-<room>` in whichever team the first
person named, so if that team is private you have to be a member of it; being in the
workspace is not enough. Worth them checking before you start, because the failure is
misleading: a project a key cannot see is indistinguishable from one that is not there,
so every command below reports the room as missing and you go hunting for a typo in its
name. If the team is open to the workspace, there is nothing for them to do.

**You mint your own keys.** A Linear personal API key acts as you and can only be
created by you; the same is true of an Anthropic key. Nobody can create either one for
you, and both live in a file on your own machine. What actually gets sent to you is one line: the room, in
the form `ettle init linear://<room>` that a successful `ettle init` prints under
**next**.

## The path

1. **Install.** `go install github.com/justinstimatze/ettle/cmd/ettle@latest`, then
   `ettle version` to confirm it is on your path.

2. **Run init once and read what it asks for.** In the directory you work in, or a
   parent of it if you want every checkout beneath it in the same room:

   ```sh
   ettle init linear://<room> --me <you> --install-hooks
   ```

   It prints a checklist of what is present and what is missing, names the exact file
   the keys belong in, and is safe to re-run as often as you like — it picks up where it
   stopped rather than starting over. `--me` is how your atoms are attributed to you,
   so pick what teammates should see. `--install-hooks` merges the hooks into
   `~/.claude/settings.json` and writes a `.bak` first; drop it to print the JSON and
   merge it yourself.

3. **Make your Linear key.** Settings → Security & access → Personal API keys → **New
   API key**.

4. **Make your Anthropic key.** <https://console.anthropic.com/settings/keys>. The
   distilling runs on your machine against your key, which is exactly why your raw
   session text never has to leave it — and it is also why the usage bills to you.
   It is a small model over a compact digest covering only the turns since the last
   run — cheap, but it lands on your bill, so it is worth knowing it is there.

5. **Put both in the file init named.** One `KEY=VALUE` per line, then `chmod 600`.
   Take the path from init's own output rather than guessing it — it is
   `~/.config/ettle/env` on Linux and under `~/Library/Application Support/` on a Mac.
   It has to be a file: the Claude Code hooks inherit whatever environment the session
   was launched with, so a key you export in a terminal is invisible to them.

6. **Re-run init, then restart your session.** Every line should come back ✓. The hooks
   load when a session starts, so one already open will not have them.

## What you can skip

Three things [LINEAR_SETUP.md](LINEAR_SETUP.md) covers at length that do not apply to
somebody joining an existing room. Named here so you do not go looking for them.

| | Why not |
|---|---|
| `LINEAR_TEAM_ID` | Only the first person in a room needs it, to create the project. Once `ettle-<room>` exists it is ignored entirely. |
| `LINEAR_AGENT_TOKEN` | The one step that costs real time, and it buys escalation only — posting a tangle onto a shared issue for people who do not run ettle. One person in a room holds it, if anyone does. |
| `--profile` | For a machine working across more than one Linear workspace, keeping a separate key set per project. Skip it unless that is you. |

## After it is on

The tangles that involve you are injected at the start of a session. As you work, your
session distills and publishes into your own document. Teammates' Linear replies arrive
the same way. Nothing is posted anywhere shared unless you escalate it on purpose,
and the whole bus is readable in Linear like any other project — documents, not a black
box. [SURFACES.md](SURFACES.md) says which output lands where.

To see it by hand at any point: `ettle horizon --me <you>`. To stop: delete the ettle
hooks from `~/.claude/settings.json`. To leave entirely, delete your document from the
project: presence is explicit and revocable, which [ADOPTION.md](ADOPTION.md) treats as
a requirement rather than a courtesy — while being honest that there is no `ettle leave`
yet, so today it is a thing you do by hand.
