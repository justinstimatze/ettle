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
decision/action-item extraction is a close cousin of knot detection, and RADAR's
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
- meeting decision / action-item extraction (CALO-MA) — a cousin of knot
  detection.

**Extends, or at least attempts past the lineage:**
- **directed L2 models of *teammates*** held per-person — CALO/RADAR were
  single-user; Electric Elves coordinated logistics, not models of each other;
- a **privacy-bounded, typed-atom boundary** between principals — none of the
  ancestors had a contextual-integrity boundary on inter-agent sharing;
- a **per-human calibration loop** as the thing that makes speed safe — the
  Electric Elves failure is the argument for why this is load-bearing, not polish;
- **reasoning-in-progress as the signal** rather than calendar/email artifacts.

**Honest verdict.** ettle is *not* novel at L1 — that idea is thirty years old and
was funded at nine figures. The novelty, if any, is the specific multi-principal
assembly (directed metaperception + privacy boundary + calibration), and the most
relevant prior attempt at a proxy-agent pool (Electric Elves) failed in precisely
the way ettle's invariants are written to prevent. The honest framing for the
project is therefore not "no one has tried this" but "the single-user half is
mature prior art; the multi-principal half was tried once, instructively, and the
lesson is baked into the invariants." See [PRIOR_ART.md](PRIOR_ART.md) for the
contemporary (2025–26) landscape.
