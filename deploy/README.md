# Local stack (docker)

Bring up the two services the distributed paths need — the **NATS** atom bus and
a **gemot** deliberation service — and run ettle against them. This is a **dev**
setup: NATS is plaintext and gemot runs in demo mode (in-memory, no auth), so
ettle is run with `--insecure-local`. See [SECURITY.md](../SECURITY.md) for the
real deployment shape (TLS + NATS creds + `GEMOT_REQUIRE_AUTH=1` + per-agent
gemot keys).

## Up

```sh
export ANTHROPIC_API_KEY=sk-ant-...      # gemot uses it for crux analysis
docker compose -f deploy/docker-compose.yml up -d --build
```

First run builds gemot from its pinned source tag (~1–2 min). `gemot` ends up on
`localhost:8088`, `nats` on `localhost:4222`.

## Run the full pipeline through it

```sh
# from the repo root; -tags nats enables the bus adapter
NATS_URL=nats://localhost:4222 ETTLE_TEAM=demo \
go run -tags nats ./cmd/ettle standup --me alice \
  --transport nats --gemot http://localhost:8088/mcp --insecure-local \
  testdata/standup/*.md
```

Atoms flow over the JetStream bus; the contested knots route to the local gemot
for a real crux + binding compromise (gemot's analysis takes a couple of
minutes — `--gemot-timeout` controls the wait, default 180s).

You can also exercise the bus and the crux paths independently: drop `--gemot`
to use the inline either/or, or drop `--transport nats` to keep everything
in-process.

## Down

```sh
docker compose -f deploy/docker-compose.yml down
```

Demo gemot is in-memory, so nothing persists — every `up` is a clean slate.
