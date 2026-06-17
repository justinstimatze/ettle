# ettle

Read **HANDOFF.md** first — it is the spine (premise, status, design invariants, critical path, next action). Then `docs/` for the full reasoning and citations.

Status: design stage with a runnable PoC. `cmd/ettle` distills each person's notes into typed atoms, reconciles them into coordination knots, and surfaces only what's relevant to each human — `go run ./cmd/ettle standup --me alice testdata/standup/*.md`. The engine is `internal/ettlemesh`; transport (`internal/transport`) and crux deliberation (`internal/crux`, gemot) sit behind seams. See HANDOFF.md for what's built vs planned and the current next action.

Design invariants are non-negotiable (see HANDOFF.md): calibration before speed; contextual privacy boundary; humans stay the deciders; consent-first / anti-viral adoption; useful at N=1; truncate the metaperspective recursion at depth 2–3.
