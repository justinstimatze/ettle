# ettle

Read **[docs/CONCEPT.md](docs/CONCEPT.md)** first — it is the spine (the premise, the three-layer model, the critical path, and the non-negotiable design invariants). The [README](README.md#status) has the status (what's built vs deliberately unbuilt); the rest of `docs/` has the full reasoning and citations.

Status: design stage with a runnable PoC. `cmd/ettle` distills each person's notes or live session into typed atoms, reconciles them into coordination knots, and surfaces only what's relevant to each human — `go run ./cmd/ettle standup --me alice testdata/standup/*.md`. The engine is `internal/ettlemesh`; transport (`internal/transport`) and crux deliberation (`internal/crux`, gemot) sit behind seams. The detector runs today; the calibration loop, the L2 directed-model mesh, and the continuous live-emit path are deliberately unbuilt (README status; CONCEPT.md).

Design invariants are non-negotiable (see [CONCEPT.md](docs/CONCEPT.md#design-invariants-non-negotiable)): calibration before speed; contextual privacy boundary; humans stay the deciders; friction in the right spots; the coordination commons is governed; no machine-speed feedback loop; consent-first / anti-viral adoption; useful at N=1; truncate the metaperspective recursion at depth 2–3.
