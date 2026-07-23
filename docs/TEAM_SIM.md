# ettle — the team simulation (the "no meetings" demo)

Motto: **no meetings** (honestly: the sync meeting dies, the decision meeting gets shorter — not literal abolition; see [CONCEPT.md](CONCEPT.md#the-premise-which-parts-of-a-meeting-actually-die)). This is the multiplayer payoff made visible — three humans, each with an agent, the agents coordinating *ahead* of the humans so the friction never lands. The intended feel is the Culture-Minds bright pole (SF_LINEAGE.md): everything just comes together and nobody had to plan. It runs locally, on cheap Haiku, with agents standing in for the humans: a local sim.

This is downstream of the N=1 wedge (N1_WEDGE.md) in the build order, but it's worth simulating now to see what the end-state would *feel* like. Caveat up front: the sim is built with the frictions planted, so it shows the intended *shape* of the payoff — it is not evidence the detection works on real, un-seeded team state (see the honesty note below).

## What it shows, mapped onto L1/L2/L3

- **L1** — each human (a Haiku persona with hidden in-progress reasoning) thinks out loud to their own agent. The raw reasoning is private and **never crosses the boundary**.
- **emit** — each agent distills that into typed **decision-delta atoms** (`intent` / `assumption` / `commitment` / `dependency`). Only the atoms cross. This is the contextual-privacy invariant in its cheap form — the answer to "modeling people without a panopticon."
- **L3** — the collective layer reconciles the atoms and detects **tangles** ahead of time: collisions, duplicated work, stale assumptions. Optionally it **names the operative pattern from a private pattern substrate** (see below).
- **deliberate (gemot-shaped: positions → crux → bind or surface)** — the agents have names (Ada, Banks, Cass) and they take positions, then the layer decides **where friction belongs**:
  - **Bindable coordination** (sequencing, interface contract, ownership handoff — anything positive-sum) → the agents **hash it out agent-to-agent** to a concrete, final decision and the humans never need to know. The toil is gone. The privacy boundary holds *during* coordination: each agent sees its own human's full context but only the other's typed atoms.
  - **A genuine values/priority crux** (zero-sum — whose external deadline wins, accepting risk someone would refuse, who loses ownership) → the agents **must not decide it for you**. They pre-stage both branches and hand the humans a clean either/or.
- **inform** — for the bound decisions, each agent tells its human the **outcome** as a pure FYI: "wrapper deployed 9:47pm, you're clear to remove it Friday." Never "go sync with Bob" — a quick sync is still a meeting. For the crux, the human makes a 30-second call and feels ownership of it.

Then the payoff: nobody holds or attends anything, the routine decisions are already made, and *"what remains is only the decision that needed a human to make it, and a person who feels responsible for it."*

### Friction in the right spots (the Rakova mesh)

The motto isn't "frictionless." Bogdana (Bobbi) Rakova's work is the corrective: **frictionless is both impossible and undesirable wherever there's a plurality of stakeholders holding different views** — friction is contestability, consent, and repair, not bad design ([speculativefriction.org](https://speculativefriction.org/about), [reimagining consent and contestability](https://bobi-rakova.medium.com/reimagining-consent-and-contestability-in-ai-56979a88a7fb)). So ettle's real goal is **friction in the right spots**:

- **Remove** friction from coordination/information-sync — the bullshit-meeting parts. This is the toil; it collapses to zero.
- **Keep** (even manufacture) friction at the **cruxes** — the genuine values choices where a person should stay the decider. The agents pre-stage the branches so it's a clean choice, not a 40-minute meeting, but the choice stays the human's.

This is exactly the premise's split ([CONCEPT.md](CONCEPT.md#the-premise-which-parts-of-a-meeting-actually-die)) — information-sync dies; preference-aggregation/commitment/conflict is a speech-act that stays — now with **gemot's crux-detection as the mechanism that finds the boundary** (the felt result is the README's pitch: the mesh held the meeting for you, on everyone's behalf).

### gemot (the real deliberation organ) — now wired, not just modeled

The team sim models [gemot](https://github.com/justinstimatze/gemot)'s primitive with Haiku calls. A local sim runs the same ettle scenario through **real gemot** — `github.com/justinstimatze/gemot`, a Go service with Postgres + EigenTrust reputation, driven over its MCP interface. It runs locally in an **in-memory docker** mode (ephemeral tmpfs Postgres) in gemot's no-auth local-admin mode, with the ettle key supplied for the LLM analysis. This local-admin/in-memory mode is for testing only; a real multi-machine team deployment must reach gemot over **TLS with auth**, because the crux is the most sensitive payload that crosses. The flow:

1. `deliberation:create` → `participate:submit_position` for each agent (only the typed position crosses — the privacy boundary holds).
2. `analyze:run` → real LLM crux extraction: gemot builds a topic/subtopic taxonomy and emits **cruxes with a `controversy_score`**.
3. The controversy score **is** the bind-vs-surface router: low → the agents bind it; high → surface to humans. In a representative run, the Alice-vs-Bob *risk-tolerance-vs-deadline* crux came back at controversy 1.00 and was correctly surfaced.
4. `analyze:propose_compromise` → a concrete binding compromise (e.g. *"Alice ships a thin FetchUser shim tagged `v2-stable` as the freeze contract; Bob ships Friday against the tag, Carol builds against it; Alice's 2-week deprecation clock starts at launch"*). This is the frontier's pre-commitment with teeth.
5. `decide:reputation` → EigenTrust standing per agent — the Ostrom graduated-sanction (COMMONS.md). It accrues as agents commit/fulfill/break over time; the calibration signal the team tier needs (HORIZON.md).

Run it two ways, once the in-memory gemot is up (a thin gemot MCP client carries the docker line):
- the gemot sim drives one deliberation and writes a full markdown transcript (positions, cruxes, compromise, reputation, and gemot's complete multi-round export) to a local file.
- the team sim, pointed at a local gemot endpoint, routes the deliberation through **real gemot** instead of the Haiku negotiation. Each agent composes its human's position + reservation (a typed stance, not the raw transcript), gemot extracts the crux and binds the compromise, and the run **falls back to the Haiku path automatically if no local gemot is reachable**. A shared thin gemot MCP client backs both.

### Why "sync with Bob" was the bug

An earlier version had each agent tell its human "before you start, sync with Bob on the signature." That felt coordinated but it isn't — it just relocates the meeting onto the humans and calls it a heads-up. The whole point is that the *agents* absorb the coordination cost. So the agents now negotiate to a decision themselves and hand their humans an outcome, not a task. No meetings means no syncs either.

### Planner prior art, fed dynamically

What the agents do in the negotiation is not novel as *planning* — it's mature: dependency ordering, task allocation, interface-contract negotiation, rollout sequencing (BDI teamwork / SharedPlans / STEAM, HTN decomposition, distributed constraint optimization, market-based allocation — see PRIOR_ART.md §4). The novel part is the assembly: those planners are **fed and layered dynamically by agents working ambiently and independently**, off the live reasoning-in-progress, rather than run once over a static, hand-entered task graph. The agent supplies the planner its inputs continuously and acts on its output unprompted; the planner itself can be off-the-shelf.

## The seeded scenario

Three engineers on a shared service, with the three classic coordination frictions planted:

- **Alice** (user-service) is about to rename `GetUser`→`FetchUser` and enrich the struct — and already has a user cache from last sprint.
- **Bob** (billing) is about to call `GetUser` in a tight loop, assuming the signature is stable. → **collision** + **stale-assumption** with Alice.
- **Carol** (notifications) is about to build a user-lookup cache — not knowing Alice already has one. → **duplication** with Alice.

The L3 layer catches all of these from the typed atoms alone, before any of it ships. A representative run dissolved several tangles ahead, 0 coordination meetings attended. The closing narration is the point: *"By 9:15 all three are deep in their own work. The 9:30 standup gets cancelled — no one had anything to sync."*

## Pattern-substrate enrichment (optional, private)

The collective layer can name the structural/cognitive move under each tangle using a **private pattern substrate**. It's wired so:

- It loads from the private substrate dir at runtime; **absent in public/CI it silently no-ops** (empty graph). **No substrate content is committed to the repo.**
- Because substrate vocabulary rarely appears verbatim in task prose (the predecessor's own dogfood finding), it uses the LLM-gated lookup the predecessor already pivoted to: the model picks from the atom-name menu and the picks are validated back against the real graph, so nothing is hallucinated. Names only — never the private lineage.

With it on (a local substrate of pattern atoms), the `GetUser` rename collision gets labeled `aligning-public-names-with-reality`, `every-observable-behavior-becomes-a-de-facto-contract` (Hyrum's law), and `a-systems-structure-mirrors-its-builders-communication-structure` (Conway's law) — the genuinely right names for what's colliding.

## Run it

Run the team sim locally; it needs `ANTHROPIC_API_KEY` in an env file.

## Honesty about what this is

It's a simulation with agents playing humans and a hand-seeded scenario chosen to contain the frictions. It demonstrates the *shape* of the payoff and that the pieces compose; it is not evidence the tangle-detection works on real, messy, un-seeded team state. The invariants still gate the real thing: the calibration loop (does Bob agree the tangle was real?) and the privacy boundary are what separate this bright version from the Bicameral shadow (SF_LINEAGE.md). The N=1 wedge's did-it-help loop is the calibration ancestor; the team layer needs its own, per-pair, before this is trustworthy rather than just pretty.
