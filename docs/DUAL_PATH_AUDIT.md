# Dual-path audit

A **dual path** is a recurring, high-cost bug class: *the same invariant is derived
independently in more than one place, so the two derivations drift apart and one
quietly stops enforcing what the other does.* It is most dangerous when the duplicated
logic is a **safety invariant** (here: what a coordination atom must be stripped of
before it crosses the privacy boundary), because the divergence is silent — each path
looks correct in isolation; only a cross-read reveals that one path skips a guard.

**Standing rule (already stated in `internal/ettlemesh/mesh.go`'s package doc):** any
logic two callers must agree on lives in **one** place both import, never two parallel
derivations. `ettlemesh` exists in part to kill a dual path — the detector logic was
once derived in two places and had already diverged (one had a team-wide pass and
confidence, the other didn't). The exported single-source pieces (`SamePerson`,
`SameKnot`, the `Kind*` constants, the FIRM threshold, `confFromWord`) are all there
for this reason.

**Signature to grep for in ettle:** two functions that each transform or gate an
`Atom` / `Knot` on its way across a boundary (distill, infer, reconcile, vote,
crux-stance) where neither delegates to a shared helper — especially any *redaction*,
*cap*, *confidence*, or *identity* step applied in one producer but not its sibling.

This file is the running ledger. Update it whenever a dual path is found or fixed.

---

## Fixed

1. **Atom boundary seal — `Distill` scrubbed, `InferImplicit` didn't (2026-06-17).**
   Both methods produce `Atom`s that cross the privacy boundary, so both must apply
   the same seal: cap to the structural length limits, run the secret-shape scanner
   (`scrubAtomFields`), then run the per-person privacy override
   (`scrubAtomUserPhrases`). `Distill` did all three; `InferImplicit` applied **only**
   the privacy override and skipped the secret scanner entirely. A token or
   connection-string password the model folded into an *inferred* assumption (or into
   the clarifying question rendered from a low-confidence inference) therefore crossed
   unredacted — the "certain" structural layer had a silent hole in one of its two
   producing paths. Found by the adversarial review panel; two independent reviewers
   (privacy + correctness) converged on it.
   **Authority:** `sealAtom(from, rawSubject, rawContent, private)` in `mesh.go` is now
   the sole boundary chokepoint; `Distill` and `InferImplicit` both call it and neither
   re-derives the cap/scrub steps, so the secret scanner cannot be present on one path
   and absent on the other. Regression guard: `TestInferImplicitScrubsSecrets` (a token
   in a high-confidence inference and a DSN in a low-confidence one — neither survives,
   in the atom or the question) and `TestSealAtom`.

2. **Connection-string redaction — `scrub.go` regex divergence from real DSN shapes
   (2026-06-17).** Not a two-function dual path but a derivation that had drifted from
   the *space of inputs* it claimed to cover: the credentials regex required a non-empty
   username (`redis://:pass@host` leaked the password whole) and stopped the password
   capture at the first `@` (a `p@ssw0rd` leaked its tail). Unified on one regex
   (`(://[^/\s:@]*:)[^/\s]+(@)`) that handles credentials-only URLs and `@`-in-password,
   with port-only URLs (no `@`) still passing through. Guards:
   `TestScrubSecretConnStringCredsOnly`, `…AtInPassword`, `…PortURLNotRedacted`.

---

## Verified — not diverging

These are invariants two or more callers depend on; each already routes through one
shared function, so they are listed here so they are not re-flagged:

- **Person identity** — `SamePerson` (trim + case-insensitive) is the sole matcher;
  voting, self-dedup, party-confidence, and `--me` filtering all call it.
- **Knot identity** — `SameKnot` (shared party + Jaccard over subject+explanation) is
  the sole "same coordination problem?" test; both multi-sample voting and
  self-assumption dedup use it.
- **FIRM threshold** — `Knot.Firm()` (`Confidence >= 0.5`) is the only firm/soft split;
  `surface` and the docs reference it rather than re-stating the number in code. (A
  doc that *restated* it as prose had drifted — `EXAMPLE_RUN.md` showed a 0.5 knot as
  soft; corrected to match the code, 2026-06-17.)
- **Confidence word→float** — `confFromWord` is the single table (it was previously two
  incompatible ones, which let an inferred atom's knot get stamped 0.9).
- **Knot kinds** — the `Kind*` constants feed both the runtime allow-lists and the tool
  schema enums, built from the same consts so the two can't drift.

## Note for tooling

The high-signal heuristic: flag any two functions in `internal/ettlemesh` (or a caller
in `cmd/ettle` / `internal/crux`) that each read an `Atom`'s `Subject`/`Content` or a
`Knot`'s `Confidence`/`Parties` to gate, redact, or score it on its way across a
boundary, where neither delegates to a shared `ettlemesh` helper. Each such pair is a
dual-path candidate — the `Distill`/`InferImplicit` seal was exactly this shape.
