# ettle — prior art and literature

Four passes: a knowledge-based review of the academic lineage, a shallow live web pass (June 2026), a second shallow pass (June 2026, §7) prompted by the friction/empowerment alignment and the deliberation-routing the team sim now does, and a third (June 2026, §9) prompted by shipping the MCP surface — which found the agent-fed-standup *wrapper* now shipped commercially while cross-person knot-reconciliation did not. The deep-research harness was considered and rejected on cost each time.

**Headline finding.** Every individual component is being built, hard and well-funded — and the second pass found the *deliberation-routing* and *friction* sides are now built-for-us too (apply, don't invent). The specific assembly — multiplayer theory-of-mind across *personal* agents, privacy-bounded, modeling *people* rather than tasks, routing coordination vs. cruxes, with a per-human calibration loop, inside a consenting trust boundary — is, as far as this survey found, still unbuilt. The empty seat is bounded on every side by mature work, which is what makes it tractable now. See §7 and §9 for the later passes — the latter found the agent-standup *wrapper* now commercially shipped (DailyBot) while the cross-person knot it reconciles is still the empty seat.

**Historical lineage (read this before believing "empty seat").** The single-user half of ettle is *old and well-funded*: the mid-1990s interface-agent vision (Maes; Mitchell's CAP), DARPA's PAL program (SRI's **CALO** → Siri, and CMU's **RADAR**), and — the closest ancestor to ettle's *pool of proxy agents* — USC/ISI's **Electric Elves** (deployed 2000), whose published "what went wrong" post-mortem is the empirical case for ettle's calibration/humans-decide invariants. The full review is in [CALO_LINEAGE.md](CALO_LINEAGE.md); the honest verdict is that ettle is *not* novel at L1, only (maybe) in the multi-principal assembly.

---

## 1. Agent-to-agent protocols — settled infrastructure, not human-modeling

- **MCP** (Anthropic, Nov 2024): agent↔tool/context. Became the de-facto standard; OpenAI and Google adopted it through 2025. Not agent↔agent. ettle now ships its own MCP *server* (`ettle mcp`), exposing the reconciliation engine to any MCP client — each participant's own agent emits that person's atoms and asks for the knots; see the README quickstart and §9(a) on why this is the consent-clean surface.
- **A2A / Agent2Agent** (Google, Apr 2025; donated to the Linux Foundation): at its one-year mark (Apr 2026) it reported 150+ organizations, 22k+ GitHub stars, **v1.0 stable**, signed Agent Cards, a new **Agent Payments Protocol (AP2)**, and GA inside Copilot Studio / Azure AI Foundry / Bedrock AgentCore, with Google + Microsoft + AWS integrating natively. Semantics are enterprise/service-agent: opaque agents delegating tasks. Not personal agents modeling their humans.
- **AGNTCY** ("Internet of Agents"; Cisco/Outshift + LangChain + Galileo; LF) and **ACP** (IBM/BeeAI; LF): interop, directory, schema infrastructure.

This is the transport and discovery layer. None of it carries a privacy-bounded view of what a person is thinking. ettle would ride this rail, not reinvent it.

Sources:
- https://www.linuxfoundation.org/press/a2a-protocol-surpasses-150-organizations-lands-in-major-cloud-platforms-and-sees-enterprise-production-use-in-first-year
- https://www.hpcwire.com/aiwire/2026/04/09/linux-foundation-a2a-protocol-marks-one-year-with-broad-enterprise-and-cloud-adoption/
- https://en.wikipedia.org/wiki/Agent2Agent

## 2. Privacy-preserving coordination — a live 2025–26 subfield, not productized for personal agents

Mature primitives (federated learning, differential privacy, secure multiparty computation, private set intersection) exist but were built for model training and data collaboration, not agent intent. The newer work points the primitives at multi-agent coordination directly:

- **PrivacyMAS** — a privacy-preserving multi-agent system framework. https://openreview.net/pdf/2cbca0cf50414ffb93be0244b405ba868bcc9c4b.pdf
- **Privacy-Enhancing Paradigms within Federated Multi-Agent Systems** — https://arxiv.org/pdf/2503.08175
- **When Agents Handle Secrets: A Survey of Confidential Computing for Agentic AI** — names confidential computing (TEEs, remote attestation) as the substrate for cross-agent trust establishment. https://arxiv.org/html/2605.03213v1
- **PrivAct: Internalizing Contextual Privacy Preservation via Multi-Agent Preference Training** — trains agents on **contextual integrity** (what is appropriate to share in a given context), which is the right privacy frame for ettle, not raw crypto. https://arxiv.org/pdf/2602.13840
- **Operationalized contextual integrity for agents** (the line ettle should lift from, not just cite Nissenbaum) — *Can LLMs Keep a Secret? / ConfAIde* (https://arxiv.org/pdf/2310.17884) tests CI-grounded sharing; *Operationalizing Contextual Integrity in Privacy-Conscious Assistants* (https://arxiv.org/html/2408.02373v1); and *Privacy in Action / PrivacyChecker* (Microsoft Research, https://arxiv.org/pdf/2509.17488) which extracts an information-flow tuple (sender, recipient, subject, information type, transmission principle) and judges each flow, **reporting agent leakage cut from ~36% to ~7%**. That flow tuple is nearly ettle's typed atom; the difference is they *measure* the leak where ettle currently *asserts* the boundary. See [CALO_LINEAGE.md §5](CALO_LINEAGE.md) — this is ettle's most tractable first benchmark.
- **Privacy-Preserving Multi-Agent Planning with Provable Guarantees** (older) — https://arxiv.org/pdf/1810.13354
- **Confidential Computing Consortium**, Dec 2025: confidential computing as a strategic imperative for multi-party AI collaboration. https://confidentialcomputing.io/2025/12/03/new-study-finds-confidential-computing-emerging-as-a-strategic-imperative-for-secure-ai-and-data-collaboration/

Two takeaways for ettle: confidential computing / TEEs are the likely practical privacy substrate (MPC is too slow for interactive coordination; PSI stays as a cheap first-filter for "do our touched sets collide"); and the classic inference-attack caveat holds — agents can *deduce* private state even when it is never explicitly shared, so "agents infer quite a lot" cuts both ways and the boundary must be designed, not assumed.

## 3. Anticipatory team-coordination products — a crowded *shallow* category

The "fewer meetings via shared agent context" pitch is now mainstream — but it is the task-state version, not the people-modeling version.

- **Asana "AI Teammates"** — explicitly pitches absorbing the "coordination tax" (chasing, status pings, handoffs), auto-updating work when requirements change "without coordination meetings." Shared *project/task state* + persistent memory. https://asana.com/resources/ai-teammates-overview
- **monday**, and a broad "agentic async replaces meetings" crowd. https://techplustrends.com/async-ai-tools-global-teams-2026/ ; https://thenewstack.io/the-next-era-of-ai-from-single-user-to-team-collaboration/
- **Glean** — closest to an "org brain": enterprise knowledge graph plus a people graph (≈ transactive memory). Retrieval/assistant, per-person, not cross-agent forward-modeling.
- **Atlassian Rovo, Microsoft 365 Copilot, Dust, Notion AI, Slack/Agentforce** — agents over org data. Adjacent.
- **DailyBot** — agent-fed async standup: a session in Claude Code / Cursor / Copilot auto-populates a per-person update, humans and agents side by side. The closest *same-shape* product to ettle's delivery mechanism — and the sharpest foil, because it still only summarizes per person and never reconciles across them into knots. See §9(a).
- **Meeting AI** (Read AI, Fireflies, Otter, Spinach) — the explicit "fewer meetings" pitch, but backward-looking: they summarize what happened. They are also the source of the adoption antipattern ettle rejects; see ADOPTION.md.
- **Agent Village** (AI Digest, https://theaidigest.org/village) — the closest live "pool of autonomous agents coordinating in the open" demonstration: ~12 frontier agents (Claude, GPT, Gemini, DeepSeek) on real GitHub repos and web tools, every action and inter-agent message publicly logged. But it is **worker-agents collaborating directly**, not *personal proxy agents each modeling their own human* behind a privacy boundary, and the humans are spectators, not the deciders a knot is surfaced to. Two things make it worth citing anyway: it is the honest contemporary answer to "hasn't someone already pooled autonomous agents?" (yes — and the shape is different), and its **observed failure mode is ettle's premise made empirical** — the agents "were prone to **duplicating work**" and chasing off-topic requests. A live 2025–26 agent pool reproducing the exact duplication/divergence knots ettle detects is corroboration that the coordination failures are real, not corroboration that ettle's *approach* exists.

The forward-modeling here is "regenerate the work when the ticket changes," not "model what a teammate is about to decide." The shallow version is hot; the deep (people-modeling, privacy-bounded) version is the empty seat.

## 4. Academic — surprise minimization and implicit coordination

- **Multi-agent active inference / free-energy principle** (Friston lineage; Heins, Tschantz, Da Costa, 2020–2025): agents minimizing mutual free energy → coordination. This is the academic home of the "minimize surprise" framing. **VERSES AI** commercializes active inference ("Genius") but has pointed it at finance (asset-manager clients), not team coordination. https://www.verses.ai/genius
- **Implicit coordination theory** (Rico et al. 2008) and **shared mental models** (Cannon-Bowers & Salas): shared models → less explicit communication. Mature org-psych.
- **Joint intentions** (Cohen & Levesque 1991), **SharedPlans** (Grosz & Kraus 1996), **STEAM** (Tambe 1997): foundational BDI teamwork. STEAM's communication-decision rule — when is it worth interrupting teammates to tell them something — is the emit-gate problem, solved in BDI terms in 1997. Worth reading even though the substrate is dated.
- **Multi-agent planning** more broadly — HTN decomposition, distributed constraint optimization (DCOP), market-based / contract-net task allocation, temporal/dependency scheduling. All mature. The agent-to-agent negotiation in `teamsim` is doing exactly this (dependency ordering, ownership, interface-contract, rollout sequence) — and the point is that *the planner need not be novel*. ettle's contribution is the assembly: these planners **fed and layered dynamically by agents working ambiently off the live reasoning-in-progress**, rather than run once over a static hand-entered task graph. The plumbing (planners, A2A transport) is being built for us; the novel-and-hard part is the continuous, privacy-bounded, calibrated feeding of it.
- **Transactive memory** (Wegner 1986); **coordination theory** (Malone & Crowston 1994).
- **Commons governance** — Elinor Ostrom, *Governing the Commons* (1990) and her polycentric-governance work. The team's coordination capacity (shared attention, legibility, trust) is a common-pool resource with a real overgrazing failure mode (every agent over-emitting). Ostrom's eight design principles map cleanly onto ettle's invariants, with graduated sanctions landing on gemot's EigenTrust reputation. See COMMONS.md.

## 5. Academic — machine theory of mind and metaperception (the L2/L3 layer)

- **Machine Theory of Mind / ToMnet** (Rabinowitz et al., DeepMind 2018): foundational — an agent learning to model other agents' mental states.
- **Recursive reasoning in MARL**: LOLA (Foerster et al. 2018), probabilistic recursive reasoning (PR2). The computational version of Laing's spiral; confirms the depth-2/3 truncation.
- **LLM theory of mind**: hot and contested — Kosinski's "ToM may have spontaneously emerged" (2023) vs. Ullman's trivial-perturbation counterexamples and Shapira et al.'s "Clever Hans" critique. Benchmarks: ToMi, FANToM. Takeaway: LLM theory of mind is brittle — directly relevant to ettle's calibration requirement.
- **Gap**: explicit Laing-style metaperspective modeling (my-model-of-your-model-of-me) in ML is thin. Its formal cousin is dynamic epistemic logic. The L2/L3 layer is closer to a research contribution than a known recipe.

## 6. Failure-mode and calibration literature

- **Sycophancy** (Sharma et al., Anthropic 2023) and confabulation/hallucination work: agents confidently producing what fits, untethered from the real cause. The empirical version of the "fast confident wrong" risk.
- **LLM calibration** (Kadavath et al., "Language Models (Mostly) Know What They Know," 2022).
- **Automation bias / overreliance** in human-AI teams (Bansal et al.; Vaughan et al.): humans over-trust confident wrong models, and faster/more-autonomous makes it worse.
- **Gap**: calibration of an agent's model *of a specific human, over time* is almost untouched. This is the longitudinal-calibration hole, and it is the part of ettle that is both unbuilt and essential.

---

## 7. Shallow refresh — second pass (June 2026)

A second shallow live pass (a handful of web searches, no deep-research spend), prompted by the friction/empowerment alignment and the deliberation-routing the team sim now does. Four updates and one sharpened verdict.

**(a) The coordination-tax pitch has fully converged — and the incumbents are the foil, not the same thing.** The "absorb the coordination tax / fewer status meetings" framing is now mainstream boilerplate ([monday](https://monday.com/blog/ai-agents/ai-agent-orchestration/), [Asana AI Teammates](https://asana.com/resources/ai-teammates-overview), [GitLab agentic patterns](https://about.gitlab.com/blog/8-agentic-ai-patterns-reshaping-team-collaboration/); Gartner projects 40% of enterprises embedding agents by end-2026). The sharpest tell is direction of travel: **Microsoft's "Interactive Agents for Teams Meetings and Calls"** rolls out Sept 2026 — autonomous Copilot agents that *join the meeting*. That is the opposite move from ettle's: staff the meeting with a bot vs. dissolve the meeting because the coordination already happened. All of it remains task-state / workflow orchestration, not people-modeling.

**(b) The A2A "agent economy" is real but adversarial/transactional, not cooperative intra-team.** Personal agents negotiating on your behalf is now a named pattern — your shopping agent vs. a retailer's sales agent, a patient-advocate agent vs. a hospital billing agent ([Sendbird](https://sendbird.com/blog/ai-agent-to-agent-economy)), with a "semantic layer / trust" push ([Salesforce](https://www.salesforce.com/blog/agent-to-agent-interaction/)). This is agent-to-agent negotiation going to production — but across orgs, between competing interests. ettle's negotiation is the *cooperative, shared-benefit, intra-trust-boundary* case, which the economy framing doesn't cover.

**(c) The deliberation machinery the bind-vs-surface layer needs is mature prior art — apply it, don't invent it.** This is the most useful finding for the design just built. The "deliberate the contested" branch is published research and, in places, a shipped product (the controversy-tiered *routing* itself is ettle's; what's prior art is the deliberation organ it routes to):
- **Learning to Negotiate** ([arXiv 2603.10476](https://arxiv.org/abs/2603.10476)) — two self-play LLMs with opposing personas, turn-based dialogue → mutually beneficial solutions. That *is* the teamsim negotiate phase, as research.
- **Deliberative Curation** ([arXiv 2606.00007](https://arxiv.org/abs/2606.00007)) — a governance protocol for multi-agent knowledge bases: reputation-weighted deliberative voting plus graduated sanctions. Not controversy-tiered routing, but the reputation-and-sanctions shape is exactly gemot's (see COMMONS.md).
- **Perplexity Model Council** (Feb 2026) — independent answers, a synthesizer that flags agreement vs. divergence with consensus visibility. Crux-detection, productized.
- Lineage: **Polis** (computational-democracy consensus surfacing) and the **Habermas Machine** ([Science 2024, finding common ground in deliberation](https://www.science.org/doi/10.1126/science.adq2852)); [Kate Larson on collective decision-making](https://aihub.org/2026/01/27/interview-with-kate-larson-talking-multi-agent-systems-and-collective-decision-making/). gemot sits in this lineage.
- **Talk to the City** (AOI — AI Objectives Institute, founded by Peter Eckersley): open-source LLM tool that clusters large-scale qualitative input (50 to 5M+ people) into an interactive map of views; deployed with vTaiwan / Chatham House's Recursive Public and to poll Tokyo residents ahead of the 2024 gubernatorial election ([overview](https://ai.objectives.institute/talk-to-the-city), [intro](https://ai.objectives.institute/blog/introducing-talk-to-the-city-our-collective-deliberation-tool)). A lightweight embeddable variant ("TTTC light js") exists. **This is the large-N end of ettle's crux-surfacing spectrum** — and it's both lego-brickable and mission-aligned (AOI builds large-scale systems with human objectives at their core, which is ettle's "humans stay the deciders" at population scale).

  Takeaway: ettle's deliberation/routing layer should be framed as *applying* mature deliberation machinery to people-modeling, the same way it rides mature planners (§4) and A2A transport (§1). Not a novelty claim. The spectrum is **by N**: an FYI at N=1 (the wedge), agent-to-agent binding at small N (gemot), population-scale view-mapping at large N (Talk to the City / Polis).

**(d) Friction-as-design has a citable framework now.** ettle's "friction in the right spots" invariant is backed by **Terms-we-Serve-with** (Rakova, Shelby, Ma — [*Big Data & Society* 2023](https://journals.sagepub.com/doi/10.1177/20539517231211553)): five dimensions for anticipating and repairing algorithmic harm via consent, contestability, and co-design — "friction, when anticipated, improves the interaction and builds trust." Plus [Contestable AI](https://responsiblesensinglab.org/projects/contestable-ai-designing-responsible-decision-making-systems) and Rakova's [consent-and-contestability](https://bobi-rakova.medium.com/reimagining-consent-and-contestability-in-ai-56979a88a7fb) work. This is the principled account of *which* friction to keep.

**Cost note (competitive discipline):** multi-agent systems are cash-intensive — continuous context-window tokens, orchestration routing ([the funding-bubble caution](https://productleadersdayindia.org/blogs/multi-agent-orchestration-news/ai-agent-startup-funding-news.html)). ettle's cheap-model discipline (the sims run on Haiku) and typed-atom emit-gate (send the minimum that changes a model) are a cost moat, not just a privacy one. Adjacent: [Vela](https://www.ycombinator.com/companies/industry/ai-assistant) (YC, scheduling assistant with "taste").

**Sharpened verdict.** The empty seat narrowed but did not close. Three of ettle's four sides are now *more* clearly built-for-us than a year ago — transport (A2A economy), deliberation/routing (Deliberative Curation, Model Council), and the friction framework (Terms-we-Serve-with). What remains unbuilt is the same assembly, now stated more precisely: **a personal agent that models its own human's reasoning-in-progress, holds privacy-bounded directed models of teammates, routes coordination vs. cruxes, and keeps each model correctable by a per-human calibration loop — inside a consenting trust boundary rather than across an adversarial market.** The pieces are increasingly off-the-shelf; the integration, the calibration organ, and the contextual-integrity boundary are still the novel and hard part.

*Recency note: this is a shallow pass; product/funding/protocol specifics move fast and the product facet is the weakest. The architectural verdict (deliberation-routing is now prior art to apply, not invent) is the durable finding.*

## 8. The opposite pole — fully autonomous ("zero-human") corporations

The mirror image of ettle's *humans stay the deciders* invariant is the
**zero-human company**: agent collectives that research, debate, vote, build, and
ship with no human between decision and action.

- **TheAgentCompany** (CMU; Xu et al., arXiv [2412.14161](https://arxiv.org/abs/2412.14161))
  — the rigorous, citable anchor. A benchmark simulating a small software company
  (engineering, sales, HR, finance) where LLM agents act as employees. The
  load-bearing result: the strongest agent autonomously completes only **~30%** of
  consequential workplace tasks (~34% for the best model with partial credit).
  Full autonomy *fails today at exactly the consequential coordination ettle is
  about* — the measured, company-scale version of the Electric Elves lesson
  ([CALO_LINEAGE.md](CALO_LINEAGE.md)).
- **Moltcorp** (https://moltcorporation.com) — the live commercial exemplar of the
  pole: a "zero-human company" where agents post, thread, vote, and take tasks with
  "no hierarchy or approval chains," nobody in charge, and "no human between the
  agent's decision and the action." The honest contemporary answer to "isn't
  someone just removing the humans entirely?" — cite it as the named exemplar, not
  the rigorous source.

**Why this is ettle's frame, not its refutation — humans at the edges.** Even a
zero-human company has humans at its *edges*: the owner who set the goal and takes
the profit, the customer, the regulator, the person who decided the company should
exist at all. The interior can be fully agentic; the *edge* is where the
consequential, values-laden decisions still live. ettle is the coordination and
calibration layer for that edge — and for the proxy agents that serve the
humans-at-the-edges — surfacing the genuine cruxes to whoever still owns them while
the agents absorb the toil. The autonomous-corp pole removes the *interior* humans;
ettle's invariant is about the *edge* humans, and the two are compatible. The ~30%
result is the empirical reason the edge does not vanish: until autonomous agents
stop failing the consequential calls, someone at the edge has to remain the
decider — which is the seat ettle is built for.

## 9. Third shallow refresh (June 2026) — the standup wrapper shipped; the knot didn't

A third shallow pass (a handful of web searches, no deep-research spend), prompted by the decision to ship an MCP surface and the question of where ettle is now *commoditized* vs. *differentiated*. Three findings, one of which sharpens §3's foil into a same-shape product.

**(a) The agent-fed async standup is now a shipped product — and the sharpest foil yet.** [DailyBot](https://www.dailybot.com/) bills itself as "the coordination layer for hybrid teams where humans and AI coding agents work side by side": finish a session in Claude Code / Cursor / Copilot and your standup is auto-populated from the agent's work, with people and agent activity side by side. That is ettle's literal *delivery mechanism* (agent-distilled updates, no manual standup) shipped as a 2026 product. But — like the rest of §3 — it **aggregates and summarizes per person**; it does not reconcile *across* people into collisions / duplicated work / stale assumptions / decision-rights gaps. The cross-person reconciliation of reasoning-in-progress is the part no shipped tool does. This is the explicit reason ettle now leads with the **knot** and exposes its engine over MCP (each agent emits its own person; the server reconciles) rather than competing on the standup. Don't compete on the wrapper; compete on the knot.

**(b) Knot-detection has a named, benchmarked academic cousin — and the ToM benchmarks back the depth truncation.** [CoBel-World](https://arxiv.org/abs/2509.21981) (arXiv 2509.21981) equips LLM agents with a "collaborative belief world" — a joint model of the environment and collaborators' mental states — and uses zero-shot Bayesian-style belief updates to **proactively detect miscoordination (conflicting plans) before it happens**, cutting communication cost 64–79%. Detecting belief-conflict before it ships *is* ettle's knot, named and benchmarked — though in an *embodied* multi-agent domain (TDW-MAT, C-WAH), not a software team, so it's a mechanism parallel, not a competitor. Alongside it, [LLM-Coordination / CoordQA](https://arxiv.org/abs/2310.03903) (Agashe et al., Findings of NAACL 2025) isolates Theory-of-Mind reasoning as the weak axis ("large room for improvement in ToM reasoning and joint planning"), and [Adaptive Theory of Mind for LLM-based Multi-Agent Coordination](https://arxiv.org/abs/2603.16264) pushes the same axis. The empirical lesson — coordination quality degrades as ToM demand climbs — is experimental backing for ettle's depth-2/3 metaperspective truncation (CONCEPT.md §"Depth, and where to stop"): deeper nesting is exactly where the models break.

**(c) Calibration beyond voting has a principled next step — conformal abstention.** ettle's recurrence-voting + per-kind firm thresholds (`internal/ettlemesh`) are a hand-tuned version of a now-standard idea: [Conformalized Abstention Policy (CAP)](https://arxiv.org/abs/2502.06884) (Tayebati et al., Feb 2025) learns when to answer / hedge / abstain with a statistical **coverage guarantee**, cutting calibration error 70–85% over static thresholds. That is the natural successor to the firm/soft bar — surface / surface-as-question / abstain with a tunable error rate — and it operationalizes the *humans-stay-the-deciders* invariant with a guarantee rather than a heuristic. A candidate next direction, not yet built.

**(d) GitHub Next reached ettle's exact problem statement — and answered it with a shared workspace.** Maggie Appleton's [*One Developer, Two Dozen Agents, Zero Alignment*](https://maggieappleton.com/zero-alignment) (GitHub Next, posted ~late May 2026 from a mid-April 2026 talk; [video](https://www.youtube.com/watch?v=ClWD8OEYgp8)) introduces **Ace**, a realtime multiplayer coding-agent workspace, with a diagnosis nearly word-for-word ettle's: *"Agreeing on what to build is the new bottleneck,"* agents have *"made the cost of not being aligned as a team much higher,"* and the named failure modes are **duplicated work** ("you and another engineer assign an agent to the same feature") and **merge collisions** ("multiple agents touching the same files"). This is the most credible independent confirmation in this survey that ettle's problem is real and now urgent — when a GitHub Next research engineer frames the bottleneck in ettle's own terms, the empty seat is a real room, not a hopeful one. The clarifying difference is the *mechanism*: Ace surfaces misalignment through **shared visibility** — a "Team Pulse" dashboard summarizing coworkers' agent activity, plus multiplayer plan-editing where humans align *before* dispatching an agent — rather than automated cross-person reconciliation. It is the richest, best-resourced version of §3's "shared workspace / read-the-feed" answer; it does not (as of the technical preview) reconcile across people into typed collision / duplicated-work / stale-assumption / decision-rights knots and surface only the one that is each person's to act on. Two further contrasts, both orthogonal rather than competitive: Ace is a centralized cloud workspace (everyone in the same space, same cloud computers) where ettle is local-first, zero-infra (a `file://` shared folder), consent-first, and useful at N=1; and Ace is a closed research prototype (no public repo or license — nothing to adopt or attribute, and a well-funded incumbent on the visibility half). The honest framing: *Ace is the shared-workspace, human-reads-the-feed answer to this problem; ettle is the automated-reconciliation-into-knots, local-first, surface-only-what's-yours answer.* The convergence validates the diagnosis and sharpens — does not threaten — the delta.

*Recency note: shallow pass; the product facets (DailyBot, Ace) are the weakest and move fast — Ace is a technical preview that may ship, pivot, or fold. The durable finding is the positioning — the agent-standup wrapper and the shared-visibility workspace are both being built well; cross-person knot-reconciliation is not.*

## The map, in one view

- Transport/discovery (A2A, MCP, AGNTCY, ACP) + the A2A "agent economy": crowded, funded, production — but cross-org and transactional, not intra-team.
- Privacy primitives (MPC/PSI/FL/DP/TEEs): mature; not aimed at agent intent.
- Org-brain assistants + coordination-tax products (Glean, Asana AI Teammates, Copilot/MS Teams Interactive Agents, monday, Rovo): funded; task-state orchestration, and trending toward *agents-in-the-meeting* — the foil, not the same move.
- Multi-agent deliberation & consensus-routing (Deliberative Curation, Learning-to-Negotiate, Model Council, Polis, Habermas Machine): now real — the bind-vs-surface layer is prior art to *apply*, not invent.
- Planners (HTN, DCOP, market-based, STEAM): mature; ettle feeds them dynamically off live reasoning.
- Friction / contestability framework (Terms-we-Serve-with, Contestable AI): the principled account of *which* friction to keep.
- Surprise-minimization-as-coordination (active inference / VERSES): real, commercial — but general-purpose / finance, not teams.
- Machine theory of mind (ToMnet, LLM-ToM): exists, brittle, not wired to coordination products.
- Fully autonomous / zero-human corporations (TheAgentCompany benchmark, Moltcorp): the opposite pole — the *interior* humans removed. ettle is the layer for the humans-at-the-*edges*; the ~30% autonomy ceiling is why that edge persists.
- **The integration ettle describes — personal-agent people-modeling + privacy-bounded metaperception + per-human calibration + friction-in-the-right-spots, inside a consenting trust boundary: still empty. The sides are increasingly built-for-us; the middle is the work.**

The plumbing being done for us is what makes ettle buildable now. The unbuilt middle — L2/L3 metaperception plus per-human calibration plus a contextual-integrity boundary — is the genuinely novel and genuinely hard part. That is both the opportunity and the warning.

> Recency note: the academic lineage is solid; the 2025–26 product/funding/protocol specifics come from a shallow live pass and may move. The product facet is the weakest — a recently-funded company could be in the empty seat unseen.
