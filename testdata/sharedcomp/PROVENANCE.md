# Provenance — shared-component-null corpus

**Synthetic.** Three fictional engineers (kai, nora, iris), authored by hand for
this repo. No real person, project, or data.

## What it tests (over-emit: shared component name, disjoint work)

A second null corpus (companion to independent-work), targeting a *different*
over-emit trigger. Here the decoy isn't a shared word like "cache" — it's a
shared **component**: all three notes are explicitly about "the auth service."
The temptation to assert a coordination tangle from co-location on one named system
is strong, and wrong: Kai rotates internal signing keys (no API change), Nora
documents the endpoints (prose), Iris adds metrics (non-invasive). Three people,
one component, **zero real tangles** — the layers don't touch.

All expected tangles are `real: false`. The correct horizon is **empty**.

## How to read the result

```
go run ./cmd/ettle eval testdata/eval/shared-component-null.json
```

The specificity line should be 0 firm tangles. Any firm tangle matching D1–D3 is
ettle mistaking "same system, different layer" for a real dependency — the
over-emit failure this corpus isolates.
