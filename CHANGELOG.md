# Changelog

## Unreleased

First runnable cut of the multiplayer coordination PoC.

- **L1 live-session capture** (`internal/capture`, `ettle capture`) — distills a
  person's real Claude Code session transcript (their stated intent from prompts
  + the work they committed via Edit/Write/Bash; exploration like Read/Grep and
  subagent sidechains are skipped) into the same digest a hand-written note would
  be. `ettle standup session.jsonl` runs the whole pipeline on **live
  reasoning-in-progress**, not after-the-fact artifacts — the thesis the design
  rests on. The digest stays local; only the distilled atoms cross. Synthetic
  session fixtures in `testdata/sessions/`.

- **`ettle standup`** — distills each participant's notes into typed atoms,
  reconciles them (pairwise + team-wide + a single-party self pass) into
  coordination knots, and surfaces only what's relevant to each human (`--me`).
  Routes FIRM knots as "worth a look", SOFT (inference-backed) as "worth a
  question".
- **Useful at N=1** — a single-party **self-assumption** pass (`ReconcileSelf`)
  surfaces an assumption a person's own later work has quietly made false; the
  pairwise/team passes are blind to it by construction. Deduped against the
  cross-person knots (shared `SameKnot` matcher) so a team-wide divergence isn't
  also reported privately.
- **Multi-sample voting** (`--samples K`, `ReconcileVoted`) — re-runs the
  reconcile passes K times and keeps only knots recurring across a majority,
  turning the stochastic detector's run-to-run noise into a confidence signal
  (each surviving knot carries `Votes`/`Samples`, kept separate from
  Confidence). Clustering uses the same `SameKnot` matcher, so a knot relabeled
  collision→decision-rights across runs still votes as one. Default `K=1` is the
  original single-run cost.
- **Transport seam** — in-process (default, zero infrastructure) and a NATS
  distributed bus (`-tags nats`, TLS + credentials enforced off localhost).
  - The NATS adapter uses **JetStream** (retained stream): a publish-before-collect
    flow over core pub/sub would race and silently drop a peer's atoms;
    retention removes the race. Covered by an embedded-server integration test
    (in CI) and a live three-process docker run.
- **Crux seam** — contested knots route to a gemot deliberation (TLS + bearer
  token, refuses anonymous off localhost) or an infra-free inline either/or.
  Validated live against gemot 0.13.1: a decision-rights knot produced a scored
  crux + binding compromise. gemot poll default 90s → 180s (`--gemot-timeout`)
  after its multi-round analysis outran the old timeout.
- **Safeguards** — `--me` validated against the roster; collected-vs-published
  participant count asserted (no silent partial "all clear"); resolver errors
  surfaced; output-truncation warning; prompt-injection guard in the prompts.
- **Local stack** — `deploy/docker-compose.yml`: NATS (JetStream) + gemot
  (demo mode) in one `docker compose up`, run ettle against it with
  `--insecure-local`.
- **Scaling design** — `docs/SCALING.md`: the anti-runaway firewalls for the
  future continuous loop (L3 emits knots not atoms; surprise-gated emit; O(1)
  shared reconcile; per-agent budget). The production hook path is gated on them.
- **Project** — MIT LICENSE, SECURITY.md, architecture diagram + example run,
  synthetic fixture, parser/loader + NATS tests, CI (both build configs),
  git-tag-derived `ettle version`.
