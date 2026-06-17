# Changelog

## Unreleased

First runnable cut of the multiplayer coordination PoC.

- **Demo** — a fully-synthetic four-person team (`testdata/northwind/`, four
  Claude Code session transcripts) shown in the README as a scenario diagram plus
  a real-run transcript: the pre-meeting collision catch, bind-vs-surface (simple
  collisions FYI'd, the freeze-date divergence routed to a pre-staged crux), the
  N=1 self-assumption, and `--show-atoms` for the boundary.

- **Transport hardening** — the dev-only `--insecure-local` (plaintext/tokenless)
  gate now **resolves** the host and requires every address to be loopback
  (`internal/loopback`), instead of string-matching the hostname — a non-loopback
  name dressed up as local is rejected. The gemot client refuses to send a bearer
  token over plaintext `http://` off-box (a token in the clear is a leak), and
  after connecting with a token it verifies the session isn't gemot's anonymous
  sandbox (a bad/expired token that silently degraded), failing loud rather than
  routing cruxes into a shared sandbox while believing it's authenticated. Honest
  limit documented: loopback resolution can't see a deliberate off-box port-forward
  from a loopback bind. README now states the Go ≥ 1.25 requirement; the local
  stack docs (`deploy/`) and the gemot client doc no longer contradict on demo
  mode vs Postgres.

- **Boundary transparency + structural caps** — `ettle standup --show-atoms`
  prints exactly the typed atoms that cross (the privacy surface) before
  surfacing knots; atoms are now structurally capped (subject/content length,
  whitespace collapsed to one clause) so the boundary is partly enforced, not
  only trusted. Per-person distillation runs in parallel (latency is the "no
  meeting" competitor), and the Anthropic client retries 429/5xx (SDK-native,
  `WithMaxRetries(4)`) so a transient rate-limit doesn't abort a whole run.

- **Calibration harness** (`internal/eval`, `ettle eval`) — scores the detector's
  precision/recall against a **committed synthetic corpus** (`testdata/eval/*.json`)
  so the accuracy claim is inspectable, not gitignored. `--ab` runs single-shot
  vs multi-sample voting with a McNemar significance test; on the current corpora
  it finds no significant gain from voting (and says so rather than overclaiming).
  Fixed the voting clustering it exercises: `SameKnot` uses a Jaccard threshold
  (was: any one shared keyword) and `voteKnots` uses order-invariant union-find
  (was: order-dependent first-match).

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
