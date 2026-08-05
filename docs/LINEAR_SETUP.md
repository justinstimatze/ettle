# Linear setup — the four keys, and what each one buys

`ettle init <room>` tells you which of these you have and which you don't. This page
is what to do about the ✗ lines. Two of the four are required; the other two each
unlock one specific thing, so a missing one is a feature that's off, not a broken
install.

| Variable | Required? | What it buys |
|---|---|---|
| `ANTHROPIC_API_KEY` | yes | Distilling your notes into typed atoms and reconciling the room. Both run **on your machine** — your raw prose never leaves it. |
| `LINEAR_API_KEY` | yes | The atom bus (a Linear project's documents) and reading teammates' replies. A personal member key. |
| `LINEAR_TEAM_ID` | first run only | Creating the room's project. Ignored once the project exists, so only the first person in a room needs it. |
| `LINEAR_AGENT_TOKEN` | no | Escalation only — posting a knot onto the coordination issue so a teammate who doesn't run ettle can see it. An OAuth **app-actor** token; the member key cannot post agent activities. |

Put them wherever your shell reads env vars, or in a `.env` beside the binary
(`.env.example` lists all four). They are read once per command; nothing is stored.

## `ANTHROPIC_API_KEY`

<https://console.anthropic.com/settings/keys>. One key per room is enough, not one per
person: a teammate driving ettle from their own agent distills client-side and never
needs one (`ettle mcp` runs key-free — see the README).

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
and the `ettle_escalate` MCP tool, which post a knot onto the room's one coordination
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

## What ettle touches in your workspace

Worth knowing before you point this at a real workspace:

- **One project per room**, named `ettle-<room>`, holding **one document per
  participant** (`ettle/<name>`). This is the bus. It carries typed atoms only —
  never your raw notes.
- **One issue per room**, titled `ettle coordination`, and only if you escalate.
  Knots are posted there as agent activities. **Never onto your feature tickets** —
  that separation is a design commitment, not a default (`docs/SURFACES.md`).
- **Nothing else.** No webhooks, no server, no background daemon. The receive path
  polls with a cursor; there is nothing hosted anywhere.
