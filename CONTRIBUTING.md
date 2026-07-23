# Contributing to ettle

ettle is an early-stage, design-led PoC, not a product. The most useful
contributions right now are the ones that make its **claims falsifiable** or its
**invariants harder to violate** — not new surface features. Read
[docs/CONCEPT.md](docs/CONCEPT.md) (the spine) and the
[README status](README.md#status) (what's built vs. deliberately unbuilt) before
starting.

## The one hard rule

Every contribution must respect the
[design invariants](docs/CONCEPT.md#design-invariants-non-negotiable). They are
non-negotiable because the most relevant prior attempt at a proxy-agent pool
(Electric Elves) failed precisely by violating them — see
[docs/CALO_LINEAGE.md](docs/CALO_LINEAGE.md). In particular: calibration before
speed, the contextual-privacy boundary, humans stay the deciders, and no
machine-speed feedback loop. A PR that makes ettle faster or more autonomous at
the cost of one of these will be declined no matter how clean the code.

## Where help matters most (ranked by leverage)

1. **The calibration loop (the biggest unbuilt thing).** Today the detector runs
   but nothing measures whether a surfaced tangle was *real to the humans
   involved*. The honest first version: a "did this tangle help?" signal per
   surfaced tangle, per pair, accumulated into a per-relationship trust estimate.
   This is the invariant the whole project rests on; it is also the hardest part.
   See the calibration framing in CONCEPT.md and the necessity-prediction prior
   art in [CALO_LINEAGE.md §5](docs/CALO_LINEAGE.md).

2. **A measured privacy boundary (now a thin first version — make it real).**
   ettle used to only *assert* its typed-atom boundary; `ettle eval --leak`
   (`internal/eval/leak.go`, `testdata/leak/*.json`) now *measures* it: plant
   private facts that must not cross, distill, report the leak rate, with a
   must-cross guard against zero-by-emitting-nothing. But the corpus is tiny
   (a handful of synthetic notes) and the matcher is a liberal substring check.
   The highest-leverage work here: many more cases across the contextual-integrity
   failure modes (ConfAIde / PrivacyChecker taxonomy — see
   [docs/PRIOR_ART.md](docs/PRIOR_ART.md) §2 and [docs/BENCHMARKS.md](docs/BENCHMARKS.md));
   a smarter matcher (the substring rule over-counts on purpose, but a semantic
   check would tighten it); and an A-vs-B over distillation prompts to show a
   boundary change actually moves the number.

3. **Strengthen the existing calibration harness.** `internal/eval` scores the
   detector against a small committed corpus (`testdata/eval/*.json`). It needs:
   more scenarios; more `Real=false` distractors (plausible-but-wrong tangles — see
   the existing ones for the shape); and a confidence-calibration / ECE readout
   that bins firm tangles by confidence and checks whether the rate of real matches
   tracks the stated confidence.

4. **Wire a public dataset as a real corpus.** [docs/BENCHMARKS.md](docs/BENCHMARKS.md)
   catalogs candidates. The tractable first step is duplicate-bug-report pairs as
   a duplication-tangle corpus. The honest caveat (artifacts vs.
   reasoning-in-progress) is documented there; a contribution that names its own
   limits is worth more than one that overclaims.

5. **L2's semantic enrichment (the structural layer is built; this part isn't).**
   The directed per-pair models, the surprise-gated emit rule, and the staleness
   diff now run deterministically (`ettle drift`, `internal/ettlemesh/directed.go`).
   What's unsolved is the *semantic* core — Alice's agent inferring what Bob is
   assuming *beyond his stated atoms* (second-order ToM,
   ground-truth-through-the-boundary), and the cross-author calibration that would
   keep such an inference honest ([docs/N1_WEDGE.md](docs/N1_WEDGE.md)). Design
   discussion (an issue, not a PR) is more useful here than code right now.

6. **Wording-independent slot identity for L2 (a concrete, scoped code task).** L2
   keys a belief on its exact `(type, subject)` string (`beliefKey` in
   `internal/ettlemesh/directed.go`). Subjects are stochastic distiller output, so a
   reworded subject on a still-held belief reads as drop+new — a false "stale" signal
   and lost per-belief savings (demonstrated in [`testdata/drift/adversarial`](testdata/drift/adversarial)).
   The fix is a deterministic slot match that tolerates rewording — most naturally by
   reusing the tangle-identity machinery already in the package (`tokenSet` / `jaccard`,
   the same fuzzy identity `SameTangle` uses) so L2 and L3 share one "same thing?"
   notion rather than deriving two. Wants a small fixture to tune the threshold
   against; deterministic, no model call (keep the O(1) / no-machine-speed invariant).

Smaller, always-welcome: better synthetic fixtures, doc clarity, honest-limit
notes, and bug fixes in the transport/crux seams.

## Mechanics

- Go ≥ 1.25. `go build ./...`, `go test ./...`, and `go vet ./...` must pass; the
  detector path and eval are covered by tests that don't require an API key.
- **Synthetic data only** in committed fixtures — no real transcripts, names, or
  org data. Real/local eval material stays gitignored.
- Keep diffs small and the commit message explaining *why*, matching the existing
  history. Open an issue first for anything touching an invariant or a seam.
- Be honest about what a change does and doesn't verify — "this is a smoke test,
  not a precision measurement" is the register the whole repo is written in.

## Contact

Security and conduct questions: see [SECURITY.md](SECURITY.md). Otherwise open an
issue.
