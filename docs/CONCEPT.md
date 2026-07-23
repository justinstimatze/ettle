# ettle — the concept

ettle is, fundamentally, about helping humans ambiently work together better. The topology is a torus, not a hierarchy: humans talk to agents, agents talk to agents, agents talk back to humans — and the loop is *required to return*. The agent↔agent interior (the gemot, the bindable subset reconciled below) exists only to close the human-to-human loop faster; it is instrumentation, never a destination. Travel "up" the abstraction ladder — delegate to your agent, your agent deliberates with mine — and you do not arrive somewhere new, you come back down to two humans coordinating. This is the discipline that separates ettle from the opposite pole, the fully-autonomous ("zero-human") corporation ([PRIOR_ART.md](PRIOR_ART.md) §8): the distinction is not "ettle has humans and they don't" but *where the loop closes*. A zero-human corp tries to close agent→agent→agent with no return; ettle's loop must return to a person. Everything below is the mechanism for making that return cheap, calibrated, and honest.

## The premise: which parts of a meeting actually die

The bottleneck in a high-trust team where everyone already works through their reasoning with an AI agent is humans telling other humans things their agents already know. But a meeting does several jobs, and only one is information sync:

1. **Information sync** — "here's what I changed / am blocked on / you should know." This is the redundant part; agents can collapse it to near-zero.
2. **Preference aggregation, commitment, conflict surfacing** — whose priority wins, who is now on the hook, where a latent disagreement becomes visible. This is not information transfer; it is politics among people with divergent interests, and it does not vanish because the agents are well-informed.

So the honest claim is not "no meetings." It is: the sync meeting dies, and the decision meeting gets shorter and sharper because nobody arrives uninformed. The bottleneck relocates to intent not yet expressed in any artifact, and to commitment, which is a speech-act rather than an inference. ettle shrinks both but does not abolish them.

## The three-layer model

The structure comes from the interpersonal-perception literature: in a dyad there is each person's relation to themselves, each person's relation to the other from their own vantage, and the relation of the pairing itself. Mapped onto a team of agents, ettle is three layers, not one flat shared pool.

**L1 — self-models.** Each session's model of its own user: what they are working on, assuming, leaning toward; what they have corrected or closed before. This is the single-user layer, one model per person — N of them. Private. (The internal working model of self, in attachment terms.) ettle ships a minimal version of this layer — `internal/capture` distills a live Claude Code session — and a richer per-person model can feed it from outside this repo.

**L2 — directed dyadic models.** Alice's session's model *of Bob*. Asymmetric, and there are N×(N−1) of them: Alice-of-Bob is not Bob-of-Alice. This is the metaperspective — what I believe you are assuming, leaning toward, about to do. It is Alice's belief about Bob, not Bob's actual state. A single-user model has no analogue here, because it has only one author. (The *structural* form of this layer — the per-pair model as a set of belief-atoms, carried across rounds, with the surprise-gated emit and the staleness diff below — is built and deterministic: `ettle drift`. The *semantic* enrichment, an agent inferring what Bob is assuming beyond the atoms he stated, is not yet — see [README status](../README.md#status).)

**L3 — the collective layer.** The merged horizon. The team-as-entity, held by no individual, where reconciliation and conflict-detection live.

## Surprise is metaperception error

The payoff of the layering is that "minimize surprise" gets a definable *shape* (though not yet a measured quantity):

> Surprise = the divergence between each person's L2 model-of-another and that other person's L1 model-of-self.

This is a type signature with a first, structural metric now under it: the divergence over typed atoms runs as a deterministic slot-diff of each L2 model against the subject's L1 (`ettle drift`), so "surprise" has a *computed* value — the per-pair emit delta — not just a name. What stays unbuilt is the *semantic* version: a divergence and threshold over meaning rather than typed slots, which is where naming it "surprise" still borrows a word from active inference ahead of borrowing the math. It tells you what to measure, and now measures the cheap form of it. Alice is surprised exactly when her model of Bob has drifted from what Bob actually is. Minimizing surprise is minimizing that divergence across the team. Two things fall out for free:

**A principled emit rule.** A session should not broadcast its whole self-model. It should emit exactly the self-deltas that would otherwise leave others' L2 models of it stale — the corrections that keep everyone else's model-of-me accurate. The emit gate stops being a heuristic: emit what, if unsaid, makes someone's model of you wrong. (This rule is built — `EmitDelta` / `ettle drift`: a session re-emits only the atoms whose `(type, subject)` slot has changed against what each teammate already believes, and an unchanged note is not even re-distilled. The savings are visible in the demo: round two sends a couple of people's deltas, not the whole team's state again. Honest limit: the slot key is exact, and subjects are stochastic distiller text — so a *reworded* subject on a still-held belief reads as drop+new, which means the savings hold per-*person* but degrade per-*belief* once a note is re-distilled. Wording-independent slot identity is the next step; it is also the line where this structural rule hands off to the semantic layer.)

**Conflict-detection as tangle-detection.** A "tangle" (Laing's term) is a metaperspective that has drifted from reality while no one checks. L3's job is to detect where Alice-of-Bob and Bob's-self-model have diverged past a threshold and surface it — before it ships as a broken dependency or duplicated work.

The L2-vs-L1 divergence measured over time is a calibration metric — the same object a longitudinal calibration-metric store is built around, and its natural home.

## Why the reasoning-in-progress is the signal

Inferring intent from artifacts (commits, sent mail, calendar entries) catches it too late and invites confabulation — the agent reconstructs a tidy "why" it has no access to. People reasoning through a decision with an agent express the intent while it is still forming. ettle's signal is that live reasoning, distilled to typed atoms, not the downstream artifacts. This is the difference between modeling people and modeling tasks, and it is the difference between ettle and the shared-task-state products that already exist.

## Depth, and where to stop

Laing's spiral is infinite — my view of your view of my view, and so on — and the deep nesting is where pathology lives. Recursive-reasoning work in multi-agent RL shows the same diminishing returns. Humans stop at depth 2 or 3. So does ettle: model self and model-of-other (depth 2), with selective depth 3 for a few high-stakes pairs. The regress is not worth its cost.

## The critical insight: act, then check

A self-model on its own is telemetry, not action: a readout the agent could display, but nothing acts on it. The actionable layer — the agent doing something different *because of* its model, unprompted — is the unbuilt part, even for one user. The multiplayer system is N copies of a thing that has to first act usefully at N=1.

So the build order is N=1 first:

1. One actionable behavior a session performs unprompted because of its model of its single human.
2. A did-it-help signal, because an actionable layer without a correction loop is a machine for fast, confident, wrong action.
3. Only then multiplayer (L2 + L3 over the NATS atom bus, gemot for cruxes).
4. Calibration loop mandatory at the team tier.

## Friction in the right spots: bind vs surface

"Minimize surprise" is not "remove all friction." Frictionless is impossible and undesirable wherever stakeholders genuinely diverge (Rakova; the full framing is in TEAM_SIM.md, citations in PRIOR_ART.md §3). So the actionable layer routes every tangle it detects:

- **Bindable coordination** (positive-sum: sequencing, an interface contract, an ownership handoff) — the agents settle it among themselves and hand each human the outcome as an FYI. This is the information-sync job a meeting did; it collapses to near-zero. The toil is gone.
- **A values/priority crux** (zero-sum: whose external deadline wins, accepting a risk someone would refuse, who loses ownership) — the agents *must not* decide it. They pre-stage the branches and hand the human a clean either/or. This is the preference-aggregation / commitment / conflict job a meeting did; it is a speech-act, and it stays with the person.

This is the same split the premise drew above — info-sync dies, politics doesn't — now made operational: the emit/act layer asks, for each tangle, *is this ours to settle or theirs to decide?* The mechanism that finds the boundary is crux-detection (gemot's primitive). The discipline is conservatism: default to bindable; surface only a genuine zero-sum values choice, and surface it pre-staged so it costs the human seconds, not a meeting. The felt result is the whole point — empowered and free of bullshit meetings, but still receiving the benefit of having had a great meeting, because the mesh held it on everyone's behalf.

The controversy-tiered routing itself is ettle's; what's prior art is the deliberation organ it routes the contested branch *to* — agent negotiation (Learning-to-Negotiate), divergence-surfacing (Perplexity's Model Council, Polis), reputation-and-sanctions governance (Deliberative Curation). ettle *applies* that mature machinery to people-modeling rather than inventing it — consistent with riding mature planners fed dynamically (PRIOR_ART.md §4).

## Why "ahead at high speed" is the real leverage — and the real danger

Agent-time is much shorter than human-time. The collective model can roll the team's state forward and act before the humans arrive there — a forward model / model-predictive-control over the group. That speed differential is what lets the system *lead* rather than merely record. It is also what makes miscalibration dangerous: a fast, autonomous collective modeling its humans ahead, without a tight correction loop, does not converge — it confidently drifts. The calibration loop is therefore not polish. It is the thing that decides whether speed is an asset or a hazard.

Stated plainly: **this loop is not built yet.** The fast people-modeling half (the detector) runs today; the correction half does not — so until it exists and is shown to fail loud on divergence, every safety claim that leans on calibration is borrowing against unbuilt code. The `ettle eval` adjudication harness is the first running piece of it.

## Design invariants (non-negotiable)

These are the constraints that separate the good version from the bad one (SF_LINEAGE.md). Trading any of them away gives you the same frictionless surface with no one home.

- **Calibration before speed.** A model of a person must stay correctable by that person.
- **Contextual privacy boundary.** Each person controls what crosses their boundary; distilling typed atoms rather than streaming transcripts is the cheap form of this (confidential computing / TEEs are the substrate at larger scale). The boundary has two layers, unequal in strength — a deterministic secret-scanner (certain, for secret-*shaped* content) under a contextual-integrity prompt rule (emit the consequence of a change, not its private cause). The semantic layer is model *judgment, not verified redaction*; `ettle eval --leak` measures it at 0% on a synthetic corpus, which is evidence, not proof, and only as good as that corpus. The honest limit is *longitudinal*: per-atom checks can't see a fact reconstructed across many individually-clean atoms accumulating in a teammate's L2 model. That property is named, not yet defended (it lives with the unbuilt calibration loop) — see [SECURITY.md](../SECURITY.md).
- **Humans stay the point, not just the deciders.** The weaker form — humans remain the deciders, not just the modeled — falls out of the stronger one: the agent mesh is instrumentation for human coordination, never a substitute for it. The loop must return to a person (the torus framing above); a mesh that closes agent→agent→agent without that return has stopped being ettle.
- **Friction in the right spots.** Remove it from coordination / information-sync; keep it at the genuine values choices a person should own — pre-staged as a clean either/or, never auto-decided by the mesh.
- **The coordination commons is governed, not assumed.** Coordinated quality without wasted time is a common-pool resource with a real overgrazing failure mode (every agent over-emitting); Ostrom's principles govern it, with gemot reputation as the graduated sanction ([COMMONS.md](COMMONS.md)).
- **No machine-speed feedback loop.** The system runs at the speed of human decisions, not the bus: L3 emits tangles not atoms, emit is surprise-gated, reconcile is O(1) and shared ([SCALING.md](SCALING.md)).
- **Consent-first, anti-viral adoption.** The unit of adoption is a team that opts in together; never represent or contact a non-participant ([ADOPTION.md](ADOPTION.md)).
- **Useful at N=1.** Value must not be hostage to network effects, or adoption pressure becomes coercive.
- **Truncate the recursion.** Model self + model-of-other (depth 2), selective depth 3 for a few high-stakes pairs; no infinite metaperspective regress.
