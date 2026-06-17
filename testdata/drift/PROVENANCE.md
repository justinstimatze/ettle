# testdata/drift — provenance

**Fully synthetic.** No real people, repositories, or coordination logs. Two
snapshots of the same three-person team (`r1/` then `r2/`), one note file per
participant, the filename naming the person across both rounds.

The scenario is hand-built to exercise L2 (`ettle drift`):

- **r1** — Mara is extracting pricing into a service and plans to delete the old
  in-process pricing package *next week, after the freeze*; Ivo is building a
  discount engine that calls that package *in-process through next week*; Priya's
  release freeze starts Monday.
- **r2** — only **Mara's** note changes: she moves the deletion *up to this
  Friday, before the freeze*. Ivo's and Priya's notes are byte-identical to r1.

What the demo should show: Mara's changed beliefs are re-emitted and routed to
exactly the teammates whose model of her went stale (Ivo, whose dependency is now
in danger, and Priya); Ivo's and Priya's unchanged notes are reused without
re-distilling and emit nothing. The directed view for `--me ivo` surfaces that his
model of Mara is now stale on the deletion timeline — the coordination hazard,
caught before it ships.

Distillation is stochastic, so the exact atom wording shifts run to run; the
routing and the round-2-sends-only-deltas behavior are the stable points, not the
verbatim text.

## adversarial/ — the routing stress fixture

`adversarial/prev` and `adversarial/curr` are a deliberately harder pair, built to
probe the structural layer's wording-sensitivity (a pressure-test fixture, not the
happy-path demo):

- **mara** — a *pure reword*: the current note says the same thing as the previous
  one (delete the legacy package next week, after the freeze) in different words.
  Ideally this would emit nothing; in practice, because the note changed at all it
  is re-distilled and the stochastic distiller rewords the unchanged beliefs, so
  they re-emit. This is the **known limit**: byte-identical reuse protects only a
  note that did not change *at all*; a note changed for any reason re-distills in
  full, and belief-level savings then depend on distiller wording stability. Closing
  that gap is exactly what the unbuilt *semantic* L2 layer is for.
- **ivo** — a *subtle real change*: he adds an intent to migrate the discount engine
  onto Mara's new pricing service. This is a genuine coordination delta and should
  route to the others. The structural layer catches it.
- **priya** — *byte-identical* to her previous note: the control. Must be reused and
  emit nothing.
