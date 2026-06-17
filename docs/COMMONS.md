# ettle — the coordination commons (Ostrom)

The thing ettle protects is a common-pool resource, and naming it that way is not a metaphor — it tells us what governs it and how it fails.

## The resource, and its tragedy

The commons is: **"we all got our shit done to a high degree of coordinated quality without wasting anyone's time."** Concretely, that resource is the team's **collective attention, mutual legibility, and trust**. It behaves like a commons:

- **Rivalrous** — every false interrupt, every needless meeting, every stale model someone acts on, every over-broadcast spends a finite pool of attention and trust that everyone draws from.
- **Non-excludable within the team** — everyone benefits from smooth coordination; everyone can degrade it.

So it has a specific **tragedy of the commons**: each agent, wanting to be sure *its* human is represented and never blamed for a missed heads-up, has a private incentive to **over-emit** — flood the shared horizon, surface everything, interrupt on any doubt. Individually rational, collectively ruinous: the horizon fills with low-relevance noise and the attention pool is grazed to dirt. The relevance-currency emit-gate (send only the delta that most changes a teammate's model per unit of their attention — Sperber-Wilson; see CONCEPT.md) is the **appropriation rule** that meters the grazing. But an appropriation rule without governance erodes. That's where Ostrom comes in.

## Ostrom's eight principles → ettle

From *Governing the Commons* (1990) — the design principles shared by commons that endured (Spanish *huerta* irrigation, Swiss alpine grazing, Japanese village forests) instead of collapsing:

1. **Clearly defined boundaries.** Who is in the commons, and what the resource is. → the consenting team (ADOPTION.md), and the resource = shared attention/legibility/trust. Never represent a non-participant; no state about a non-participant enters the horizon.
2. **Congruence — rules fit local conditions.** → the calibration loop tunes each team's emit thresholds and crux-detection to *that* team's tempo and norms, rather than one global rule.
3. **Collective-choice — those affected set the rules.** → the team decides what counts as bindable coordination vs. a crux worth surfacing, and where the emit thresholds sit. Humans stay the rule-makers.
4. **Monitoring, by monitors accountable to the members.** → the calibration loop is the monitor: model-vs-reality divergence, false-interrupt rate, did-it-help. A longitudinal calibration-metric store is its longitudinal substrate. The monitor reports to the humans, not to the agents.
5. **Graduated sanctions.** → **[gemot](https://github.com/justinstimatze/gemot)'s EigenTrust reputation.** An agent that over-emits, false-interrupts, or proposes binds the humans keep overriding loses standing. Sanctions escalate: first its contributions are down-weighted; then its emit threshold is raised so it must clear a higher relevance bar; then it loses the right to *bind* and must *surface* to its human instead. Reputation earned back the same way it's lost — by being right. (The concrete anti-overgrazing mechanics — surprise-gated emit, per-agent budget, O(1) shared reconcile, and the rule that L3 emits no atoms so there's no machine-speed loop — are in [SCALING.md](SCALING.md).)
6. **Cheap, accessible conflict-resolution.** → gemot deliberation for the small-N crux; crux-surfacing for the values call; Talk to the City / Polis for the large-N distribution of views (PRIOR_ART.md §7). Local arenas, not an escalation to management.
7. **Minimal recognition of the right to organize.** → consent-first, bottom-up adoption (ADOPTION.md). The platform does not impose coordination norms from outside; the team governs its own commons.
8. **Nested enterprises (polycentric governance).** → the L1/L2/L3 layering *is* nesting, and the N-spectrum (team → org → community → city) is polycentric scaling: small-N binds locally, large-N maps views, each layer governs itself and composes upward.

## Where it lives (the answer to "not sure where it should live")

It is a **governance layer over the whole mesh**, but the mechanisms land in specific, mostly-already-present places — and only one is genuinely new code:

- **Graduated sanctions → gemot reputation** is the concrete new home, and the most actionable: gemot already carries EigenTrust, so the anti-overgrazing teeth are a primitive we wire, not invent.
- **Monitoring → calibration loop / longitudinal calibration-metric store** (already the critical path).
- **Boundaries, collective-choice, right-to-organize → ADOPTION invariants** (already hard requirements).
- **Nested/polycentric → L1/L2/L3 + the N-spectrum** (already the architecture).
- **Conflict-resolution → gemot + crux-surfacing + TTTC** (already the deliberation layer).
- **Congruence → calibration tuning** (already how local rules adapt).

So Ostrom doesn't add a component so much as it **names the governance that was implicit and tells us the one missing piece is graduated sanctions** — which is exactly the gemot-reputation wiring already on the roadmap.

## The discipline (don't skip this)

Ostrom's principles govern commons among **humans who organize themselves**. In ettle the appropriators are *agents acting for humans*. So:

- **Monitoring and sanctions apply to the agents** — they are the ones who can overgraze.
- **Rule-making and collective-choice stay with the humans** — principles 3 and 7 are human rights, not agent capabilities. If the agents start setting their own emit rules and sanctioning each other without the humans, that is the self-governing collective the humans cannot audit — the treacherous-turn / Bicameral failure from HORIZON.md.

The bright version: a commons the team governs, with the agents as monitored, sanctionable appropriators of a shared attention pool. The dark version: a commons the *agents* govern, with the humans as the grazed resource. Same eight principles, opposite locus of control. Keep the rule-making human.

## Lineage

Elinor Ostrom, *Governing the Commons* (1990); Ostrom's later **polycentric governance** work. The graduated-sanctions ↔ reputation link runs through gemot's EigenTrust primitive, and there is a sibling polycentric-governance simulation that already exercises gemot's deliberation-and-reputation primitives under capture-resistance stakes — the natural test-bed for ettle's commons-governance, and a place to crib patterns rather than invent them.
