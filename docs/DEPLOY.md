# Deploying ettle in your org

This is the operational counterpart to [ARCHITECTURE.md](ARCHITECTURE.md): how
you actually run ettle for more than one person on one laptop, what
infrastructure that implies, and — just as important — **what you should not turn
on yet.**

Read the honesty note first, because it changes how you deploy.

## The honesty note (read before standing anything up)

ettle is a design-stage PoC. The **detector** (the fast people-modeling half)
runs; the **calibration loop** that keeps each model correctable, and the
**continuous live-emit** path, are deliberately unbuilt (see the
[README status](../README.md#status) and the
[design invariants](CONCEPT.md#design-invariants-non-negotiable)).

Two of those invariants are operational constraints, not slogans:

- **Calibration before speed.** Until the calibration loop exists, the emit-gate
  is uncalibrated — and we have measured what that means: on an independent-work
  corpus where the correct horizon is *empty*, ettle still surfaces 1–2 firm
  knots, and the surfaced set is only ~25% stable run-to-run (model-invariant;
  `ettle eval [--stability]` on `testdata/eval/independent-work.json`). So a
  deployment that **auto-broadcasts** knots to a shared channel would over-graze
  the attention commons ([COMMONS.md](COMMONS.md)). Run it in *surface-to-`--me`*
  mode — each person sees their own horizon, on demand — not as an always-on
  firehose, until calibration lands.
- **No machine-speed feedback loop.** L3 emits no atoms; the reconcile is O(1)
  shared work, not a per-message fan-out ([SCALING.md](SCALING.md)). Don't wire
  ettle's output back into an automated actor.

So "deploying in your org" today means **running the batch `standup` / `drift`
flow over a shared, secured substrate (a synced folder or a bus), on demand**,
with humans as the deciders — not an autonomous mesh. The seams below are real and tested; the always-on product
on top of them is not built.

## Tier 0 — one team, no infrastructure (the default)

Nothing to deploy. The transport defaults to in-process and contested knots fall
back to an inline either/or, so a single run reads everyone's notes locally and
prints each person's horizon:

```sh
go run ./cmd/ettle standup --me alice path/to/notes/*.md
```

This is also the right mode for a first org trial: collect the team's notes (or
session transcripts — `*.jsonl`, distilled by `capture`), run it on one machine,
share the per-person output. No bus, no service, no secrets. `--show-atoms`
shows exactly what would cross the boundary (typed atoms, never the raw note).

## Tier 1 — a shared folder (no broker)

When agents run on different machines but the team already shares a folder
(Dropbox / Google Drive / git / Syncthing), point each at it with
`--transport file://<path>` — multiplayer with **no server to run**. Each
participant's agent writes only its own file under `<path>/.ettle/`; reconcile
reads the folder. Securing and replicating the folder is the sync tool's job, and
only boundary-distilled atoms cross — never the raw notes.

```sh
# each teammate, on their own machine, pointed at the same synced folder:
go run ./cmd/ettle standup --me alice --transport file://$HOME/Dropbox/team-x notes.md
```

| Property | Behavior |
|---|---|
| Storage | replace-current — each file holds that person's latest atoms (clean exit = delete your file; no history pile-up, so the longitudinal-leak surface isn't amplified) |
| Identity | the filename is authoritative; that a file is really its owner's rests on the folder's access control — a convention, **not** structurally enforced (per-envelope signing is reserved, not built) |
| Freshness | every run prints a roster + per-member staleness, so a partially-synced horizon is never read as a bare "all clear" |
| Conflicts | sync conflict-copies (Dropbox/Syncthing markers) are skipped; an undetected one (OneDrive/Drive numbering) surfaces as a visible extra roster member, not silent corruption |

**When this fits:** the batch model — run `standup` on demand and read whatever has
synced. Eventual-consistency lag (seconds–minutes) is fine here; it would only bite
a continuous live-emit loop, which is deliberately unbuilt ([SCALING.md](SCALING.md)).
**Honest limit:** a teammate whose file never reached your copy of the folder is
invisible — coverage reports who/how-stale among files *present*, not against an
out-of-band roster. For real identity + an audit trail with the same no-server
model, use the leat git-repo bus below; for low-latency exchange or a hard
membership guarantee, use the NATS bus.

## Tier 1b — a private git repo (leat, no server)

The git-native upgrade of the shared folder: point each agent at a **private git
repo** with `--transport leat://<local-clone>`. Still **no server to run** — your
git host (GitHub / GitLab / self-hosted) already provides auth, TLS, and
replication. leat ([github.com/justinstimatze/leat](https://github.com/justinstimatze/leat),
a stdlib-only Go package owned by mcp-dispatch, which ettle consumes) treats the
repo as an append-only, per-author-lane bus: each agent only ever appends to the
one lane file it owns (`channels/<team>/<agent>.jsonl`), so concurrent pushes are
always fast-forwards and never conflict. Sharing the repo URL is the whole
onboarding — it's the invite.

**Easiest — `ettle room`.** It does the boring parts (clone, seed a HEAD, remember
the repo path / agent / remote) so day-to-day use is just `standup --room`:

```sh
# first person starts the room (creates + seeds the repo); the URL is the invite:
go run ./cmd/ettle room init git@github.com:payments/standup-room.git
# everyone else joins on their own machine:
go run ./cmd/ettle room join git@github.com:payments/standup-room.git --as alice
# then, no env vars or paths to remember:
go run ./cmd/ettle standup --room standup-room --me alice notes.md
# presence — who's in the room and what each is working on (no knots, no model call):
go run ./cmd/ettle room status standup-room
# (ettle room list shows configured rooms; --as sets your lane id, default $USER)
```

**Or wire it by hand** (what `--room` resolves to — a `leat://` clone plus three
env vars):

```sh
# each teammate, on their own machine, against a clone of the same private repo:
export LEAT_AGENT=alice          # this agent's id == its lane filename == commit author
export LEAT_REMOTE=origin        # the git remote to push/fetch (omit for local-only)
export ETTLE_TEAM=payments       # the room channel (isolates this team's lanes)
go run ./cmd/ettle standup --me alice --transport leat://$HOME/clones/team-x notes.md
```

| Property | Behavior |
|---|---|
| Storage | append-only per-author lanes; `Collect` folds the latest atoms per participant (LWW). `git log` of a lane is the **audit trail** — and the basis for the future drift/provenance feature |
| Identity | **hardened**: a line whose author != its lane owner is dropped as a spoof (surfaced via warnings) — stronger than the folder tier's filename-convention, though still resting on git-host access control, not per-line signatures (reserved) |
| Transit | git over HTTPS/SSH — TLS + auth come from the host; only boundary-distilled atoms cross, never raw notes |
| Membership | the room repo *is* the boundary — who can clone/push is who's in the room |

**When this fits:** the same batch model as the folder tier — run `standup` on
demand against whatever has been fetched — but when you want non-spoofable
identity and a durable, replayable record (which the folder tier can't give) at
zero server cost. **Honest limit:** like the folder, it's pull-based — "freshness"
is last-fetch, so it's async-standup latency, not live; and `leat://` expects an
existing local clone (clone the room repo first).

## Tier 2 — a shared atom bus (NATS)

When agents run on different machines and need low-latency atom exchange (or a
membership guarantee the folder can't give), build with the
`nats` tag and point each at a shared NATS server. **TLS and auth are enforced**
— `DialNATS` refuses to connect without credentials unless you explicitly opt
into the localhost-plaintext path, and `--insecure-local` is rejected for any
non-loopback URL.

```sh
export NATS_URL=tls://nats.internal.example:4222   # tls:// for anything off-box
export NATS_CREDS=/etc/ettle/team.creds            # NATS user credentials file
export ETTLE_TEAM=payments                          # isolates this team's atom stream
go run -tags nats ./cmd/ettle standup --transport nats --me alice notes.md
```

| Setting | Env | Notes |
|---|---|---|
| Bus URL | `NATS_URL` | `tls://…` off-box; bare `nats://` only on loopback |
| Credentials | `NATS_CREDS` | path to a NATS creds file; **required** unless `--insecure-local` on loopback |
| Team | `ETTLE_TEAM` | names the JetStream stream; different teams = different streams (a boundary, per [COMMONS.md](COMMONS.md) principle 1) |

For a local dry-run against a docker NATS with no TLS:

```sh
go run -tags nats ./cmd/ettle standup --transport nats --insecure-local --me alice notes.md
```

`--insecure-local` resolves the host and refuses anything that isn't actually
loopback, so a remote server dressed up as `localhost` can't be run unauthed.

## Tier 3 — contested knots to a gemot deliberation

Most coordination is bindable and never leaves the mesh. When a knot is a real
values/priority choice (a *crux*), route it to a [gemot](https://github.com/justinstimatze/gemot)
deliberation instead of resolving it inline. gemot sees the most sensitive
payload on the wire, so it's reached over TLS with a bearer token.

```sh
export ETTLE_GEMOT_TOKEN=…                          # bearer token for the endpoint
go run ./cmd/ettle standup \
  --gemot https://gemot.internal.example/mcp \
  --gemot-timeout 180s \
  --me alice notes.md
```

Empty `--gemot` keeps the inline either/or (no external service). A contested
knot can spend minutes in deliberation, hence the generous default timeout.
gemot's EigenTrust reputation is also where the commons' **graduated sanctions**
land ([COMMONS.md](COMMONS.md) principle 5) — the anti-overgrazing teeth — but
that wiring is roadmap, not shipped.

## Security posture (what protects the team)

- **The boundary is a typed-atom digest, never the raw note or transcript.** What
  crosses is a small set of typed atoms; inspect it with `--show-atoms`.
- **Per-person privacy override.** A `private:` line in a note lists phrases kept
  off the boundary — enforced in two layers (prompt suppress-list + deterministic
  scrub), covering inferred atoms too.
- **Transport refuses to be insecure off-box.** No creds → no connection; plaintext
  → loopback only, host-resolved.
- **Team isolation.** `ETTLE_TEAM` separates streams; no state about a
  non-participant enters any horizon ([COMMONS.md](COMMONS.md) principle 1).

## What you cannot deploy yet (and shouldn't fake)

- **No always-on mesh / live-emit.** The continuous path is gated on the
  anti-runaway requirements in [SCALING.md](SCALING.md) and is unbuilt. Deploy the
  batch flow, run on demand.
- **No calibration loop.** Emit thresholds aren't yet tuned to your team, so
  expect over-emission and run-to-run drift; keep a human in front of the output
  and don't auto-route it anywhere. This is the single biggest reason not to wire
  ettle into anything automated today.
- **Adoption is consent-first and bottom-up** ([ADOPTION.md](ADOPTION.md)): every
  participant opts in, and the tool is useful at N=1, so you never need to
  mandate it org-wide to get value.
