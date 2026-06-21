# Provenance — schema-collision corpus

**Synthetic.** Three fictional engineers (bex, cyrus, dana), authored by hand for
this repo. No real person, project, or data.

## What it tests (a dedicated collision, with a read-only decoy)

`standup-rename` proves collision detection on a code-signature rename. This
corpus isolates the *database* collision — two people defining the same column in
overlapping migrations — which is the highest-stakes collision in practice
(merge two migrations that both create `orders.status` and one fails on deploy).

- **K1 (real).** Bex's `0042` adds an `orders.status` enum; Cyrus's `0041`
  renames `orders.state` to `status` as an enum. Same column, same window, plus a
  migration-number ordering conflict. A genuine collision on a shared resource.

- **D1 (decoy).** Dana queries the orders table read-only for a report and says
  "status." She writes nothing and runs no migration — there is no resource to
  collide on. Surfacing her is the over-emit trap: mistaking a read-only consumer
  for a participant in the write collision.

## How to read the result

```
go run ./cmd/ettle eval testdata/eval/schema-collision.json
```

Recall recovers K1; precision holds if Dana (D1) is not asserted as firm. The
discrimination is read-vs-write on a shared table — being downstream of a schema
is not the same as colliding on it.
