# ettle — prior art and literature

Three passes: a knowledge-based review of the academic lineage, a shallow live web pass (June 2026), and a second shallow pass (June 2026, §7) prompted by the friction/empowerment alignment and the deliberation-routing the team sim now does. The deep-research harness was considered and rejected on cost each time.

**Headline finding.** Every individual component is being built, hard and well-funded — and the second pass found the *deliberation-routing* and *friction* sides are now built-for-us too (apply, don't invent). The specific assembly — multiplayer theory-of-mind across *personal* agents, privacy-bounded, modeling *people* rather than tasks, routing coordination vs. cruxes, with a per-human calibration loop, inside a consenting trust boundary — is, as far as this survey found, still unbuilt. The empty seat is bounded on every side by mature work, which is what makes it tractable now. See §7 for the latest pass.

---

## 1. Agent-to-agent protocols — settled infrastructure, not human-modeling

- **MCP** (Anthropic, Nov 2024): agent↔tool/context. Became the de-facto standard; OpenAI and Google adopted it through 2025. Not agent↔agent.
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
- **Privacy-Preserving Multi-Agent Planning with Provable Guarantees** (older) — https://arxiv.org/pdf/1810.13354
- **Confidential Computing Consortium**, Dec 2025: confidential computing as a strategic imperative for multi-party AI collaboration. https://confidentialcomputing.io/2025/12/03/new-study-finds-confidential-computing-emerging-as-a-strategic-imperative-for-secure-ai-and-data-collaboration/

Two takeaways for ettle: confidential computing / TEEs are the likely practical privacy substrate (MPC is too slow for interactive coordination; PSI stays as a cheap first-filter for "do our touched sets collide"); and the classic inference-attack caveat holds — agents can *deduce* private state even when it is never explicitly shared, so "agents infer quite a lot" cuts both ways and the boundary must be designed, not assumed.

## 3. Anticipatory team-coordination products — a crowded *shallow* category

The "fewer meetings via shared agent context" pitch is now mainstream — but it is the task-state version, not the people-modeling version.

- **Asana "AI Teammates"** — explicitly pitches absorbing the "coordination tax" (chasing, status pings, handoffs), auto-updating work when requirements change "without coordination meetings." Shared *project/task state* + persistent memory. https://asana.com/resources/ai-teammates-overview
- **monday**, and a broad "agentic async replaces meetings" crowd. https://techplustrends.com/async-ai-tools-global-teams-2026/ ; https://thenewstack.io/the-next-era-of-ai-from-single-user-to-team-collaboration/
- **Glean** — closest to an "org brain": enterprise knowledge graph plus a people graph (≈ transactive memory). Retrieval/assistant, per-person, not cross-agent forward-modeling.
- **Atlassian Rovo, Microsoft 365 Copilot, Dust, Notion AI, Slack/Agentforce** — agents over org data. Adjacent.
- **Meeting AI** (Read AI, Fireflies, Otter, Spinach) — the explicit "fewer meetings" pitch, but backward-looking: they summarize what happened. They are also the source of the adoption antipattern ettle rejects; see ADOPTION.md.

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

## The map, in one view

- Transport/discovery (A2A, MCP, AGNTCY, ACP) + the A2A "agent economy": crowded, funded, production — but cross-org and transactional, not intra-team.
- Privacy primitives (MPC/PSI/FL/DP/TEEs): mature; not aimed at agent intent.
- Org-brain assistants + coordination-tax products (Glean, Asana AI Teammates, Copilot/MS Teams Interactive Agents, monday, Rovo): funded; task-state orchestration, and trending toward *agents-in-the-meeting* — the foil, not the same move.
- Multi-agent deliberation & consensus-routing (Deliberative Curation, Learning-to-Negotiate, Model Council, Polis, Habermas Machine): now real — the bind-vs-surface layer is prior art to *apply*, not invent.
- Planners (HTN, DCOP, market-based, STEAM): mature; ettle feeds them dynamically off live reasoning.
- Friction / contestability framework (Terms-we-Serve-with, Contestable AI): the principled account of *which* friction to keep.
- Surprise-minimization-as-coordination (active inference / VERSES): real, commercial — but general-purpose / finance, not teams.
- Machine theory of mind (ToMnet, LLM-ToM): exists, brittle, not wired to coordination products.
- **The integration ettle describes — personal-agent people-modeling + privacy-bounded metaperception + per-human calibration + friction-in-the-right-spots, inside a consenting trust boundary: still empty. The sides are increasingly built-for-us; the middle is the work.**

The plumbing being done for us is what makes ettle buildable now. The unbuilt middle — L2/L3 metaperception plus per-human calibration plus a contextual-integrity boundary — is the genuinely novel and genuinely hard part. That is both the opportunity and the warning.

> Recency note: the academic lineage is solid; the 2025–26 product/funding/protocol specifics come from a shallow live pass and may move. The product facet is the weakest — a recently-funded company could be in the empty seat unseen.
