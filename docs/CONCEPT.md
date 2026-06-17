# ettle — the concept

## The three-layer model

The structure comes from the interpersonal-perception literature: in a dyad there is each person's relation to themselves, each person's relation to the other from their own vantage, and the relation of the pairing itself. Mapped onto a team of agents, ettle is three layers, not one flat shared pool.

**L1 — self-models.** Each session's model of its own user: what they are working on, assuming, leaning toward; what they have corrected or closed before. This is the single-user layer (supplied by a private predecessor project), one model per person — N of them. Private. (The internal working model of self, in attachment terms.)

**L2 — directed dyadic models.** Alice's session's model *of Bob*. Asymmetric, and there are N×(N−1) of them: Alice-of-Bob is not Bob-of-Alice. This is the metaperspective — what I believe you are assuming, leaning toward, about to do. It is Alice's belief about Bob, not Bob's actual state. A single-user model has no analogue here, because it has only one author.

**L3 — the collective layer.** The merged horizon. The team-as-entity, held by no individual, where reconciliation and conflict-detection live.

## Surprise is metaperception error

The payoff of the layering is that "minimize surprise" becomes precisely definable:

> Surprise = the divergence between each person's L2 model-of-another and that other person's L1 model-of-self.

Alice is surprised exactly when her model of Bob has drifted from what Bob actually is. Minimizing surprise is minimizing that divergence across the team. Two things fall out for free:

**A principled emit rule.** A session should not broadcast its whole self-model. It should emit exactly the self-deltas that would otherwise leave others' L2 models of it stale — the corrections that keep everyone else's model-of-me accurate. The emit gate stops being a heuristic: emit what, if unsaid, makes someone's model of you wrong.

**Conflict-detection as knot-detection.** A "knot" (Laing's term) is a metaperspective that has drifted from reality while no one checks. L3's job is to detect where Alice-of-Bob and Bob's-self-model have diverged past a threshold and surface it — before it ships as a broken dependency or duplicated work.

The L2-vs-L1 divergence measured over time is a calibration metric — the same object a longitudinal calibration-metric store is built around, and its natural home.

## Why the reasoning-in-progress is the signal

Inferring intent from artifacts (commits, sent mail, calendar entries) catches it too late and invites confabulation — the agent reconstructs a tidy "why" it has no access to. People reasoning through a decision with an agent express the intent while it is still forming. ettle's signal is that live reasoning, distilled to typed atoms, not the downstream artifacts. This is the difference between modeling people and modeling tasks, and it is the difference between ettle and the shared-task-state products that already exist.

## Depth, and where to stop

Laing's spiral is infinite — my view of your view of my view, and so on — and the deep nesting is where pathology lives. Recursive-reasoning work in multi-agent RL shows the same diminishing returns. Humans stop at depth 2 or 3. So does ettle: model self and model-of-other (depth 2), with selective depth 3 for a few high-stakes pairs. The regress is not worth its cost.

## The critical insight: act, then check

The single-user layer today is telemetry, not action. The injected shared-model block is a readout the agent reads; nothing acts on it. The actionable layer — the agent doing something different *because of* its model, unprompted — is unbuilt even for one user. The swarm is N copies of a thing that does not yet act at N=1.

So the build order is N=1 first:

1. One actionable behavior a session performs unprompted because of its model of its single human.
2. A did-it-help signal, because an actionable layer without a correction loop is a machine for fast, confident, wrong action.
3. Only then multiplayer (L2 + L3 over the NATS atom bus, gemot for cruxes).
4. Calibration loop mandatory at the swarm tier.

## Friction in the right spots: bind vs surface

"Minimize surprise" is not "remove all friction." Frictionless is impossible and undesirable wherever stakeholders genuinely diverge (Rakova; the full framing is in TEAM_SIM.md, citations in PRIOR_ART.md §3). So the actionable layer routes every knot it detects:

- **Bindable coordination** (positive-sum: sequencing, an interface contract, an ownership handoff) — the agents settle it among themselves and hand each human the outcome as an FYI. This is the information-sync job a meeting did; it collapses to near-zero. The toil is gone.
- **A values/priority crux** (zero-sum: whose external deadline wins, accepting a risk someone would refuse, who loses ownership) — the agents *must not* decide it. They pre-stage the branches and hand the human a clean either/or. This is the preference-aggregation / commitment / conflict job a meeting did; it is a speech-act, and it stays with the person.

This is the same split the premise drew (HANDOFF.md) — info-sync dies, politics doesn't — now made operational: the emit/act layer asks, for each knot, *is this ours to settle or theirs to decide?* The mechanism that finds the boundary is crux-detection (gemot's primitive). The discipline is conservatism: default to bindable; surface only a genuine zero-sum values choice, and surface it pre-staged so it costs the human seconds, not a meeting. The felt result is the whole point — empowered and free of bullshit meetings, but still receiving the benefit of having had a great meeting, because the mesh held it on everyone's behalf.

There is also prior art for the routing itself, not just the pieces: "process the uncontroversial efficiently, deliberate the contested" is a published multi-agent protocol shape (Deliberative Curation; Perplexity's Model Council; Polis). ettle *applies* it to people-modeling rather than inventing it — consistent with riding mature planners fed dynamically (PRIOR_ART.md §4).

## Why "ahead at high speed" is the real leverage — and the real danger

Agent-time is much shorter than human-time. The collective model can roll the team's state forward and act before the humans arrive there — a forward model / model-predictive-control over the group. That speed differential is what lets the system *lead* rather than merely record. It is also what makes miscalibration dangerous: a fast, autonomous collective modeling its humans ahead, without a tight correction loop, does not converge — it confidently drifts. The calibration loop is therefore not polish. It is the thing that decides whether speed is an asset or a hazard.
