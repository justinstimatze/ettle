# ettle

A rolling shared horizon of minimized surprise for a high-trust team whose members already think through their work with AI agents.

Each person's agent models its own human (the single-user layer). It also keeps a directed model of each teammate, from its own vantage. A merged collective layer reconciles those models and surfaces the deltas that would otherwise become a surprise — a dependency someone is about to break, two people converging on the same work, an assumption one person holds that another has quietly abandoned. The aim is that coordination mostly happens before anyone notices they would have needed a meeting.

It is easy to misread as "a shared dashboard." It is the opposite: your raw notes are never transmitted verbatim — your agent distills them into typed atoms and only those cross; there is no shared channel humans read (your own agent surfaces only what's relevant to *you*); and friction is kept on purpose — but only at the genuine choices a human should own. (The distillation is a model judgment, not a verified redaction — what an atom *contains* is the real privacy surface, not the raw note. See [SECURITY.md](SECURITY.md).)

```mermaid
flowchart TB
    subgraph A["Alice's machine — L1 (private)"]
        AN["her notes / live session<br/>(capture distills the transcript)"] --> AD["her agent: distill"]
    end
    subgraph B["Bob's machine — L1 (private)"]
        BN["his notes / live session"] --> BD["his agent: distill"]
    end
    AD -- "typed atoms only" --> BUS
    BD -- "typed atoms only" --> BUS
    BUS{{"atom bus — NATS<br/>(TLS + auth)"}}
    BUS --> RC["L3 reconcile<br/>pairwise + team-wide<br/>= knot detection"]
    RC --> CONF{"knot<br/>confidence?"}
    CONF -- "FIRM &ge; 0.5" --> FIRM["worth a look"]
    CONF -- "SOFT &lt; 0.5" --> SOFT["worth a question"]
    FIRM --> CONTEST{"contested?<br/>decision-rights /<br/>team-wide divergence"}
    CONTEST -- "no — bindable" --> SURF
    CONTEST -- "yes" --> GEMOT["gemot crux<br/>positions &rarr; crux &rarr;<br/>binding compromise"]
    SOFT --> SURF
    GEMOT --> SURF
    SURF["each agent surfaces only<br/>what's relevant to ITS OWN human"]
    SURF -. "to Alice" .-> AN
    SURF -. "to Bob" .-> BN
    SURF --> CAL["did-it-help?<br/>(calibration loop)"]
    CAL -. "keeps each model<br/>correctable by its human" .-> RC
```

*Full reading guide: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).*

The name is a Scots / Northern-English verb: **to intend, to aim at, to plan or prepare ahead.** The system's job is to ettle on the team's behalf — to act on intent ahead of time — not merely to record shared state.

The aim is not "frictionless." It is **friction in the right spots**: remove it from coordination and status-sync (the bullshit-meeting toil → zero), and keep it exactly where a genuine values choice belongs to a person — surfaced as a clean, pre-staged either/or, never auto-decided by the mesh. The felt result: empowered and free of bullshit meetings, while still getting the benefit of having had a great meeting, because the mesh held it on everyone's behalf.

**What this repo is:** a runnable proof-of-concept (`cmd/ettle` — see [Quickstart](#quickstart)) *and* the design reasoning behind the larger system it's the first wedge into (the `docs/`). The CLI is what runs today; the essays are the thinking, marked clearly where they extrapolate ([HORIZON.md](docs/HORIZON.md) is explicitly the speculative end-state). If you want the tool, start with the Quickstart and [the example run](docs/EXAMPLE_RUN.md); if you want the ideas, start with [ARCHITECTURE](docs/ARCHITECTURE.md) and [CONCEPT](docs/CONCEPT.md).

Status: the coordination **engine** is built and runs — it distills typed atoms from each person's working notes, reconciles them across the team, and surfaces only the knots (collisions, duplicated work, stale assumptions, decision-rights gaps). Accuracy is not yet broadly validated — but it's now *inspectable*: `ettle eval testdata/eval/*.json` scores precision/recall against a committed synthetic corpus you can read, and `--ab` runs the honest single-shot-vs-voting comparison with a McNemar test (which, at this corpus size, correctly declines to claim voting helps). See [HANDOFF.md](HANDOFF.md). A slim, runnable **multiplayer PoC** built on that engine — something a small real team can run during an actual workday and get value from, no meeting — is the current build. Concept demos exist as local simulations on cheap models (agents standing in for the humans) to show the payoff shape; those are illustrations, not the product. The spine is [HANDOFF.md](HANDOFF.md).

## Quickstart

Requires **Go ≥ 1.25** and one Anthropic API key.

```sh
# one Anthropic API key in .env (see .env.example)
cp .env.example .env && $EDITOR .env

# surface the coordination knots across a team's notes — no meeting
go run ./cmd/ettle standup --me alice testdata/standup/*.md

# or run it on real LIVE sessions — Claude Code transcripts, not notes —
# the L1 layer that distills what each person actually reasoned about and did
go run ./cmd/ettle standup testdata/sessions/*.jsonl
go run ./cmd/ettle capture testdata/sessions/kit.jsonl   # preview what a session distills to
go run ./cmd/ettle standup --show-atoms testdata/sessions/*.jsonl   # see exactly what crosses the boundary

# useful at N=1 too: one person's own stale self-assumption
go run ./cmd/ettle standup testdata/solo/dana.md

# stabilize the stochastic detector by majority-voting across samples
go run ./cmd/ettle standup --samples 3 --me alice testdata/standup/*.md
```

Each note file is one participant (an optional `name:` / `role:` header, then
their working notes). `--me` shows only what's relevant to that person; drop it
for the full team view. Cost is ~2N+3 model calls for N participants (cheap on
Haiku); `--samples K` re-runs the reconcile passes K times and keeps only knots
that recur across a majority (the detector is stochastic — voting turns that into
a confidence signal, at +2 calls per extra sample). It's **useful at N=1**: a
single person's notes still get a self-assumption pass (an earlier assumption
their own later work has quietly made false). It runs with **no infrastructure**
— the transport defaults to in-process and contested knots fall back to an inline
either/or.

**See [docs/EXAMPLE_RUN.md](docs/EXAMPLE_RUN.md) for exactly what it prints** on
the bundled fixture — no key needed to read it.

Going distributed and secure is opt-in behind the same seams:

```sh
# atoms over a NATS bus (TLS + auth); needs the build tag
go run -tags nats ./cmd/ettle standup --transport nats --me alice notes.md

# route contested knots to a real gemot deliberation (TLS + bearer token)
go run ./cmd/ettle standup --gemot https://gemot.example/mcp ...
```

## Docs

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — **start here:** a diagram of the whole flow and the three things that make it unintuitive.
- [docs/EXAMPLE_RUN.md](docs/EXAMPLE_RUN.md) — real output on the bundled fixture (no key needed to read).
- [HANDOFF.md](HANDOFF.md) — where this stands, the critical path, next steps.
- [docs/CONCEPT.md](docs/CONCEPT.md) — the three-layer model, surprise as metaperception error, the N=1 wedge.
- [docs/N1_WEDGE.md](docs/N1_WEDGE.md) — the first buildable behavior (the prior-decision guard) and its did-it-help signal.
- [docs/TEAM_SIM.md](docs/TEAM_SIM.md) — the multiplayer payoff: agents negotiate, bind the toil, surface the cruxes. Friction in the right spots.
- [docs/HORIZON.md](docs/HORIZON.md) — the extrapolated end-state (the vision and its shadow).
- [docs/COMMONS.md](docs/COMMONS.md) — coordinated quality without wasted time as a commons; Ostrom's eight principles mapped to ettle, with graduated sanctions on gemot reputation.
- [docs/SCALING.md](docs/SCALING.md) — how the continuous version avoids a token-burn feedback loop (atoms up, knots down; L3 emits no atoms; surprise-gated emit; O(1) shared reconcile).
- [docs/PRIOR_ART.md](docs/PRIOR_ART.md) — literature and product map, with citations.
- [docs/ADOPTION.md](docs/ADOPTION.md) — consent-first, bottom-up adoption; the anti-viral stance.
- [docs/SF_LINEAGE.md](docs/SF_LINEAGE.md) — the fictional touchstones and the bright/dark fork they mark.
- [docs/NAMING.md](docs/NAMING.md) — why `ettle`, and the collisions that ruled out the alternatives.

## Relationship to sibling projects

- **the single-user layer (L1)** — ettle ships its own minimal L1: [`internal/capture`](internal/capture) distills a person's **live Claude Code session transcript** (their stated intent + the work they committed) into the same digest a note would be, so the public tool runs end-to-end on real reasoning-in-progress, not just hand-written notes (`ettle standup session.jsonl`). A richer private predecessor supplies a fuller per-person model (deeper L1 telemetry); ettle is the multiplayer extension on top — the directed and collective layers, plus the actionable layer, that the single-user layer never had.
- **the atom bus** — a [NATS](https://nats.io) bus moves typed atoms between participants' machines (TLS + auth, pub/sub, replay). Behind a transport seam, so a zero-infra in-process adapter covers local testing and other rails (Slack, Matrix, A2A) can drop in later.
- **the human-legible side** — there is no shared human channel: each person's own agent surfaces the relevant knot back to them, in-session, when helpful. You only ever see what your own agent judged relevant to you.
- **a calibration-metric store** — typed agent memory with a longitudinal metric; the natural home for scoring how well each agent's model of each teammate stays calibrated over time.
- **[gemot](https://github.com/justinstimatze/gemot)** — structured deliberation (positions → cruxes → binding compromise, with EigenTrust reputation). The inter-agent negotiation organ for *contested* knots: it locates the crux (where friction belongs) and binds the rest, and its reputation deltas become the team-tier calibration signal. Reached over TLS with auth — the crux is the most sensitive payload on the wire.
