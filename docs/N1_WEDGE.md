# ettle — the N=1 wedge

The critical path ([CONCEPT.md](CONCEPT.md)) is the one thing a single-user agent layer never did even for a single user: **act on the model, and check whether the action was right.** This document specs that — one behavior, its trigger, its outcome signal, and its offline criterion — concretely enough to build. Everything multiplayer (L2 directed models, L3 tangle-detection) reuses this object; the spec calls out the reuse seam at each step.

> **Scope note.** This N=1 wedge is a *spec*, not built here — it builds on a private predecessor agent's hooks (its closed-decision cells) that are **not in this repo**. The runnable thing in this repo is the multiplayer PoC ([`cmd/ettle`](../cmd/ettle)), which is **standalone** and does not depend on that layer. This document is the design lineage for the calibration loop the multiplayer tier reuses.

## The behavior: the prior-decision guard

> When the live turn is about to **re-open something the user previously closed**, the session surfaces that once — as a question, before proceeding — instead of silently complying.

Such a layer already stores `user-closed` cells (an injected state block with entries like `user closed: rabbit-hole, drop-it, artifacts-tooling`). Today they are telemetry: injected, read, acted on by nobody. The wedge makes the session *do something* with one: name the reopen and let the human decide, rather than re-walking a path the human already decided against.

This is the lowest-stakes form of "act on a model of the person." It emits words, not file edits — but a wrongly-fired guard still spends the user's attention, and attention is the scarce resource, so it genuinely exercises act-then-check. The correction loop matters even when the only action is a question. That is the point of starting here.

### Why this one, over the other two candidates

Three candidates were considered. Scored against the invariants (useful at N=1; models the *person* not the task; clean did-it-help; safe/reversible; generalizes to L2):

| Candidate | Useful at N=1 | Models person | Clean signal | Generalizes |
|---|---|---|---|---|
| **Prior-decision guard** | Yes — saves re-deciding | Yes — their past decisions | **Yes — next turn confirms/rejects** | Yes — L2 tangle is the same record |
| Predict-next-file pre-read | Marginal (saves seconds) | No — models the task | Weak (invisible, low-stakes) | No |
| Ownership prompt ("touched X 3×, owning it?") | Forced — ownership is meaningless solo | Partly | Medium | Yes, but only meaningful *with* teammates |

The pre-read models tasks, which CONCEPT.md explicitly distinguishes ettle *against*. The ownership prompt's value is hostage to having teammates, violating "useful at N=1." The prior-decision guard is the only one whose did-it-help signal is the *same object* the multiplayer calibration loop needs (L2-of-someone checked against their L1), and whose value is real for one person alone.

## The mechanism — a hybrid loop over the predecessor's hooks

Deterministic detection, LLM judgment at the one consequential fork, typed outcome, calibration. Each stage is an existing predecessor hook plus a thin addition.

1. **Detect (deterministic, cheap) — `inject` at UserPromptSubmit.**
   the predecessor already topic-matches the incoming prompt against state. Addition: flag when the prompt's subject intersects a **high-confidence closed cell**. When it does, inject not just the cell but a *directive*: "The user closed `<subject>` on `<date>` with evidence `<quote>`. If this turn re-opens it, surface that once before proceeding; otherwise stay silent."

2. **Decide to fire (fuzzy, LLM, in-context).**
   The session judges whether the turn is *actually* an unwanted reopen worth one interruption, or a deliberate, context-aware re-entry it should let pass. This is the only judgment that belongs in the model rather than the hook — the gate is mechanical, the discrimination is semantic. **Default is silence.** Fire only above a confidence threshold (precision over recall — see the false-interrupt guard).

3. **Fire (the action).**
   One sentence: *"You closed this before (`<quote>`, `<date>`) — still want it, or did it slip back in?"* Then stop and let the human decide. No file mutation. Human remains the decider (invariant).

4. **Observe (deterministic capture) — `observe-prompt` on the next turn.**
   The user's reply to the guard is captured as the outcome of a logged `GuardEvent`.

5. **Score (fuzzy, LLM) — classify the reply into a typed outcome,** and **calibrate** — write the result back to the closed cell. The predecessor's `observe-prompt` classifier already routes user replies into typed sources; the wedge adds three outcome labels (below).

## Atom schemas

Two atoms. The first is a *view* over the predecessor's existing cells; the second is new and is the did-it-help substrate.

**`ClosedDecision`** — a cell where `Source ∈ {user-closed, user-corrected-claude}`, `Status = closed`, `Confidence ≥ θ_high`.

```
subject     string   // topic / path / approach the user closed
closed_at   time
evidence    string   // the user's own words, verbatim (the predecessor already stores this)
confidence  float    // predecessor cell confidence
```

**`GuardEvent`** — one per fire (and per deliberate suppression, for coverage accounting).

```
id            string
fired_at      time
trigger       ref<ClosedDecision>
turn_excerpt  string                 // what in the live turn matched
decision      enum{ fired, suppressed }
outcome       enum{ pending, helped, overridden, false_interrupt }
resolved_in   int                    // turns from fire to outcome
```

This resolves open question #1 (decision-delta atom schema), in its N=1 form. The multiplayer atom is the same `GuardEvent` with `trigger` pointing at *another participant's* closed cell — that is the L2 tangle record, surfaced by L3. Build it once here.

## The did-it-help signal

When the guard fires, the user's next reply is classified into exactly one of three outcomes. The discipline is that two of these are *not* failures — only one is.

- **`helped`** — the reopen was unintended; the user confirms / drops it / thanks. → reinforce the closed cell. **Counts as a true positive.**
- **`overridden`** — the user is re-opening it *on purpose* ("changed my mind", "different now"). The model was right that it *had been* closed; the user's intent legitimately moved. → **retire or down-weight the cell.** This is the calibration loop closing, not a miss. Excluded from precision; it is the healthy update path.
- **`false_interrupt`** — it was never a real reopen, or the question was noise. → **penalized hard.** This is the confabulation-amplifier failure the whole category risks.

**Primary metric — guard precision:**

```
precision = helped / (helped + false_interrupt)
```

`overridden` is deliberately excluded — punishing the system for a user's genuine change of mind would teach it to suppress correct flags. Coverage (did it fire on reopens it should have caught) is tracked separately from `suppressed` events and missed reopens surfaced in retro; precision is primary because a noisy guard is worse than a quiet one.

**False-interrupt guard (absolute, hard):** borrowed directly from the predecessor's false-positive guard. If the per-window `false_interrupt` rate exceeds a small absolute bound, the wedge is downgraded regardless of precision and the firing threshold `θ` is raised before anything else proceeds. A guard that interrupts wrongly is the failure mode that makes users turn the category off.

This is the **N=1 instance of the L2-vs-L1 divergence metric** from CONCEPT.md. At N=1 the "other's L1" is simply the user's actual next reply; the agent's belief "you still consider this closed" is its L2-of-its-own-user, checked against reality every fire. Accumulated `helped`/`false_interrupt`/`overridden` rates over time *are* the longitudinal calibration-metric store object, prefigured for one user. The team tier **reuses this object's schema**, but it is not "the same thing with a second author": at N=1 the author of the model and the ground truth are the same person, replying in-session immediately; at the team tier the model is of a *different* person, checked against their separately-distilled, privacy-bounded atoms, asynchronously. Second-order theory-of-mind, the Alice-of-Bob ≠ Bob-of-Alice asymmetry, and getting a teammate's ground truth through a boundary that deliberately discards most of their reasoning are the genuinely hard, **unsolved** parts that appear only at L2 — the N=1 result de-risks the schema and the act-then-check loop, not those. This prefigures open question #4 (L3 conflict-detection = the same divergence, cross-author) without claiming to have solved it.

## Scrappy simulation — running now, agents standing in for the human

We don't need a real human team (or even a real day of dogfooding) to watch the loop behave. It runs locally today, on cheap LLMs, with **agents standing in for the human**: a local sim.

Per seeded scenario, the loop is four cheap Haiku calls:

1. A **stand-in human** persona-agent is given a *hidden intent* — it either forgot it closed a topic (`slip`), is reopening on purpose (`deliberate`), or is doing adjacent work that only looks similar (`continuation`) — and writes its next message.
2. The **wedge** sees the closed-decision (real provenance: `dim=fatigue status=fatigued source=user-closed`, lifted from the predecessor's state layer) plus the message, and decides **FIRE** or **PASS**, biased toward silence.
3. If it fires, the stand-in human **reacts** in character.
4. A **judge** labels the reaction `helped` / `overridden` / `false_interrupt`. A PASS on a real reopen is a `missed`; a PASS on adjacent work is `correct_silence`.

It reuses the predecessor's pieces rather than reinventing: the Haiku call pattern (cached system prompt) is lifted verbatim from the predecessor's cached-prompt dialogue helper; the closed-decision is the production closed-decision view; the seed/judge shape mirrors its paired-scenario judge harness. The next rung up is to swap the inline seed list for richer social-simulation personas (richer personas with objectives), which already exist in the module.

First run (a handful of hand-seeded scenarios, all on `claude-haiku-4-5`): 2 `helped`, 2 `overridden`, 2 `correct_silence`, **0 false interrupts, 0 missed**. The point isn't the score on a few toy rows — it's that the act-then-check loop is closed and observable end-to-end, so we can now grow the scenario set, perturb the prompts, and watch where the guard gets noisy. The one number to drive down is `false_interrupt`: a guard that interrupts wrongly is the one users switch off.

Run it: run the local sim with `-v` (needs `ANTHROPIC_API_KEY` in an env file).

When this graduates to a pre-committed offline criterion, it slots into the predecessor's existing paired-scenario judge harness (paired reopen-vs-continuation rows, LLM-judge, the false-interrupt guard as the hard bound) — the apparatus is already there. We don't need that formality to start learning from the sim.

## How the invariants are honored

- **Calibration before speed.** The outcome signal *is* the calibration loop; the wedge does not ship its action without it. `overridden` keeps the model correctable by the person — a deliberate reopen retires the cell.
- **Humans remain the deciders.** The action is a question; the human decides. No autonomous mutation.
- **Contextual privacy boundary.** N=1: nothing crosses a boundary. The atoms are typed (`ClosedDecision`, `GuardEvent`), so when this goes multiplayer the cheap distill-to-atoms form of the boundary is already the data shape.
- **Useful at N=1.** Saving one re-decided rabbit-hole is real value for one person, with no teammate required.
- **Truncate the recursion.** Depth 2: the session models its user's prior decision and acts on it. No metaperspective regress.

## Out of scope for the wedge (deliberately)

- Multiplayer (L2/L3, over the NATS atom bus with [gemot](https://github.com/justinstimatze/gemot) for cruxes) — only after the N=1 precision and the offline tier are met.
- File-mutating actions — the wedge is question-only by design; a guard that *edits* is a later, higher-stakes step that this one de-risks.
- The other two candidate behaviors — deferred, not rejected; revisit once the act-then-check loop is proven on this one.
- Emit-gate tuning for *broadcast* (CONCEPT.md's "emit what, if unsaid, makes someone's model of you wrong") — that is the L2 emit rule; the N=1 firing threshold is its single-author ancestor and is enough here.

## Build order

1. ~~Stand up a scrappy, locally-runnable sim of the loop with agents as humans.~~ **Done** — a local sim, all on cheap Haiku.
2. Grow the scenario set (more topics, harder near-misses) and swap the inline seed for richer social-simulation personas; watch where `false_interrupt` creeps up.
3. Add the `GuardEvent` store + the three outcome labels to the predecessor's `observe-prompt` classifier (move from sim to the live hook path).
4. Extend `inject` to emit the firing directive when a closed-decision cell is re-opened in a real session.
5. Dogfood live; watch the false-interrupt rate over real sessions. Acceptable → proceed to L2 (the same `GuardEvent`, trigger repointed at a teammate's closed cell, over the NATS atom bus with gemot for cruxes).
