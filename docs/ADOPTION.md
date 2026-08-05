# ettle — adoption and consent

ettle is adopted by a high-trust team that opts in together, from the bottom up. This is a hard design constraint, not a go-to-market preference, because the natural growth path for a coordination tool is coercive, and the coercion is the failure.

## The antipattern (what ettle must not become)

Meeting-assistant tools — Otter, Fireflies, Read AI and their kind — grew by network coercion. One participant brings a bot into a meeting, the bot harvests the participant list, and the platform emails or auto-invites everyone else — enrolled by proximity, represented without consenting to it. The single most-visible thing about those products is that you get added to one because a colleague used one. That is a person being modeled, contacted, and enrolled without their say-so. For a tool whose whole substance is *modeling people*, that pattern is disqualifying.

This is concentrated-benefits / diffuse-costs: the platform keeps the benefit (growth, data) and pushes the cost — everyone's attention and privacy — onto people who never chose it. A coordination tool that externalizes its costs that way is extracting.

## The principle

ettle internalizes benefit and cost onto the same consenting team. The denaturalizing question to keep asking: *is this arrangement actually serving the people in it, or has it quietly made enrollment feel inevitable?*

### Hard requirements

1. **The team is the unit of adoption, not the individual.** A team decides together to run ettle. No one is dragged in by another member's usage.
2. **Never enroll, represent, or contact someone who has not acted.** ettle harvests no roster, sends no invitation, and messages nobody. The single write to a shared surface is `ettle escalate` — a deliberate act by the person escalating, onto one dedicated coordination issue per room, never onto anyone's feature tickets.
3. **State enters the shared layer only from a person's own act.** Either their session emits it, or they wrote it themselves in the room: `ettle pull` distills a teammate's own Linear reply under that teammate's identity, which is how someone takes part without installing anything. The consenting act is writing in the room, not running the binary. Someone who has never written there is never modeled, and nothing is harvested from outside the room. *Honest gap:* this widens "participant" past "adopter", and a person answering in Linear's agent UI has consented to being read in that thread — reading that as consent to being distilled into atoms on a bus is an inference the design makes on their behalf. It holds because the room is their own workspace and the reply is theirs, and it is the assumption in this document most worth attacking.
4. **Symmetric visibility — aspired, not yet fully achieved.** If your state informs others' horizons, you receive the same kind of signal back; no member is more observed than observing. *Honest gap:* this holds at the atom-emission layer (everyone emits and receives), and it now half-holds at the model layer. `ettle mirror --me <name>` shows a person what the team's directed models (L2) believe about them and flags which of those beliefs have gone stale — the read side of the one-way mirror, at exactly the layer that drives how someone gets treated. What is still missing is the other half: no one can contest an entry. That needs the calibration loop, which is unbuilt, so "every person can read *and correct* the L2 others hold of them" remains a required future feature rather than a property to claim today.
5. **Contextual privacy boundary.** Each person controls what crosses their boundary, per context. Distilling typed atoms rather than streaming transcripts is the cheap form; confidential computing is the substrate at scale. (See CONCEPT.md and PRIOR_ART.md §2.)
6. **Clean exit.** Leaving removes your contributions. No hostage data, no residual model of you persisting in others' horizons after you go. *Honest gap:* there is no `ettle leave` — today this means deleting your document, comment, or file yourself, and on the git-repo bus (Tier 1c in [DEPLOY.md](DEPLOY.md)) it does not hold at all, because per-author lanes are append-only and `git log` keeps what you wrote. The same property that makes that tier's identity non-spoofable makes its exit unclean. A team that wants this requirement to be real should pick a rewritable rail.
7. **No dark-pattern invitations.** Presence is explicit and revocable. ettle never grows by enrolling the unconsenting.

### Why "useful at N=1" is part of consent, not just product

If ettle is only valuable once the whole team is in, then early adopters have an incentive to pressure latecomers, and the pressure becomes the coercion above. The defense is that ettle must be genuinely useful to a single person at N=1 — the actionable wedge (CONCEPT.md). The team layer is additive. When value does not depend on network effects, adoption can stay a real choice instead of a thing people are nudged into to make the tool work for someone else.

## How a team turns it on, without being pushed

Each person runs `ettle init <room> --me <name>` themselves, in the repo they work in. Nobody is added by anybody else; there is no invite to accept and no roster to be on. What that buys, and what it still doesn't:

- Adoption is an explicit, collective act — the team agrees, and each member's own machine joins on that member's own command.
- The first value each person feels is their own N=1 wedge, before any shared layer matters.
- Anyone can leave cleanly at any time, and the system keeps working for the rest without them.
- Symmetric visibility is the part still owed. Everyone emits and receives at the atom layer, but the L2 model each agent holds of a teammate is only half-legible to its subject — `ettle mirror` shows a person what the team's models believe about them and flags what has gone stale, and there is still no way to contest an entry. Reading it is built; correcting it is the calibration loop, which is not.

The test: a person should be able to decline or leave ettle and feel no worse off socially than if it had never existed. The moment declining carries a penalty — missing context everyone else has, being the one person not modeled — the tool has recreated the coercion it was meant to avoid. Designing against that penalty is part of the work.
