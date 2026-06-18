# ettle — how it actually works

ettle is easy to misread as "a shared dashboard of what everyone's doing." It is
the opposite. Three things make it unintuitive, and the diagram below is built to
make them obvious:

1. **Your raw notes are never transmitted verbatim.** Only *typed atoms* cross —
   short, structured deltas (an intent, an assumption, a commitment, a
   dependency). The panopticon version streams transcripts; ettle distills first.
   Caveat worth stating plainly: distillation is a model judgment, not a verified
   redaction — a sensitive sentence *can* be distilled into a coordination-relevant
   atom. The atom contents are the privacy surface, and (roadmap) a `--show-atoms`
   preview + structural caps are how that surface gets enforced rather than trusted.
2. **There is no shared channel humans read.** The collective layer is for the
   *agents*. Each person's own agent surfaces back to them only the knots
   relevant to *them*. You never read the team feed; your agent does.
3. **Friction is kept on purpose — but only at the cruxes.** Routine coordination
   gets bound automatically; a genuine values/priority choice is pre-staged as a
   clean either/or and handed to a human. The mesh never decides those for you.

## The flow

The flow diagram lives at the top of the [README](../README.md) (kept in one
place so the two can't drift). This page is its reading guide.

## Reading the diagram

- **The boundary is the `typed atoms only` edge.** Above it (L1) is private and
  per-person. Below it is the shared, agent-only collective layer. The privacy
  invariant is just: nothing but typed atoms crosses that edge.
- **The bus is swappable.** NATS is the default distributed rail (TLS + auth). A
  zero-infra in-process adapter runs the whole loop on one machine for testing;
  Slack / Matrix / A2A can drop in behind the same seam.
- **The L2 node is the directed-model layer.** Between the bus and the reconcile,
  each agent holds a per-pair model of every teammate (Alice-of-Bob, asymmetric),
  carried across rounds so it can go stale. A session emits only the deltas that
  would leave a teammate's model of it stale — the surprise-gated emit rule — so a
  change is routed to exactly whoever's model went stale, not broadcast. This runs
  today in its structural (deterministic) form: `ettle drift`. The single-round
  standup is its degenerate case (everyone learns everyone); the layer earns its
  keep across rounds, where it sends a couple of deltas instead of the whole state.
- **FIRM vs SOFT** is confidence propagation: a knot resting on an *inferred*
  (uncertain) atom is SOFT — surfaced as a question, not asserted as fact.
- **gemot only sees contested knots.** Most coordination is bindable and never
  reaches it. When a real values choice is at stake, gemot finds the crux and
  proposes a binding compromise; the human still makes the call. The crux is the
  most sensitive payload on the wire, so gemot is reached over TLS with auth.
- **The calibration loop is what makes speed safe.** Acting on a model of a person
  is only as good as the model stays correctable. The did-it-help signal feeds
  back so a wrong inference gets retired, not amplified.

## What runs today

The PoC (`cmd/ettle`) implements the solid path: distill → atoms → reconcile
(pairwise + team-wide) → FIRM/SOFT routing → contested knots to a resolver
(gemot, or an inline either/or) → surface to `--me`. The directed-model layer (L2)
runs in its structural form on its own command — `ettle drift` builds the per-pair
models, carries them across two rounds, and emits only the surprise-gated deltas.
The NATS bus and the gemot crux are wired behind seams; the in-process + inline
fallbacks let it run with no infrastructure at all. See the [README status](../README.md#status) for what is
built vs deliberately unbuilt, and [CONCEPT.md](CONCEPT.md) for the model and the
design invariants. To run it for a team — the bus, the gemot endpoint, the
secrets they need, and what not to turn on until calibration lands — see
[DEPLOY.md](DEPLOY.md).
