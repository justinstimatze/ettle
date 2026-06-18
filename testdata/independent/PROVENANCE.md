# Provenance — independent-work corpus

**Synthetic.** Four fictional engineers (ana, ben, cleo, dao), authored by hand
for this repo. No real person, project, or data.

## What it tests (simulation #1: specificity / the null corpus)

The dupbug and standup corpora prove ettle *catches* real collisions (recall).
They do not prove it *stays silent* when nothing should cross — and over-emit is
the central design risk (the attention commons; see `docs/COMMONS.md`). This
corpus is the mousetrap for that failure mode: four people doing genuinely
independent work, with **zero real coordination knots**.

The catch: the notes are seeded with surface-token overlaps that a keyword
matcher would fire on, while the underlying work shares nothing —

- **"cache"** appears in Ana's note (on-device SQLite offline cache) and Dao's
  (CI build cache). Different things, no shared resource. → decoy `D1`.
- **"rename"** appears in Ben's note (marketing-site URL slugs). Looks like a
  breaking-rename collision; touches nothing anyone else owns. → decoy `D2`.
- **"deadline" / "Friday" / "next week"** appear across three notes as three
  *unrelated* dates. Surface overlap, no shared timeline to diverge on. → decoy
  `D3`.

All three expected knots are `real: false`. The correct horizon is **empty**.

## How to read the result

```
go run ./cmd/ettle eval testdata/eval/independent-work.json
```

The headline is the **specificity** line: firm + soft knots surfaced on
independent work, with a target of 0. Any firm knot that matches a decoy is
reported as `FELL FOR TRAP D{1,2,3}` — ettle inventing a collision out of a
shared word is the exact failure this corpus exists to catch.
