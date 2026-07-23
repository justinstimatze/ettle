# Provenance — calendar-divergence corpus

**Synthetic.** Three fictional engineers (jun, kara, liam), authored by hand for
this repo. No real person, project, or data.

## What it tests (teamwide-divergence, in isolation — and vs the null)

`KindTeamwideDivergence` (three+ people holding incompatible beliefs about a
shared fact) appears in `standup-rename` but never alone. This corpus isolates it
and, critically, pairs with `shared-deadline-null`: that corpus has a shared
deadline everyone *agrees* on (no tangle); this one has a shared freeze everyone
reads *differently* (a real tangle). Together they pin the exact discrimination —
divergence is about disagreement, not co-occurrence.

- **K1 (real).** Jun thinks the freeze is the 27th, Kara thinks it moved to the
  30th, Liam thinks there's no freeze (continuous deploy). Three incompatible
  beliefs about one shared fact — the classic teamwide divergence, and the kind
  most likely to cause a blown handoff. The recall target.

- **D1 (decoy).** Jun's "reviewers" and Kara's "design review" share the token
  "review" but are unrelated processes — not a contested shared fact.

## How to read the result

```
go run ./cmd/ettle eval testdata/eval/calendar-divergence.json
```

Recall recovers the 3-party K1; precision holds if the "review" coincidence (D1)
is not asserted. Read alongside `shared-deadline-null` — the pair is the real
test: ettle must fire here and stay silent there.
