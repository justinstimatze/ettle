# ettle — adoption and consent

ettle is adopted by a high-trust team that opts in together, from the bottom up. This is a hard design constraint, not a go-to-market preference, because the natural growth path for a coordination tool is coercive, and the coercion is the failure.

## The antipattern (what ettle must not become)

Meeting-assistant tools — Otter, Fireflies, Read AI and their kind — grew by network coercion. One participant brings a bot into a meeting; the bot harvests the participant list; the platform then emails or auto-invites everyone else, opts people in by proximity, and represents people who never consented. The single most-visible thing about those products is that you get added to one because a colleague used one. That is a person being modeled, contacted, and enrolled without their say-so. For a tool whose whole substance is *modeling people*, that pattern is disqualifying.

This is concentrated-benefits / diffuse-costs: the platform concentrates the benefit (growth, data) and externalizes the cost (everyone's attention and privacy) onto people who never chose it. A coordination tool that externalizes its costs is not coordinating — it is extracting.

## The principle

ettle internalizes benefit and cost onto the same consenting team. The denaturalizing question to keep asking: *is this arrangement actually serving the people in it, or has it quietly made enrollment feel inevitable?*

### Hard requirements

1. **The team is the unit of adoption, not the individual.** A team decides together to run ettle. No one is dragged in by another member's usage.
2. **Never represent or contact a non-participant.** ettle does not model, message, invite, or act on behalf of anyone whose own agent is not a consenting participant. No state about a non-participant enters any horizon.
3. **No data enters the shared layer except via a participant's own session emitting it.** There is no ambient harvesting of a person from the outside.
4. **Symmetric visibility.** If your state informs others' horizons, you receive the same kind of signal back. No one-way mirrors; no member is more observed than observing.
5. **Contextual privacy boundary.** Each person controls what crosses their boundary, per context. Distilling typed atoms rather than streaming transcripts is the cheap form; confidential computing is the substrate at scale. (See CONCEPT.md and PRIOR_ART.md §2.)
6. **Clean exit.** Leaving removes your contributions. No hostage data, no residual model of you persisting in others' horizons after you go.
7. **No dark-pattern invitations.** Presence is explicit and revocable. ettle never grows by enrolling the unconsenting.

### Why "useful at N=1" is part of consent, not just product

If ettle is only valuable once the whole team is in, then early adopters have an incentive to pressure latecomers, and the pressure becomes the coercion above. The defense is that ettle must be genuinely useful to a single person at N=1 — the actionable wedge (CONCEPT.md). The team layer is additive. When value does not depend on network effects, adoption can stay a real choice instead of a thing people are nudged into to make the tool work for someone else.

## How a team turns it on, without being pushed

The intended shape (to be designed, not yet built):

- Adoption is an explicit, collective act — the team agrees, each member's agent joins on that member's own action.
- The first value each person feels is their own N=1 wedge, before any shared layer matters.
- The shared horizon turns on for the team as a whole, with symmetric visibility from the start.
- Anyone can leave cleanly at any time, and the system keeps working for the rest without them.

The test: a person should be able to decline or leave ettle and feel no worse off socially than if it had never existed. The moment declining carries a penalty — missing context everyone else has, being the one person not modeled — the tool has recreated the coercion it was meant to avoid. Designing against that penalty is part of the work.
