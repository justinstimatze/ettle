# Linear setup — the four keys, and what each one buys

`ettle init <room>` tells you which of these you have and which you don't. This page
is what to do about the ✗ lines. Two of the four are required; the other two each
unlock one specific thing, so a missing one is a feature that's off, not a broken
install.

**Joining a room somebody else already set up?** Two of the four below are already
somebody else's problem, and one of the three paths in needs no key at all. Start at
[JOINING.md](JOINING.md) and come back here only for the key you are actually missing.

## First: what a room is, and how to pick one

A **room** is the space a group of people coordinate in. One room per group who
should see each other's work — not one per repo, not one per person, not one per
session. It maps to a Linear project named `ettle-<room>`, holding one document per
participant.

Choosing is a one-time decision, and the only one your team has to make together:

- **Pick a short name the team already uses for itself** — `crew`, `platform`,
  `payments`. It becomes a visible project name in your Linear workspace.
- **Everyone types the identical string.** `crew` and `Crew` are two different
  rooms. The failure is quiet — each of you sits in a room seeing only yourself,
  looking like ettle has nothing to say. Don't retype it: a successful `ettle init`
  prints `tell a teammate: ettle init linear://crew` under **next**, and that
  fully-qualified form resolves to the same room as the bare name. Send that line.
- **One room can span several repos.** Point `ettle init --dir` at a shared parent and
  every checkout beneath it resolves the same room; the room is about the people, not
  the code. Nothing is written into any repo — the mapping is per-machine, in
  `~/.config/ettle/rooms.json`, because a room pointer inside a repo would enrol
  whoever clones it into a room they never chose.
- **You are not stuck with it.** Re-running `ettle init <newroom>` repoints the
  project. The atoms already published stay in the old project, so re-point early
  rather than after a week of history you would rather keep.

On the **GitHub** path you skip this entirely: run `ettle init` with no argument
inside a checkout and the room is derived from the `origin` remote, so two teammates
cannot land in different rooms by typing different names.

| Variable | Who needs it | What it buys |
|---|---|---|
| `LINEAR_API_KEY` | everyone running ettle | The atom bus (a Linear project's documents) and reading teammates' replies. A personal member key. |
| `ANTHROPIC_API_KEY` | everyone on the hook path — see below | Distilling your notes into typed atoms and reconciling the room. Both run **on your machine** — your raw prose never leaves it. |
| `LINEAR_TEAM_ID` | the first person in the room | Creating the room's project. Ignored once the project exists. |
| `LINEAR_AGENT_TOKEN` | nobody, until you escalate | Posting a tangle onto the coordination issue so a teammate who doesn't run ettle can see it. An OAuth **app-actor** token; the member key cannot post agent activities. One person holds it, not one per teammate. |

**Where to put them: `~/.config/ettle/env`**, one `KEY=VALUE` per line, `chmod 600`.
(Working across more than one Linear workspace? See [More than one
workspace](#more-than-one-workspace) below — one global file cannot serve two.)
Every ettle command reads it, which is the point — the Claude Code hooks inherit
whatever environment the session was launched with, so a key you export in one
terminal is invisible to them. An explicit environment variable still wins if you
prefer to export. (`.env.example` lists all four.)

## `ANTHROPIC_API_KEY`

<https://console.anthropic.com/settings/keys>. Whether this is one key per room or one
per person depends on which path you are on, and the difference catches people out:

- **The hook path** (`ettle init --install-hooks`, the lead path) needs **one per
  person**. `ettle capture` and `ettle horizon` both call the model on your own
  machine and fail without a key, so every teammate running the hooks has their own.
  `ettle init` reports it as required for this reason.
- **The MCP path** needs **none at all**. `ettle mcp` runs key-free: the agent already
  in the session does the distilling and calls `ettle_emit` with the atoms, so a
  teammate driving ettle from their own agent never mints one.

## `LINEAR_API_KEY` — a personal member key

Linear → Settings → Security & access → Personal API keys → **New API key**. This is
the key the docs-as-bus uses; it reads and writes documents in one project and reads
agent activities. It acts as **you**, which is why it cannot post as the ettle agent.

## `LINEAR_TEAM_ID` — only to create the room's project

The room `crew` maps to a Linear project named `ettle-crew`. The first `ettle init`
in a room creates it and needs to know which team owns it; every run after that finds
the existing project and ignores this variable entirely.

Get it from the API with the member key:

```sh
curl -s https://api.linear.app/graphql -H "Authorization: $LINEAR_API_KEY" \
  -H 'Content-Type: application/json' \
  --data '{"query":"query{ teams(first:20){ nodes{ id key name } } }"}'
```

## `LINEAR_AGENT_TOKEN` — the OAuth app-actor token

This is the one that takes ten minutes, and it is worth knowing exactly what it is
for before you spend them: **escalation only.** Without it everything else works —
the bus, the horizon in your session, capture, pull. What you lose is `ettle escalate`
and the `ettle_escalate` MCP tool, which post a tangle onto the room's one coordination
issue for teammates who never installed ettle. If everyone on the team runs ettle,
you never need this token.

It has to be an app-actor token because Linear's agent activities are posted *by an
application*, not by a person. A member key authenticates as you, and Linear will
not let you write an agent activity as yourself.

**1. Create an OAuth application.** Linear → Settings → API → OAuth applications →
create one. Set the callback URL to `http://localhost:8787/callback`. Keep the
**Client ID** and **Client secret**. You need workspace admin to install it.

**2. Authorize it with `actor=app`.** Open this URL in a browser, substituting your
client ID and any random string for `state`:

```
https://linear.app/oauth/authorize
  ?response_type=code
  &client_id=YOUR_CLIENT_ID
  &redirect_uri=http%3A%2F%2Flocalhost%3A8787%2Fcallback
  &state=SOME_RANDOM_STRING
  &scope=read,write,app:assignable,app:mentionable
  &actor=app
```

(One line, no spaces.) `actor=app` is the whole point: it makes the resulting token
act as the application rather than as you. `app:assignable` and `app:mentionable`
give the agent a real presence teammates can @-mention and assign to. Approving
redirects to `localhost:8787/callback?code=…` — copy the `code`.

**3. Exchange the code for the token**, within a few minutes (codes expire):

```sh
curl -s -X POST https://api.linear.app/oauth/token \
  -d grant_type=authorization_code -d code=THE_CODE \
  -d redirect_uri=http://localhost:8787/callback \
  -d client_id=YOUR_CLIENT_ID -d client_secret=YOUR_CLIENT_SECRET
```

The `access_token` in the response is `LINEAR_AGENT_TOKEN`. It is sent as a
`Bearer` token, unlike the member key, which Linear takes raw — ettle handles that
distinction for you (`internal/transport/linear.go`).

Confirm it authenticated as the app and not as you:

```sh
curl -s https://api.linear.app/graphql -H "Authorization: Bearer $LINEAR_AGENT_TOKEN" \
  -H 'Content-Type: application/json' --data '{"query":"query{ viewer{ name } }"}'
```

The name should be your application's, not yours.

## More than one workspace

A Linear member key is scoped to **one workspace**, so a single global
`~/.config/ettle/env` cannot serve two. Name a **profile** instead:

```
ettle init crew --profile work
```

That records the profile for this directory alongside its room, and reads its keys from
`~/.config/ettle/env.d/work` — one file per workspace, however many projects share it.
The profile is a **name, not a secret**, so the line stays as safe to commit as `room`
already is; the keys never leave your machine. A project with no `profile` line
behaves exactly as before.

Four places a value can come from, in order:

1. an explicit environment variable (an `export` always wins)
2. the named profile, `~/.config/ettle/env.d/<name>`
3. the global `~/.config/ettle/env`
4. a `.env` in the working directory — `ANTHROPIC_API_KEY` only

A profile only needs the keys that *differ*; anything it omits keeps the global value.
If your teammate names their profile something else, `ETTLE_PROFILE` overrides the
committed line per machine.

**The guard.** Pointing a project at the wrong workspace used to fail silently in the
worst way: the key simply cannot see `ettle-<room>`, so ettle would **create a second
project of that name** in the wrong workspace, and the teammate you meant to reach
would never see it. `ettle init` now records which workspace a room was found in, and
a later run holding a different workspace's key is refused by name rather than
creating anything. Two honest limits:

- On a room's **first** `ettle init` there is nothing recorded yet, so nothing is
  checked. What protects you there is the report line naming the workspace it
  resolved — read it before you carry on.
- The record is per-machine and keyed by room, so it protects *your* machine. A
  teammate's first init is their own first init.

## What ettle touches in your workspace

Worth knowing before you point this at a real workspace:

- **One project per room**, named `ettle-<room>`, holding **one document per
  participant** (`ettle/<name>`). This is the bus. It carries typed atoms only —
  never your raw notes.
- **One issue per room**, titled `ettle coordination`, and only if you escalate.
  Tangles are posted there as agent activities. **Never onto your feature tickets** —
  that separation is a design commitment, not a default (`docs/SURFACES.md`).
- **Nothing else.** No webhooks, no server, no background daemon. The receive path
  polls with a cursor; there is nothing hosted anywhere.
