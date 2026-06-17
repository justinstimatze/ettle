# Example run

What `ettle standup` actually prints — no key required to read this. The input
is the synthetic fixture in [`testdata/standup/`](../testdata/standup): three
engineers (alice/bob/carol) on a shared service, each with a few lines of
working notes, with the classic coordination frictions planted (a breaking
rename, two people building the same cache, an unscheduled dependency, and a
deadline the team reads two different ways).

```
$ go run ./cmd/ettle standup testdata/standup/*.md
```

```
  ettle — coordination horizon for the team
  20 atoms across 3 people; 8 knots surfaced

  worth a look (firm)
    • [collision] GetUser→FetchUser signature change timing
      Alice commits to landing the breaking change by Thursday EOD; Bob assumes GetUser remains stable through end-of-week and has built reconciliation job on that contract, creating a Thursday collision when Alice's rename lands before Bob can adapt.
      parties: alice, bob · confidence 0.6
    • [collision] API stability vs. launch deadline pressure
      Bob's end-of-week comfort runway directly conflicts with Carol's Friday firm deadline; if Carol's Friday safety review surfaces issues requiring API changes, Bob's reconciliation job (due end-of-week) gets blocked mid-implementation.
      parties: bob, carol · confidence 0.6
    • [stale-assumption] Friday launch timeline
      Alice assumes all downstream consumers can absorb GetUser→FetchUser churn by Thursday EOD; Bob explicitly states he needs GetUser stable through end-of-week, invalidating Alice's absorption timeline.
      parties: alice · confidence 0.6
    • [decision-rights] safety review timing and Friday launch authority
      Carol lists safety review as a commitment dependency for Friday launch sign-off, but it is unscheduled; unclear whether Carol has authority to defer launch if review surfaces blockers, or whether someone else owns that go/no-go call.
      parties: carol, ? · confidence 1.0
      → crux (inline): safety review timing and Friday launch authority
        ↳ safety review timing and Friday launch authority as carol frames it
        ↳ safety review timing and Friday launch authority as the other party frames it

  worth a question (soft — rests on an inference)
    • [collision] cache implementation ownership and visibility
      Alice depends on existing user-service/cache/ being discoverable and usable; Carol is simultaneously building a separate user-lookup-cache in notifications repo, risking divergent cache layers.
      parties: alice, carol · confidence 0.4
    • [duplication] user-service caching layer
      Alice depends on existing user-service/cache/ for enriched reads; Carol is building a separate user-lookup-cache in notifications repo rather than extending or consuming it, creating two parallel cache implementations instead of one shared layer.
      parties: alice, carol · confidence 0.4
    • [teamwide-divergence] GetUser API stability through launch
      Alice is breaking GetUser's signature this week; Bob and Carol both assume GetUser stays stable through Friday launch and beyond, blocking their work streams.
      parties: alice, bob, carol · confidence 0.4
```

(The `stale-assumption` knot with a single party — `parties: alice` — is the
**self pass**: Alice's own stated commitment to absorb the churn by Thursday
contradicts an assumption her plan rests on. No teammate is needed to surface it;
see the N=1 run below.)

## How to read it

- **worth a look (firm)** — knots resting on what people actually stated
  (confidence ≥ 0.5): the breaking-change collision, the deadline conflict, the
  single-party self-assumption, the unscheduled-review authority gap — all
  surfaced before anyone shipped, no meeting.
- **the crux** — a `decision-rights` (or `teamwide-divergence`) knot is
  *contested*: a real authority/values call, not a positive-sum coordination. It
  routes to a resolver and is pre-staged as an either/or for a human, rather than
  the mesh deciding it. Here the inline fallback frames the branches; with
  `--gemot` it becomes a real deliberation with a binding compromise (below).
- **worth a question (soft)** — knots that depend on an *inferred* assumption
  (here, the unstated deadlines), at confidence 0.4. ettle surfaces these as
  questions, not facts — friction in the right spot.
- **`--me alice`** would show only the knots involving Alice — her agent
  surfacing to her, not a shared feed everyone reads.

The detector is stochastic, so the exact wording, the firm/soft split, and which
party-pairs a recurring knot lands on vary run to run; the dominant real knots
(the breaking-change collision, the cache duplication, the deadline divergence)
recur every run.

## Stabilizing the noise: `--samples`

`--samples K` runs the reconcile passes K times and keeps only knots that recur
across a majority, stamping each with how many samples it appeared in — the
run-to-run variance becomes a confidence signal:

```
$ go run ./cmd/ettle standup --samples 3 --me alice testdata/standup/*.md

  ettle — coordination horizon for alice
  20 atoms across 3 people; 4 knots surfaced

  worth a look (firm)
    • [collision] GetUser signature stability
      Alice is renaming GetUser→FetchUser by Thursday; Bob assumes GetUser signature remains stable through end of next week and has direct dependency on it.
      parties: alice, bob · confidence 0.6 · seen in 3/3 samples
    • [decision-rights] API breaking-change authority
      Alice is committing to ship the rename by Thursday as fait accompli; Bob treats GetUser stability as a non-negotiable precondition, with no stated agreement on who decides whether the breaking change proceeds.
      parties: alice, bob · confidence 0.6 · seen in 2/3 samples
      → crux (inline): API breaking-change authority
        ↳ ...

  worth a question (soft — rests on an inference)
    • [collision] cache implementation locus
      Alice depends on the user-lookup cache in user-service; Carol is building the cache in notifications — two different locations, unclear which is authoritative.
      parties: carol, alice · confidence 0.5 · seen in 3/3 samples
```

The robust collisions land `3/3`; the flakier decision-rights framing lands
`2/3`. A one-off hallucination that shows up in a single sample falls below the
majority and is dropped. Cost is K× the reconcile passes, so it is opt-in.

## Useful at N=1: a single person's own stale assumption

The pairwise and team passes are blind to a knot inside one person's own plan, so
a self pass catches it — which is what makes ettle useful with one note file:

```
$ go run ./cmd/ettle standup testdata/solo/dana.md

  ettle — coordination horizon for the team
  6 atoms across 1 person; 1 knot surfaced

  worth a question (soft — rests on an inference)
    • [stale-assumption] retry logic scope
      Dana assumes the existing retry logic (designed for synchronous charge failures) is compatible with the async billing queue, but her own dependency statement that billing moved to async events implies the retry logic scope must expand beyond its original design — a change not yet reflected in her stated assumption.
      parties: dana · confidence 0.4
```

Dana's notes never mention a teammate; the knot is the gap between an assumption
she's relying on (synchronous billing) and a decision she made later (async
queue). Run-to-run it routes firm or soft depending on whether it leans on the
inferred deadline — `--samples` would stabilize that too.

## With gemot: a contested knot becomes a binding compromise

Run with `--gemot <url>` and the contested knots (decision-rights, team-wide
divergence) route to a real [gemot](https://github.com/justinstimatze/gemot)
deliberation instead of the inline either/or. From a live local run against
gemot (the whole stack on docker — see [deploy/](../deploy)), the cache-ownership
decision-rights knot came back as a scored crux and a concrete binding
compromise:

```
    • [decision-rights] cache-layer ownership and design authority
      ...neither has stated who decides the cache contract.
      parties: carol, alice · confidence 0.6
      → crux (gemot): Building the user-lookup cache inside the notifications
        repository — rather than extending the existing cache/ component in
        user-service — tends to create stronger long-term maintainability risks
        by fragmenting caching logic across service boundaries...
        controversy 1.00
        proposed: The user-service team (Alice) will extend the existing cache/
        component to expose a thin, versioned FetchUser cached-read interface,
        delivered by Wednesday EOD. The notifications-service (Carol) consumes
        this shared layer rather than building a local cache... Carol retains
        ownership of a config interface for TTL and cache-size hints. Neither
        team may unilaterally change the shared interface contract without a
        documented, agreed change process. ...ships within the Friday deadline.
```

That's the bind-vs-surface split working: the routine knots are surfaced for a
human to glance at; the genuine ownership choice gets a real deliberation with a
controversy score and a binding proposal, with the human still the decider.
