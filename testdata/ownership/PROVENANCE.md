# Provenance — ownership-dispute corpus

**Synthetic.** Three fictional engineers (hana, ivo, jana), authored by hand for
this repo. No real person, project, or data.

## What it tests (decision-rights, in isolation, post per-kind firm bar)

`decision-rights` is the flickery-but-real kind — it recurs in only ~30% of
samples, which is why it gets a lower firm bar (`firmVoteFractionByKind`,
mesh.go). It appears in `auth-migration` but never alone; this corpus isolates it
so the per-kind threshold is exercised against a clean positive and a clean decoy.

- **K1 (real).** Hana is rolling out a cross-service error standard unilaterally;
  Ivo holds that error format is an API-review-owned platform decision. Both claim
  the same choice — a decision-rights conflict (who owns it), not a collision
  (no shared file) and not duplication (not the same work). The recall target.

- **D1 (decoy).** Jana weighs streaming vs in-memory for her *own* export. It is
  a real open decision but **self-only** — no second party claims it. A
  decision-rights tangle requires contested ownership; a private deliberation is
  not a cross-person tangle. Surfacing it is the over-emit trap of reading any
  "decision" language as a coordination conflict.

## How to read the result

```
go run ./cmd/ettle eval testdata/eval/ownership-dispute.json
```

Recall recovers K1 (helped by the lower decision-rights firm bar); precision
holds if Jana's self-only deliberation (D1) is not asserted. The discrimination
is contested-ownership vs private-choice.
