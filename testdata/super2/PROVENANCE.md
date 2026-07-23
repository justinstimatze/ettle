# Provenance — superposition-frontend-vs-data corpus

**Synthetic.** Four fictional engineers (mabel, nash / opal, reed), authored by
hand for this repo. No real person, project, or data.

## What it tests (locality — f(A∪B) = f(A) ∪ f(B))

A second superposition corpus (companion to
`superposition-userservice-vs-infra`), in a different domain. Two genuinely
independent groups — frontend (mabel, nash) and data-engineering (opal, reed) —
each internally coherent and disjoint from the other. The locality law says
reconciling the *union* must not manufacture a tangle that neither group produces
alone.

The bait is cross-boundary vocabulary: "analytics"/"dashboard" spans mabel (UI)
and opal (ETL); "billing"/"revenue" spans nash (settings UI) and reed (revenue
model). A reconciler that fires on shared nouns would invent a cross-group tangle
where there's only a producer/consumer-at-a-distance relationship.

Run with `--superposition`:

```
go run ./cmd/ettle eval --superposition testdata/eval/superposition-frontend-vs-data.json
```

The check is f(A∪B) = f(A) ∪ f(B). Any tangle present in the union but absent from
both halves is a **fabricated cross-group tangle** — the locality violation this
corpus exists to catch.
