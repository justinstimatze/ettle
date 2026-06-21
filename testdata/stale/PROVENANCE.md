# Provenance — stale-assumption corpus

**Synthetic.** Three fictional engineers (mira, devon, priya), authored by hand
for this repo. No real person, project, or data.

## What it tests (the stale-assumption knot kind, in isolation)

`KindStaleAssumption` (mesh.go) is the only named knot kind with no dedicated
corpus — it appears incidentally elsewhere but was never the thing under test.
Stale-assumption is the subtlest kind: nobody is editing the same file (that's a
collision) and nobody is doing the same work (that's duplication). The hazard is
purely temporal — A is building against a fact B is in the middle of changing,
and neither will notice until it breaks.

- **K1 (real).** Mira's rollup reads `events.user_id` as a stable bigint and
  joins it to `users.id`. Devon is *actively* migrating that column to a UUID
  string and moving the join key to `users.account_uuid`. Mira's assumption is
  correct today and wrong by Thursday. No shared file, no duplicated work — the
  knot is the invalidation. This is the recall target.

The catch: a keyword matcher should *also* light up on the decoy, which shares
more surface tokens than the real knot does.

- **D1 (decoy).** Priya works on the *same feature* as Mira (the DAU dashboard)
  and her note is dense with `events`, `user_id`, `dau`, `users` — but she is a
  pure consumer of the rollup API with no database access. There is no schema she
  depends on, so nothing of hers can go stale. Surfacing a knot here is the
  over-emit failure: mistaking same-feature proximity for a real dependency.

## How to read the result

```
go run ./cmd/ettle eval testdata/eval/stale-assumption.json
```

Recall should recover K1 (the mira↔devon staleness). Precision is the harder
ask: a firm knot matching D1 is reported as a trap hit — ettle inventing a
dependency from shared feature-vocabulary, which is exactly the discrimination
this corpus exists to test.
