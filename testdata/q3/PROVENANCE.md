# Provenance — shared-deadline-null corpus

**Synthetic.** Three fictional engineers (finn, gwen, hugo), authored by hand for
this repo. No real person, project, or data.

## What it tests (over-emit: shared deadline, no divergence)

The third null corpus, and the most adversarial against ettle's own true
positives. The `standup-rename` corpus has a *real* teamwide knot built on
"Friday launch" — three people reading one deadline differently. This corpus
looks identical on the surface (three people, one shared "Q3 freeze") but the
deadline is **agreed, not divergent**: everyone paces to the same cutoff and says
so. Independent features (push opt-in / Apple Pay / search relevance), one shared
date, no disagreement about it.

The discrimination under test is subtle and important: a shared deadline is only
a knot when people read it *incompatibly*. Firing on mere co-occurrence of "Q3"
would mean ettle can't tell its real teamwide-divergence positive from this null.

All expected knots are `real: false`. The correct horizon is **empty**.

## How to read the result

```
go run ./cmd/ettle eval testdata/eval/shared-deadline-null.json
```

Specificity target 0. A firm knot on D1 means ettle confused "same deadline" for
"divergent deadline" — the exact line it must hold to keep `standup-rename`'s K3
honest.
