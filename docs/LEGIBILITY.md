# ettle — turning the mirror around (legibility & contestability)

*A design draft, written against a structured adversarial pressure-test: ettle's [CONCEPT.md](CONCEPT.md) and code read through four critical lenses drawn from published work — situated action (after Suchman), contextual integrity (after Nissenbaum), extraction skepticism (after Bender), and commons governance (after Ostrom). The surnames name the **frameworks applied here as analytical lenses**, not reviews by those authors. Applied independently, all four surface the same fault line; this doc stages the fix from cheap-and-shippable to the structural lift. The far-future version of it already lives in [HORIZON.md](HORIZON.md) — "a ledger she can read, and to be correctable, always, by her, about her." This doc is the near-term bridge to that line: the part of it that is missing from the design today, not just from the code.*

---

## The fault line

Applied independently to [CONCEPT.md](CONCEPT.md) and the code, the four lenses converge on the **same** structural error and point to the **same** fix. That convergence is the signal — it means the problem is in the architecture, not the prose.

The error: **ettle treats a model's output as a fact about a person, and the person it is about cannot see or correct it.**

- L2 — *Alice's agent's belief about Bob* — "drives behavior" (gates what reaches Alice, decides whether Bob is FYI'd or surfaced in a knot) and is, per [ADOPTION.md](ADOPTION.md), "a one-way mirror at exactly the layer that drives behavior."
- The **knot** is asserted into a teammate's view as a typed, confidence-stamped claim about a named colleague — an intervention, not a report (situated-action lens). The negotiation that would have resolved it is pre-empted by the object that names it.
- **Surprise = L2-vs-L1 divergence** scores everyone's understanding against Bob's *self-account* as if it were ground truth. It isn't; it's another distillation of another text (extraction-skepticism lens). Calibrating L2 against L1 is *inter-model agreement*, not calibration against reality.
- An **inferred atom** manufactures a claim about a person that exists in no originating context, and the `--leak` eval — which scans for markers the person *wrote* — is structurally blind to it (contextual-integrity lens).
- The **sanction** for over-emitting lands on a resettable agent while the human principal keeps the benefit; the **monitor** that would detect over-grazing is the unbuilt calibration loop (commons-governance lens).

The fix each lens points to:

| Lens | The fix |
|---|---|
| Situated action (Suchman) | knot → an *invitation-to-repair* addressed to the pair, not a verdict to individuals |
| Contextual integrity (Nissenbaum) | inferred atoms *shown to the subject before they flow* to anyone else |
| Extraction skepticism (Bender) | every unconfirmed cross-person knot is a *question*; the human's answer is the calibration label |
| Commons governance (Ostrom) | a *mutual-legibility ledger* — every L2 readable and contestable by the person it's about |

One move underwrites all four:

> **Demote the model's output from a private assertion to a legible, contestable signal — and treat the human's correction as the ground-truth label the system currently invents.**

This is not "build a better detector." It is *turn the mirror around* so the thing the system believes about you is the thing you can most easily see and fix.

---

## The build, staged by cost and reversibility

Each stage names the critique it answers, the concrete change (in real component terms — `Atom`, `Knot`, `EmitDelta`/`ettle drift`, `GroundKnots`, the surface layer, the `--leak` eval), the cost, and the honest limit. Stages are ordered so the cheap, reversible, high-trust changes ship first and the structural ones inherit their machinery.

### Stage 0 — honesty in the existing surface (cheap, reversible, no schema change)

These three change *what the current system says*, not what it computes. They are shippable now and each closes a failure mode the pressure-test named without touching the invariants.

**0a. Legible abstention** *(legibility lens; extraction-skepticism lens).* When the coupling check or the abstention floor drops a candidate, the surface today drops it silently — and the pitch ("the mesh held it on everyone's behalf") then trains the human to stop watching. The two drop sites want *different* surfacing, and conflating them is a mistake the first draft of this doc made: a **coupling-check** kill is a *high-recurrence* knot the judge overruled — exactly the call a human might disagree with — so it is **listed**, off the main agenda, in a "held back — judged not a real conflict, shown in case that's wrong" section, filtered to `me`. A **floor** drop recurred in ≤1 of 5 samples — noise by design — so listing each one would just retrain the human to ignore the notice; it belongs as a single quiet **aggregate count**, not a list. *(Shipped, both halves — `GroundKnots` returns `(kept, suppressed)` and `ReconcileVoted` returns the floor-drop count; both surface on the CLI `surface` and the MCP `horizon`: coupling-check kills listed off-agenda, floor drops as one quiet aggregate line.)* Limit: a held-back knot is shown, not re-asserted — it restores *vigilance*, not the agenda slot.

**0b. Honest inferred-atom marking** *(contextual-integrity lens, lighter form).* `Atom.Inferred` already exists. When an inferred atom about Bob would surface to the team, mark it visibly as *inferred, not stated* in the output, and — the one real behavior change — route it to **Bob first** as "your agent inferred this about you; correct or kill it before it travels." This is the smallest version of "shown to the subject before it flows," using machinery that exists. Cost: a routing branch in the surface layer + a confirm sink. Limit: until the calibration loop (Stage 2) exists, "Bob corrects it" is a manual gate, not a learned one.

**0c. Interrogative register for cross-person knots** *(situated-action lens; extraction-skepticism lens).* The FIRM/soft recurrence split stays *internally* (a ranking signal), but the **output register** for a cross-person knot becomes a question addressed to the parties — "[possible collision] … Real, or already handled?" — not "[collision] … a direct conflict." The act/ask line, grounded in mixed-initiative design (Horvitz) and trust-calibration (Lee & See): *act only when confident and positive-sum, ask otherwise.* The honest reading of "confident" **today** is the key call — recurrence is test-retest *stability*, not validity, and no cross-person knot is calibration-confirmed yet — so the line is **self vs cross-person**: only self knots (own drift, which the person can verify directly) are asserted; every cross-person knot is posed as a question, ordered firm-first. The Firm-and-bindable act-lane for cross-person knots opens later, *earned per kind* against the calibration label (stage 2b) — so this stage is also the **active-learning query front-end** the calibration loop needs (Settles): the question is where the human's confirm/dismiss *label* will be captured. *(Shipped — the CLI `surface` routes self→assert / cross-person→question; the MCP `horizon` marks each cross-person knot `question:true` and gives it a wording-independent `key`. **0c-2 also shipped:** an `ettle_respond` MCP tool captures the human's verdict (`real` / `not_real` / `handled`) on a knot key as a `Label` appended to a local JSONL — the active-learning label stream the stage-2 calibration loop will consume. It records only; it binds and decides nothing. The CLI stays one-shot, with no response channel, by design.)* Limit: the question is ambient and dismissible (no forced confirm), so it does not re-add interruption toil; this is where the "toil → zero" headline gets honestly paid for.

### Stage 1 — provenance and the read-side mirror (schema + one new view)

**1a. The atom carries its transmission principle** *(contextual-integrity lens, core).* Today `sealAtom` reduces an atom to `{type, subject, content, confidence}` and the *context it was uttered in is discarded* — which is why a fact reconstructed across many clean atoms has no origin to violate against. Extend the sealed atom:

```
Atom{
  type, subject, content, confidence,
  derivation:  stated | inferred          // formalizes the existing Inferred bool
  origin:      self-to-own-agent (default, most restrictive)
  flow:        permitted onward contexts + transmission principle
               (default: none beyond origin until a standing rule or the subject opens it)
}
```

The default is the *restrictive* one: an atom does not acquire onward-flow permission from the model's per-call cause-vs-consequence guess (under the contextual-integrity lens, an unsituated third party deciding which intimate attributes become coordination). It acquires it from a **standing rule the person set** ("availability changes may flow to the team as consequence-only") or from the subject opening it (Stage 0b). The cause-vs-consequence prompt becomes a *suggestion* the standing rule adjudicates, not the adjudicator.

This reframes `--leak` from "did a banned substring cross?" to the only contextually-meaningful question: **"did any atom reach a context not in its permitted set?"** — which finally makes the *inference* leak and the *longitudinal* leak detectable, because each atom now drags its norm with it instead of being laundered into context-free content at the seal. Cost: a schema migration + the leak eval rewrite; medium. Limit: the *transmission principle* is still authored by humans-via-standing-rules and defaults; this makes the norm *explicit and enforced at the crossing*, it does not make the norm *correct*. That is the right division of labor — the system enforces the flow, the person owns the rule.

**1b. The read-side mirror — `ettle mirror --me bob`** *(situated-action, commons-governance, malleable-software lenses).* `ettle drift` already builds each per-pair L2 model deterministically as a set of belief-atoms. The missing half is a *read*: a view that shows Bob the union of what the team's models currently assert *about him* — the surprise deltas first (those are exactly the beliefs most likely to be wrong). This is the smallest real turn of the mirror: Bob can *see* the rule that governs how he's treated, even before he can change it. It is buildable on `drift` today with no new model call.

The privacy tension here is real and is engaged below (showing Bob what is believed about him is *itself* a flow that touches the believers). The Stage-1 answer: the mirror shows Bob the *beliefs held about him* and that they are held, with attribution coarsened by default (the belief, not necessarily "Alice specifically thinks this") — read-only, no correction propagation yet. Attribution granularity becomes a per-team knob in Stage 2.

### Stage 2 — contestability and the human-labeled loop (the structural lift)

This is the part [CONCEPT.md](CONCEPT.md) already marks "deliberately unbuilt … the part that needs the most care." The panel's contribution is to say *what it must be built out of*: not L2-vs-L1 agreement, but **human corrections as the ground-truth label.**

**2a. Contestability — the correction is a first-class event.** From the Stage-1 mirror, Bob can mark an L2 atom "wrong — I never assumed that." That correction is the load-bearing object the whole program was missing, and it does four jobs at once (this is why all four lenses converge on it):

- it is the **monitoring event** Ostrom's principle 4 requires to be *member-generated* and member-accountable — a metric store cannot supply it;
- it is the **calibration label** the extraction-skepticism lens shows the loop structurally lacks — a signal not produced by a model, from a person who actually observed the referent;
- it makes the L2 layer the **collective-choice register** (Ostrom principle 3) — the person the rule acts on can now change it;
- it is the **repair occasion** the situated-action lens wants the knot to *host* rather than replace.

**2b. The calibration loop, defined against the human label.** Calibration becomes: *the detector's flag-rate vs. the human-confirmed-real-rate, per knot kind, over time* — not L2-vs-L1 convergence. It runs at **human speed** by construction (it waits on confirmations), which is what the "no machine-speed feedback loop" invariant already demands; the pressure-test's apparent paradox ("calibration done honestly is itself a meeting") dissolves once the label is the cheap async confirm of Stage 0c, not a synchronous standup. The system earns the right to flag more aggressively — and eventually to *bind* without asking — only as its confirmed-real-rate rises against that external label. This is the gate that turns "calibration before speed" from inter-model agreement into a real safety property.

**2c. Governance stake on the principal** *(Ostrom 1, 4, 5).* Move the sanction from the resettable agent to the **human principal's standing**, computed over the legibility ledger (over-emits, overturned binds, corrected-wrong L2 beliefs). Now the party who holds the incentive to over-emit is the party whose bind-rights degrade — restoring the deterrence the agent-firewall currently defeats — and "the coordination commons is governed" stops being a crosswalk. Design-only here; it inherits 2a's ledger as its substrate.

---

## Tensions — where this draft does not simply obey the pressure-test

A robust draft argues with its own lenses. Four places the prescriptions cut against something real, and how this design holds the tension rather than capitulating.

- **"Nothing auto-binds until both confirm" vs. "toil → zero."** The situated-action invert-the-default (and the extraction lens's "no FIRM tier") would make *every* coordination, even genuinely positive-sum sequencing, cost a human confirm — which reintroduces exactly the low-grade toil the premise kills. This design *narrows* the demotion: the interrogative register (0c) and confirm-before-bind apply to **unconfirmed cross-person knots**, and the bind-without-asking right is *earned per kind* against the calibration label (2b). A team whose interface-contract knots run at 0.98 confirmed-real over months should get silent binding *for that kind*; a fabricated-prone kind never does. The headline becomes honest — "toil → near-zero *for the kinds the system has earned it on*" — instead of either false ("→ zero") or defeatist ("everything is a question forever").

- **Legibility has its own contextual-integrity cost.** The mirror (1b) shows Bob what is believed about him — but those beliefs are held *by* Alice's agent, and surfacing "Alice thinks you assume P" is a flow that touches Alice. The panel's legibility fix and the pressure-test's privacy fix are in tension. This is why attribution is *coarsened by default* (1b) and granularity is a collective-choice knob (2a/2c), not a global constant: the team governs how much the mirror attributes, the same way it governs emit thresholds. Turning the mirror around cannot mean building a new surveillance surface pointed at the modelers.

- **Full contextual-integrity flow-engine vs. the 80/20.** The contextual-integrity lens's complete prescription is a flow engine that refuses any transmission failing a typed transmission principle. That is heavy and could ossify a system whose whole value is fluid coordination. Stage 1a takes the 80/20 — *provenance + restrictive default + subject-gated inference* — which closes the inference and longitudinal holes (the ones the leak eval is blind to) without a full policy VM. The full engine is a horizon item, not a prerequisite.

- **The detector is not worthless because it can't certify truth.** The extraction-skepticism lens is right that the model has no ground truth for "is this a real conflict." But a *salience filter* — "a human should look at this pair" — is a real and useful thing the model *can* do, and the demotion to interrogative (0c) is precisely the move that makes the detector honest about being a filter, not a referee. The fix is not to delete the detector; it is to stop it asserting.

---

## What each fix buys, against the invariants

The three invariants the pressure-test flagged as "borrowing prestige," and what moves them from aspirational to earned:

| Invariant ([CONCEPT.md](CONCEPT.md)) | Today | Moved by | Becomes |
|---|---|---|---|
| Contextual privacy boundary | access-control (regex + suppress-list) | 1a provenance + 0b subject-gated inference | flow governed by an explicit, enforced, person-owned norm |
| Calibration before speed | L2-vs-L1 inter-model agreement | 2a correction-as-label + 2b human-labeled loop | flag-rate calibrated against an external human label |
| The commons is governed | crosswalk; sanction on the agent | 2a member-accountable monitor + 2c stake on the principal | monitoring and sanction reach the party who holds the incentive |
| Humans stay the point | true for *deciding*, false for *being modeled* | 1b mirror + 2a contestability | the modeled person can see and correct the model |

The fixes the pressure-test left *standing* — the bindable-vs-crux split, reasoning-in-progress as signal, useful-at-N=1 — are untouched here; this draft adds the legibility spine *under* them, it does not replace them.

---

## Non-goals (what stays unbuilt, on purpose)

- **The full CI policy engine** (a flow VM) — Stage 1a's provenance is the 80/20; the engine is a horizon item.
- **Machine-speed binding** — even at high earned confidence, binding stays gated by the calibration label, which is human-paced by construction. The speed differential ([CONCEPT.md](CONCEPT.md) §"ahead at high speed") buys *pre-staging*, never *autonomous commitment*, until 2b has the evidence.
- **Attribution-rich mirrors by default** — the mirror coarsens attribution until a team opts into more, so legibility does not become a surveillance surface pointed at the modelers.

---

## Build order

`0a → 0b → 0c` ship now (days each, reversible, no schema change, close named failure modes immediately). `1a → 1b` are the next increment (schema migration + one read view; they make the leak eval honest and turn the mirror's *read* side). `2a → 2b → 2c` are the structural lift and the real research — but they now inherit a concrete substrate (the correction event) instead of starting from "specced, not built." The through-line: every stage makes the system *say less than it knows and show more of what it believes* — which is the only version of ettle that earns the invariants it already claims.
