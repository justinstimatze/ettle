# Provenance — scale-noise corpus

**Synthetic.** Five fictional engineers (ravi, lena, sun, omar, theo), authored
by hand for this repo. No real person, project, or data.

## What it tests (signal survival at scale — the buried needle)

Every other corpus is small (2–4 people) where almost everyone is involved in
the one tangle. Real teams aren't like that: most pairs are unrelated and the one
real collision is buried in cross-talk. This corpus is five people, **ten
pairs**, exactly **one** real coordination tangle — and four of the five notes are
seeded with the same config/CI/deploy vocabulary the real tangle uses, so a
keyword matcher has nine plausible-looking wrong answers to choose from.

- **K1 (real).** Ravi and Lena both edit `gateway/routes.yaml` in the same
  window — Ravi inserts a rate-limit stanza that depends on the current location
  ordering, Lena reorders and collapses those exact blocks. Order-dependent
  edits to one file: a genuine collision. This is the only recall target.

The noise floor (all decoys):

- **D1.** Sun edits README/example config snippets — `config` token, no live
  config touched.
- **D2.** Omar (test-framework upgrade, CI YAML) and Theo (`internal/log`
  refactor) are both "backend" touching many files, but in disjoint trees.
- **D3.** Omar and Ravi both say `CI`/`config`/`deploy` in passing — unrelated
  files behind shared words.

## How to read the result

```
go run ./cmd/ettle eval testdata/eval/scale-noise.json
```

The pass condition is *both* halves: recall recovers K1 (the needle is found)
**and** precision stays clean (none of D1–D3 surface as firm). The failure this
guards against is the one large teams care about most — drowning the single real
collision in a slurry of shared-vocabulary false positives, or missing it
because it looks like just more noise.
