# Scaling & the anti-runaway design

The thing that kills a system like this isn't compute — it's a **token-burn
feedback loop**: agents emitting atoms onto the bus, each emission waking every
other agent to re-reconcile (an LLM call), reconciliation prompting more
emissions, agents burning tokens forever trying to keep up with each other. A
coordination commons has an overgrazing failure mode (every agent over-emitting)
— see [COMMONS.md](COMMONS.md). This document is how ettle is designed so that
can't happen.

## What's safe today, what isn't

Today's `ettle standup` is a **one-shot batch**: it distills, reconciles once,
prints, exits. Cost is bounded at `2N+3` model calls for N participants
(`--samples K` multiplies only the reconcile passes, a knob the user sets) — it
*cannot* run away. The risk lives entirely in the **continuous/live version**
(the deferred "production hook path"): agents emitting as they work. The rules
below are the hard requirements for that loop, written down *before* it's built
so they're designed in, not bolted on.

## The cost model

- **Distill / infer** — per emitter, cheap (Haiku), only when their own state
  changed. Scales O(emitters-who-changed), not O(team).
- **Reconcile** — the expensive shared step (it reasons over everyone's atoms).
  This is the one that must not be replicated or re-triggered carelessly.
- **Crux (gemot)** — most expensive, but reached only for *contested* knots,
  which are rare by construction. Naturally throttled.

So the whole game is: **keep the number of reconciles bounded, and keep emits
gated on genuine surprise.**

## The firewalls (structural, not just rate limits)

### 1. Atoms flow up, knots flow down — reconcile is O(1)/tick, not O(M)

The single biggest lever. Do **not** have every agent independently re-reconcile
the shared atom set (that's O(M) identical LLM calls for one answer — and it's
the one redundancy the current multi-process path actually exhibits). Instead:
one reconcile per tick (a designated reconciler — leader-elected, or a small
bus-side service), broadcasting the resulting **knot-set** back down. Each agent
then filters that knot-set to its own human locally — free, no tokens. The bus
carries atoms up and knots down; the expensive step happens once.

### 2. L3 emits knots, never atoms — the loop-breaker

**Hard invariant.** The only source of atoms is human-paced reasoning (L1).
Reconciliation produces knots *for humans to see*; it never writes a new atom
back onto the bus. This makes a machine-speed cascade **structurally
impossible**: the only thing that can inject a new atom is a human actually
deciding something, which is slow and self-limiting. A surfaced knot may
eventually cause a human to change course → a new atom — but that's
human-paced, exactly the rate the system is supposed to run at.

### 3. Surprise-gated emit, decided locally (no tokens to not-spend tokens)

An atom crosses the boundary only if it would make a teammate's model of you
*wrong* (CONCEPT.md's emit rule). The gate is a **cheap local diff** of the new
distilled atoms against what this agent last emitted — unchanged or
model-consistent state never crosses, and deciding not to emit costs nothing.
Re-emitting state nobody's model has drifted from is the primary source of
needless reconcile triggers; this kills it at the source.

### 4. Cheap deterministic pre-filter before the LLM reconcile

Before spending a reconcile call, a free check: do the new/changed atoms even
*share a subject or party* with anything else? Embedding/keyword/party overlap is
deterministic and ~free. No overlap → no possible knot → no LLM call. Only atoms
that could plausibly knot reach the model.

### 5. Content-hash skip + prompt caching

Hash the atom set feeding a reconcile; if it's unchanged since the last tick,
return the cached knots — zero tokens. The shared reconcile system prompt is
`cache_control`-marked (already true in the engine), so even a real call only
re-bills the variable atom tail, not the framing.

### 6. Debounce to quiescence ticks, not per-atom

Reconcile on a bounded cadence — batch a window of atom arrivals (or wait for
quiescence) and reconcile once per tick. Bounds reconciles to O(ticks), which is
a knob the team sets, regardless of how chatty the bus is.

### 7. The Ostrom sanction, with teeth

[COMMONS.md](COMMONS.md) frames the commons; here's the enforcement. Each agent
has an emit/reconcile **budget per window**. An agent that over-emits, or whose
flags keep scoring `false_interrupt` / low "did-it-help", gets a **tighter
budget and a longer debounce** — graduated sanction via the calibration signal.
Overgrazing self-throttles; well-calibrated agents earn more bandwidth.

### 8. Knot suppression

A knot already surfaced (or adjudicated) does not re-fire every tick. It's
suppressed until its *supporting atoms materially change* (shape-keyed, like the
adjudication sidecar). Resolved coordination doesn't keep costing.

## Why this converges

Per tick: **one** reconcile (firewall 1), only if the atom set changed (5) and
only over atoms that could knot (4); emits are bounded by surprise (3) and a
hard budget (7); reconciliation cannot trigger more emissions (2); and resolved
knots stop costing (8). Total burn per unit time is bounded by
`(ticks/period) × one-reconcile + (humans-actually-deciding) × distill` — both
human-paced and budget-capped. There is no machine-speed term. That's the whole
point: **the system runs at the speed of human decisions, not at the speed of
the bus.**

## Enforced today vs. required for the live loop

- **Today (one-shot CLI):** bounded by construction (`2N+3`, single reconcile by
  default; `--samples K` multiplies only the reconcile passes and is opt-in);
  the reconcile prompt is already cache-marked; the cost is printed in usage; L3
  already emits only knots, never atoms (firewall 2 holds in the code now).
- **Required before the live loop ships:** firewalls 1, 3, 4, 5, 6, 7, 8. None
  is built yet. The live "production hook path" in [HANDOFF.md](../HANDOFF.md) is
  explicitly gated on them — calibration-before-speed means a runaway-safe emit
  loop *before* anything emits continuously.
