# ettle — the CALO lineage (personal-assistant agents: what ettle inherits, and what it extends)

ettle's L1 layer — *an agent that models its own user's work* — is not new. It
sits at the end of a deep, heavily-funded lineage that peaked with DARPA's PAL
program in the 2000s. Honest prior art means naming that lineage rather than
implying an empty field. The short version: **the single-user personal assistant
was attempted hard, twice, with hundreds of millions of dollars; the less-trodden
path is a *pool of proxy agents modeling each other and their teammates* — and the
one serious prior attempt at proxy-pools is also the clearest cautionary tale for
ettle's invariants.** This page is the lit review behind that claim.

## 1. The academic roots (the mid-1990s interface-agent vision)

- **Maes, "Agents that Reduce Work and Information Overload"** (CACM 37(7):30–40,
  1994). The foundational statement: an autonomous *interface agent* that
  collaborates with you like a personal assistant — meeting scheduling, email,
  news filtering — and **learns by watching you**, via "indirect management" where
  human and agent both initiate, monitor, and act.
- **Mitchell, Caruana, Freitag, McDermott, Zabowski, "Experience with a Learning
  Personal Assistant"** (CACM 37(7):80–91, 1994 — same issue). **CAP**, the
  Calendar APprentice: a learning apprentice that watches you manage a meeting
  calendar and learns your scheduling preferences. The first solid evidence that a
  PA can *learn a specific person's* habits from observation.
- **Horvitz, "Principles of Mixed-Initiative User Interfaces"** (CHI'99,
  pp. 159–166). The Lookout scheduling system, and the principle set for *when an
  agent should act vs. defer to the human* — expected-value-of-action, the cost of
  guessing wrong, graceful handoff.

ettle's L1 (`internal/capture` distilling a live session) and its
"humans-stay-the-deciders / friction-at-the-cruxes" invariants are this lineage's
direct descendants. The one shift: these systems learned from *artifacts* (your
calendar, your email); ettle's thesis is to read the *reasoning-in-progress*.

## 2. The DARPA PAL program (2003–2008): the single-user peak

DARPA's **Personalized Assistant that Learns (PAL)** program funded two large
cognitive-assistant efforts in parallel:

- **CALO** (Cognitive Assistant that Learns and Organizes; SRI-led, ~$150M, 300+
  researchers across 22 institutions). A personal assistant for a busy knowledge
  worker: email, personalized time management, task reasoning and execution, and
  the **CALO Meeting Assistant** — multiparty meeting capture with dialogue-act
  tagging, **action-item recognition, decision extraction**, and summarization.
  CALO **spun out Siri** (2007–08) and, as a byproduct, prepared and released the
  **Enron email corpus** (see [BENCHMARKS.md](BENCHMARKS.md)).
- **RADAR** (Reflective Agents with Distributed Adaptive Reasoning; CMU, ~$7M
  first-year). A multi-agent, mixed-initiative assistant aimed squarely at
  **email overload**: it observes experts performing tasks and learns to classify
  mail, draft replies, and manage tasks, asking for confirmation on the
  consequential ones.

Both are direct ancestors of the parts of ettle that already work — CALO-MA's
decision/action-item extraction is a close cousin of tangle detection, and RADAR's
"learn from observed work, confirm the consequential" is exactly the
mixed-initiative posture ettle takes. **But both modeled one user.** Neither built
a *directed model of other specific people* held by each person's agent, nor a
privacy-bounded collective that reconciles those models. They are the L1 ceiling,
not the L2/L3 thing ettle is reaching for.

## 3. The proxy-pool ancestor — and the cautionary tale

The closest prior work to ettle's *pool of proxy agents* is **Electric Elves**
(USC/ISI; Tambe, Pynadath, Russ, Oh; deployed 24/7 at a research institute from
June 2000). It ran ~15 agents, including a **"Friday" proxy agent for each of nine
people**, coordinating a real group: rescheduling meetings (a Friday tells the
other Fridays, who tell their humans), selecting research presenters, tracking
locations, organizing lunches. This is the multi-principal, one-agent-per-person
shape ettle proposes — built and *deployed*, 25 years ago.

Two things make it the most instructive entry in this review:

1. **It coordinated logistics, not models-of-each-other.** Friday agents
   negotiated *when and where* (scheduling), not *what each person believes,
   assumes, or is about to change*. There was no directed metaperception (a
   private model of a teammate's reasoning), no contextual-privacy boundary on
   what crossed between proxies, and no surprise-minimization objective. That gap
   is precisely ettle's L2/L3 claim.
2. **"Electric Elves: What Went Wrong and Why"** (Tambe et al., *AI Magazine*
   29(2), 2008) is a published post-mortem: the proxies, given too much autonomy,
   made socially costly decisions (volunteering users to present, cancelling
   meetings wrongly), which drove the team's later work on **adjustable
   autonomy**. This is the empirical precedent for ettle's hardest invariants —
   *calibration before speed*, *humans remain the deciders*, *friction at the
   cruxes*. A pool of proxy agents acting on models of people, without a tight
   correction loop, has already been tried and has already failed in exactly the
   way [CONCEPT.md](CONCEPT.md) and [HORIZON.md](HORIZON.md) warn about. That is
   corroboration, not coincidence.

## 4. What ettle inherits vs. what it extends

**Inherits (and should stop pretending is novel):**
- the learning personal assistant that models its own user (Maes, CAP, CALO,
  RADAR) — ettle's L1;
- mixed-initiative / adjustable autonomy, humans deciding the consequential calls
  (Horvitz; Electric Elves' hard-won lesson) — ettle's "friction at the cruxes";
- meeting decision / action-item extraction (CALO-MA) — a cousin of tangle
  detection.

**Extends, or at least attempts past the lineage:**
- **directed L2 models of *teammates*** held per-person — CALO/RADAR were
  single-user; Electric Elves coordinated logistics, not models of each other;
- a **privacy-bounded, typed-atom boundary** between principals — none of the
  ancestors had a contextual-integrity boundary on inter-agent sharing;
- a **per-human calibration loop** as the thing that makes speed safe — the
  Electric Elves failure is the argument for why this is load-bearing, not polish;
- **reasoning-in-progress as the signal** rather than calendar/email artifacts;
- **resolve-and-apply, not detect-and-announce.** The lineage's assistants stop
  at extracting a decision or action-item and handing it to a human (CALO-MA, and
  every modern meeting tool); a plain tangle detector likewise *stops and announces
  the conflict*. ettle's bindable subset is instead **deliberated to an actionable
  conclusion by the agents themselves** — they enter a gemot, reach a concrete
  decision, and fold it back into each human's workflow **ambiently**, surfacing
  only the genuine cruxes. The honest qualifier: autonomous agent action with
  write-back is *not* itself novel in 2026, and Electric Elves (EE) already did it
  (Fridays rescheduled meetings and volunteered presenters on their own) — which
  is exactly why its post-mortem exists. The load-bearing novelty is therefore not
  the autonomy but the **partition** — what is safe to auto-bind vs. what must stay
  the human's call — applied to tangles derived from *privacy-bounded models of
  teammates* and gated by the calibration loop. That partition is the piece EE
  lacked and the reason it failed.

**Honest verdict.** ettle is *not* novel at L1 — that idea is thirty years old and
was funded at nine figures. The novelty, if any, is the specific multi-principal
assembly (directed metaperception + privacy boundary + calibration), and the most
relevant prior attempt at a proxy-agent pool (Electric Elves) failed in precisely
the way ettle's invariants are written to prevent. The honest framing for the
project is therefore not "no one has tried this" but "the single-user half is
mature prior art; the multi-principal half was tried once, instructively, and the
lesson is baked into the invariants." See [PRIOR_ART.md](PRIOR_ART.md) for the
contemporary (2025–26) landscape, and §5 below for where CALO's mission actually
went and what is liftable from its modern descendants.

## 5. The 2026 arc — where CALO's mission went, and what ettle can lift

CALO's charter was a cognitive assistant that could "reason, learn from
experience, be told what to do, explain what it is doing, reflect on its
experience, and **respond robustly to surprise**." Eighteen years on, that
mission has split into parts that are *done*, parts that are *commoditized*, and
two parts that are still open — and the two open parts are exactly what ettle is
reaching for.

**Done / shipped.** The single-user learning assistant (Maes → CAP → CALO) is no
longer research. CALO spun out Siri directly; the broad LLM-assistant wave (and
per-user "memory" features) is the mature descendant. "Learns your preferences
from observation" is table stakes, not a contribution.

**Commoditized.** CALO-MA's meeting capture — dialogue-act tagging, action-item
recognition, **decision extraction**, summarization — now ships in every
conferencing stack (Otter, Granola, Zoom/Teams/Meet companions, Copilot recap).
ettle's tangle detector is a cousin of this, so it inherits the maturity but
**cannot claim novelty** there.

**Still open (and ettle's actual target):**

1. **"Respond robustly to surprise" → proactive-trigger / necessity detection.**
   CALO's literal goal of acting well under surprise is the unfinished one, and
   it is a live 2025–26 research line: *when should an agent act unprompted at
   all?* **ContextAgent** (arXiv [2505.14668](https://arxiv.org/abs/2505.14668))
   frames this as **necessity prediction** — predicting from context whether
   proactive assistance is warranted before acting — and ships
   **ContextAgentBench** (1,000 samples, 9 daily scenarios, 20 tools) to measure
   it. That decision *is* ettle's surface-vs-stay-quiet gate (is this tangle worth
   a human's attention?). **Liftable:** the necessity-prediction framing and a
   ContextAgentBench-style eval are a concrete model for ettle's missing
   calibration loop — turning "did surfacing this tangle help?" into a measured
   prediction task rather than a hand-tuned threshold.

2. **The privacy boundary between principals → operationalized Contextual
   Integrity.** ettle already names Nissenbaum's contextual integrity as its
   boundary principle, but cites none of the hot 2024–26 work that
   *operationalizes* CI for LLM agents — and that work is both prior art ettle is
   missing and a mechanism ettle can lift:
   - *Can LLMs Keep a Secret?* / ConfAIde (arXiv
     [2310.17884](https://arxiv.org/pdf/2310.17884)) — tests CI-grounded
     information-sharing in LLMs via the CI tenet that disclosure must fit the
     context.
   - *Operationalizing Contextual Integrity in Privacy-Conscious Assistants*
     (arXiv [2408.02373](https://arxiv.org/html/2408.02373v1)).
   - *Privacy in Action* / **PrivacyChecker** (Microsoft Research, arXiv
     [2509.17488](https://arxiv.org/pdf/2509.17488)) — an Information-Flow-
     Extraction step (sender, recipient, subject, information type, transmission
     principle) plus a per-flow privacy judgment that **reports cutting agent
     privacy leakage from ~36% to ~7%**.

   PrivacyChecker's flow tuple is almost exactly ettle's typed atom
   (author / subject / content crossing a boundary). The difference is that ettle
   currently *asserts* its boundary (structural atom caps, "trust the schema")
   where this line *measures* the leak rate. **Liftable, and high-value:** adopt
   the flow-extraction + per-flow-judgment pattern to make ettle's boundary a
   measured filter, and borrow the leak-rate metric as a real benchmark for the
   privacy claim — more tractable than the team-coordination benchmarks in
   [BENCHMARKS.md](BENCHMARKS.md), because the artifact (did private content
   cross?) is directly checkable.

3. **Adjustable autonomy → still the governing lesson, now at A2A/MCP scale.**
   Electric Elves' transfer-of-control problem (Pynadath & Tambe) is exactly what
   the 2025–26 Agent-to-Agent / MCP autonomy wave re-opened: agents handling
   sensitive communications with too much autonomy. ettle's bind-vs-surface
   threshold *is* an adjustable-autonomy transfer-of-control decision; the formal
   MDP machinery from that line is a grounding for what is currently a hand-tuned
   controversy score.

**Net.** CALO's single-user mission is finished and the meeting-extraction half
is a commodity. The two pieces ettle is actually betting on — surprise /
necessity-gated proactivity and a *measured* inter-principal privacy boundary —
are both active, benchmarked 2025–26 research. The honest read is encouraging and
humbling at once: ettle is not alone on either, but it is one of the few attempts
to put **both** behind a single multi-principal boundary, and the
CI-operationalization line in particular hands ettle a tractable first benchmark
for its hardest claim.
