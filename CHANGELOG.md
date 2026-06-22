# Changelog

## Unreleased

- **Inference-channel measurement (stage 1a-1) — `ettle eval --leak-inference`**
  ([docs/LEGIBILITY.md](docs/LEGIBILITY.md)). The `--leak` harness scans crossed atoms
  for markers the person *wrote*, so it is structurally blind to the inference pass —
  which manufactures *de-novo* claims the person never stated (and `--leak` never even
  runs inference). The new opt-in mode runs `InferImplicit` on a trap corpus (notes
  whose behavioral cues tempt a sensitive conclusion) and scans the **inferred** atoms
  (`Inferred=true` only) for that conclusion's markers (`eval.InferenceLeaks`). Opt-in
  because it adds one inference call per case — the cheap `--leak` path is unchanged.
  **Measured (haiku, `testdata/leak/inference-traps.json`):** ~1/3 traps tripped — from
  an innocuous "documenting my runbooks / pairing Kit on deploy" note the pass
  reproducibly inferred *"the speaker is leaving or transitioning out of their current
  role"* (conf 0.6), a claim the note never made; **0/6 inferred atoms were demoted to
  questions** — they cross *asserted* at conf 0.4–0.6. (Rate is noisy, n=1/case and
  stochastic; the qualitative finding — the inference channel fabricates sensitive
  de-novo claims and asserts them — is the result, and it earns the enforcement step
  0b.) A methodology note caught in review: the liberal substring matcher false-tripped
  on the 3-letter marker `ill` ⊂ `will`; the trap corpus now avoids collision-prone
  short markers. Tested deterministically (`TestInferenceLeaks`: a sensitive inferred
  atom trips, an operative-only one doesn't, a STATED marker is the `--leak` channel and
  is ignored here).
- **Read-side mirror (stage 1b) — `ettle mirror --me <name>`** turns the one-way
  mirror around ([docs/LEGIBILITY.md](docs/LEGIBILITY.md)). L2 — the directed model of
  *you* that drives how you're treated — was, per ADOPTION.md, "a one-way mirror at
  exactly the layer that drives behavior." The new command shows a person what the
  team's directed models (L2) currently believe **about them**, flagging the beliefs
  that have gone **stale** (you've drifted from what teammates still hold) surprise-
  first. It reuses `drift`'s exact pipeline — the shared `buildMesh`/`loadAndDetect`
  were extracted so the two commands can't drift apart — and renders the subject-
  centric view: the union of every teammate's beliefs about you, deduped on the
  engine's slot identity (new exported `ettlemesh.Canonical`), staleness from
  `StaleBeliefs`. **Attribution is coarsened by default** (the belief, not which
  teammate holds it — naming a believer surfaces *their* private model, a flow that
  touches them); `--by-observer` opts into attribution. Read-only, no correction
  propagation yet (that's stage 2); no model call beyond drift's distill. Tested
  deterministically (`TestMirror`: beliefs shown, drift flagged stale, coarsen-by-
  default vs `--by-observer`). Also folded `printKnot`/`printAsk`'s duplicated
  vote-suffix into one `voteSuffix` (calque dual-path, score 0.44).
- **Label capture (stage 0c-2) — `ettle_respond` records the human verdict**
  (`internal/mcpserver`; [docs/LEGIBILITY.md](docs/LEGIBILITY.md)). A new MCP tool lets
  a person's agent answer a cross-person knot from `ettle_horizon` — `real` /
  `not_real` / `handled` — keyed by the knot's wording-independent `key` (now on every
  `knotView`). Each verdict is appended as a `Label` (`{key, verdict, by, note, ts}`)
  to a local JSONL (`ETTLE_LABELS_PATH`, default `ettle-labels.jsonl`, gitignored).
  This is the **active-learning label stream** stage 2's calibration loop will consume
  — written now so the data accrues before the loop exists (a detector flag-rate is
  only calibratable against confirmations from people who saw the work). It records
  **only**: no binding, no horizon mutation — humans stay the deciders. Label sink is
  an interface (file by default; tests inject memory). Tested:
  `TestRespondCapturesLabel` (capture + verdict/field validation, no-capture on
  reject), `TestKnotKeyStableAndCrossCallMatch` (order/case-stable key).
- **Interrogative register (stage 0c) — cross-person knots are posed as questions,
  not asserted** ([docs/LEGIBILITY.md](docs/LEGIBILITY.md)). The detector has no
  ground truth for a cross-person conflict, and recurrence is test-retest *stability*,
  not validity — so it has no standing to assert one. The CLI `surface` now routes
  **self knots** (a person's own drift, which they can verify) to an asserted "worth a
  look" lane and **every cross-person knot** to a "worth checking together (a question,
  not a claim)" lane — "[possible collision] … Real, or already handled?" — ordered
  firm-first; contested ones still pre-stage their either/or. The MCP `horizon` marks
  each cross-person `knotView` `question:true` so agent consumers present it as a
  question too. Grounded in mixed-initiative design (act when confident+positive-sum,
  ask otherwise) and trust calibration (communicate true uncertainty, don't overclaim).
  The Firm-and-bindable act-lane for cross-person knots opens later, *earned per kind*
  against the calibration label (stage 2) — so this register is also the active-learning
  query front-end that loop will need. Deterministically tested (`TestSurfaceActAskRouting`).
- **Legible abstention (stage 0a) — the coupling check stops dropping silently**
  (`GroundKnots` now returns `(kept, suppressed)`; [docs/LEGIBILITY.md](docs/LEGIBILITY.md)).
  A clear horizon that silently hid a suppressed call trains the human to stop
  watching — the exact failure a structured adversarial pressure-test (legibility /
  extraction-skepticism lenses) flagged. Knots the coupling check judges
  *not a real conflict* are now surfaced
  **off the agenda**, in a "held back — shown in case that's wrong" section (CLI
  `surface`) / a `held_back` field + summary tail (MCP `horizon`), filtered to `me`.
  Coupling-check kills are *listed* (high-recurrence, a human might overrule them);
  the abstention-floor drops (≤1/5 samples, noise by design) surface as a single
  quiet **aggregate count** ("+N below the confidence floor, not shown") so the
  notice doesn't get trained into the ignore pile — `ReconcileVoted`/`voteKnots` now
  return that count alongside the kept knots. Deterministically tested
  (`TestSurfaceHeldBack` captures both the listed section and the floor line;
  `TestDropFloor` asserts the count; `applyGroundingVerdicts` returns the suppressed
  set). First increment of the
  legibility program drafted in `docs/LEGIBILITY.md` (the response to the panel:
  turn the model's output from a private assertion into a legible, contestable
  signal). No detection-accuracy change — the eval still scores only kept knots.
- **Cross-person coupling check — generalizes the collision direction-check to
  duplication + teamwide-divergence** (`GroundKnots`/`groundableKnots` in
  `internal/ettlemesh/ground.go`). The collision pass (below) closed the *collision*
  vector, but a `--samples 5` re-measure found the **same root error** — two people
  bridged on a shared topic word while working in *independent scopes* — surviving
  voting under two **other** kinds: a fake `[duplication] alice,cleo` (a user-lookup
  cache and a Grafana metrics dashboard read as redundant work) and a fake
  `[teamwide-divergence] alice,bob,cleo` (cleo's unscheduled internal maintenance
  swept into a product launch deadline), together **0.40 FIRM cross-boundary
  knots/run** on `superposition-userservice-vs-infra`. The pass now asks a
  kind-appropriate **coupling** question of each cross-person collision/duplication/
  teamwide knot: collision → do both *edit the same artifact*; duplication → are both
  *building the same deliverable twice*; teamwide → does the named assumption actually
  *govern every party* and do they hold it *differently*. decision-rights is excluded
  (a who-decides truth condition the coupling question would misjudge). Measured
  (haiku, `--samples 5`): userservice-vs-infra FIRM cross-boundary **0.40 → 0.00**
  (CI 0.00–0.00, both fabs gone); **real-knot recall held 1.00 across kinds** — real
  teamwide (calendar K1), real duplication (duplicate-util K1), real collision
  (schema-collision K1) all kept at precision 1.00; labeled fakes duplicate-util D1
  (CI test-retry vs HTTP backoff) and shared-deadline-null D1 (agreed Q3 freeze)
  dropped. To keep each kind's instruction undiluted, the pass makes **one focused
  call per kind present** (collision / duplication / teamwide) rather than one merged
  3-kind prompt — cost is +1 model call per additional distinct kind. The same change
  numbers each prompt's knots by their **full-slice index**, fixing a latent
  verdict-mismap that silently failed to drop a fabrication whenever a
  self/decision-rights knot preceded a groundable one (fail-open kept it).
  Re-smoke-tested after the split: userservice-vs-infra FIRM still **0.00**, real
  teamwide (calendar K1) and real duplication (duplicate-util K1) recall held **1.00**.
  **Caveat:** the pass is a *single probabilistic judge call*, not a deterministic
  gate — it lowers fabrication probability but a borderline fab still flickers firm
  run-to-run (frontend-vs-data's mabel/opal collision, calendar's "review" D1); n=5
  can't claim a stable per-corpus rate, and that flicker (finding #5) is accepted for
  now. Default ON across `standup`, `eval`, and the **MCP horizon**; disable with
  `--no-ground`.
- **Collision direction-check — closes the residual fabrication the floor couldn't
  reach, now ON by default** (`GroundKnots` in `internal/ettlemesh/ground.go`). The
  abstention floor (below) kills the flickery fabrication tail, but a *high*-recurrence
  misread survives it: a producer/consumer pipeline read as a collision because both
  people name the same topic word (mabel "consuming the metrics API" vs opal "writing
  warehouse tables the metrics service queries" — both say "metrics"). This is
  lexically inseparable from a real collision (bex+cyrus both say "orders"/"status"),
  so no token filter can catch it — the discriminator is the *relationship*. The
  reframed pass asks one bounded question of each cross-person COLLISION: do both
  parties **edit the same artifact** (real), or does one **produce what the other
  consumes** / do they touch **different artifacts sharing a topic word** (fabricated)?
  Measured (haiku, `--samples 5`): FIRM cross-boundary fabrication on
  superposition-frontend-vs-data **0.50 → 0.00**, the "auth service" collision trap
  cleared, **real-collision recall held 1.00 on every clear corpus** (schema, scale,
  standup GetUser), pooled FP 6 → 3. This is the same scaffold that shipped *off* in
  June under a *validity* framing ("do they share a referent?" — both do, so it
  failed); the *direction* framing is answerable from the atoms and works. Now default
  ON across `standup`, `eval`, and the **MCP horizon** (`ettle_horizon`); disable with
  `--no-ground`. Scope: collisions only — duplication/teamwide/decision-rights have
  different truth conditions and pass through.
- **Abstention gate — the recurrence noise floor** (`dropFloorFraction` in
  `internal/ettlemesh/mesh.go`, applied in `voteKnots`) closes the bulk of the
  cross-group fabrication the robustness battery surfaced. A voted knot recurring
  below the floor (0.25 of samples — strictly under the lowest per-kind firm bar, so
  it can never drop a knot the firm bar would assert) is dropped entirely: not
  asserted, not asked. It catches the fabrication *tail* (separability: fabricated
  cross-group knots recur ≤~0.17 of runs), which is most fabrications, at **zero
  clear-knot recall cost**. Measured (haiku, `--samples 5`): on the worst corpus
  `superposition-frontend-vs-data` FIRM (asserted) cross-boundary fabrication fell
  **2.60 → 0.50 knots/run** (~80%); on `superposition-userservice-vs-infra`,
  **0.40 → 0.00**. Real-knot recall held 1.00 on all eight clear-knot corpora;
  pooled real-knot false positives halved (4 → 2). The only recall casualty is
  auth-migration K2, a flickery `decision-rights` knot already lost to detection
  flicker pre-floor — an accepted miss under the **"lighter agenda, not no meeting"**
  framing (precision is the goal; missing a flickery knot just leaves it on the human
  agenda). Residual: high-recurrence *polysemy* misreads (e.g. `mabel↔opal` both on
  "analytics") survive — the floor structurally can't reach a 0.5-recurrence knot;
  that needs a separate structural fix on the collision/teamwide pass (out of scope).
- **`eval --superposition` now measures what ships** — runs *voted* at `--samples`
  (not single-shot) and splits the headline into **FIRM** cross-boundary (asserted —
  the stop-ship number, target 0) vs **all** (firm+soft pooled). The old single-shot,
  firm+soft-pooled headline overstated fabrication by counting questions as claims.
- **DEPLOY.md** — documents the `file://` shared-folder transport as a deployment
  tier (zero-infra multiplayer, no broker), between the single-machine default and
  the NATS bus; tiers renumbered accordingly.

## v0.1.0 — 2026-06-18

First runnable cut of the multiplayer coordination PoC.

- **MCP server** (`ettle mcp`, `internal/mcpserver`) — serves the coordination
  engine over the Model Context Protocol so any MCP client (Claude Code, Cursor)
  drives it directly: `ettle_emit` distills a person's notes server-side through
  the privacy boundary (stores only atoms, drops the raw notes), `ettle_horizon`
  reconciles the team's atoms into firm/soft knots filtered to `me`, and
  `ettle_self_check` runs the N=1 self pass with no team. MCP is the consent-clean
  surface a meeting bot is not (each agent emits only its own person; nothing
  harvested — see ADOPTION.md). Depends on a narrow `reconciler` interface so the
  handlers are tested key-free, including a full in-memory MCP round-trip.
- **`file://` directory transport** (`internal/transport/dir.go`) — zero-infra
  multiplayer over a folder a team already shares (Dropbox/Drive/git/Syncthing):
  each participant writes only its own `<root>/.ettle/<name>.atoms.jsonl`,
  reconcile reads the folder, no broker to run. Replace-current storage (trivial
  clean-exit, no longitudinal pile-up); atomic temp-rename writes; lenient parse;
  `.ettle/` namespacing + conflict-copy skip; filename-authoritative identity; and
  a Coverage/staleness roster so a partially-synced horizon is never read as a bare
  "all clear". NATS stays a scheme-selected option (`file://` | `nats://` |
  inproc), the `file://` parse single-sourced so it can't drift across build tags.
- **Per-kind firm bar** — recurrence-voting ranks knots firm (assert) vs soft
  (ask), and the bar is now per-kind: a genuinely flickery `decision-rights` knot
  asserts at a lower recurrence (0.3) than the default (0.5), staying clear of the
  fabrication floor. The hand-set seed of the Phase-3 calibration loop. (The
  separability diagnostic established recurrence-frequency, not model confidence,
  is what discriminates real knots from fabricated ones.)
- **L2 — the directed-model layer — is built (structural form).** The pipeline used
  to skip straight from distill (L1) to a flat-pool reconcile (L3); the documented
  centerpiece between them, the per-pair directed models, was specced but absent.
  `internal/ettlemesh/directed.go` now implements it: a `DirectedModel` (one
  observer's belief-atoms about one subject, asymmetric, N×(N−1) of them), the
  surprise-gated emit rule (`EmitDelta` — a session re-emits only the atoms that
  changed against what each teammate already believes), the L2-vs-L1 staleness diff
  (`StaleBeliefs`), and a `MeshState` that carries the models across rounds. All
  deterministic (no extra model call, O(1) per the no-machine-speed-loop invariant)
  and unit-tested without an API key. New `ettle drift <prev-dir> <curr-dir>`
  demonstrates it over two rounds on [`testdata/drift/`](testdata/drift): round two
  re-sends a changed teammate's deltas to exactly the people whose model of them went
  stale, reuses unchanged notes without re-distilling, and (with `--me`) shows whose
  model the caller now holds stale. "Surprise" — defined in CONCEPT.md as the
  L2-vs-L1 divergence — now has a *computed* value, not just a type signature.
  Pressure-tested with a deterministic adversarial test pass, a live adversarial
  fixture, and an adversarial review panel: fixed a same-slot collision (silent data
  loss + phantom re-emission, now collapsed via `canonical`), unified the L2-internal
  identity relation (the self-skip and the store key both use `normPerson`, so an
  exotic Unicode fold can't skip a real pair), normalized the reuse gate on whitespace
  (a reflow no longer forces a re-distill), and added N=1 / absent-person /
  new-arrival handling to `drift`. **Known structural limit, documented not hidden:**
  the slot key is an exact `(type, subject)` match over *stochastic* distiller text,
  so a reworded subject on a still-held belief reads as drop+new — savings hold
  per-person but degrade per-belief on a re-distill, and the surfaced "stale" line is
  hedged accordingly. Still unbuilt there: wording-independent slot identity (the fix
  for that limit), the *semantic* enrichment (inferring a teammate's unstated
  assumptions), and the calibration loop; docs flipped accordingly.
- **Adversarial-review hardening** — an adversarial expert panel pressure-tested the
  whole repo (find → independent refutation → synthesis); the surviving findings drove
  this pass. The load-bearing fix closes a **dual path** in the privacy boundary: atoms
  cross via two producers (`Distill` for stated atoms, `InferImplicit` for inferred
  ones), and the structural secret-scanner was wired into `Distill` only — so a token
  or DSN folded into an *inferred* assumption (or the question rendered from one)
  crossed unredacted. Both producers now funnel through one chokepoint (`sealAtom`), so
  the secret scanner and the per-person override cannot be present on one path and
  absent on the other.
  Also: the connection-string redactor now catches credentials-only URLs
  (`redis://:pass@host`) and `@`-in-password DSNs (both previously leaked); `clip` no
  longer splits a multibyte rune across the boundary; `voteKnots` confidence no longer
  double-counts a run that names one divergence in both the pairwise and team-wide
  pass; a bare `name:` header no longer blanks a participant into the `--me ""`
  full-team sentinel; and the gemot poll loop honors parent-context cancellation
  instead of spinning to its local deadline. Doc-honesty corrections from the same
  panel: CONCEPT/README no longer state the semantic layer as "enforced" or "0% leak"
  as a settled property (it is model judgment, measured on a synthetic corpus);
  `EXAMPLE_RUN` no longer shows a 0.5 knot as soft (the code routes ≥0.5 to firm); the
  README banner disambiguates the unbuilt N=1 *safety wedge* from the working N=1
  self-assumption pass; and BENCHMARKS states the dupbug A/B's structural ceiling
  (single-shot 8/8 leaves the McNemar "voting helps" cell pinned at zero) and a
  Wilson CI on the 8/8 recall. All fixes are unit-tested (no API key); `go test ./...`
  and the `-tags nats` build stay green.

- **Demo** — a fully-synthetic four-person team (`testdata/northwind/`, four
  Claude Code session transcripts) shown in the README as a scenario diagram plus
  a real-run transcript: the pre-meeting collision catch, bind-vs-surface (simple
  collisions FYI'd, the freeze-date divergence routed to a pre-staged crux), the
  N=1 self-assumption, and `--show-atoms` for the boundary.

- **Transport hardening** — the dev-only `--insecure-local` (plaintext/tokenless)
  gate now **resolves** the host and requires every address to be loopback
  (`internal/loopback`), instead of string-matching the hostname — a non-loopback
  name dressed up as local is rejected. The gemot client refuses to send a bearer
  token over plaintext `http://` off-box (a token in the clear is a leak), and
  after connecting with a token it does a best-effort check that the session
  isn't gemot's anonymous sandbox (a bad/expired token that silently degraded) —
  a defense-in-depth signal behind the hard token+TLS gate, not a guarantee. Honest
  limit documented: loopback resolution can't see a deliberate off-box port-forward
  from a loopback bind. README now states the Go ≥ 1.25 requirement; the local
  stack docs (`deploy/`) and the gemot client doc no longer contradict on demo
  mode vs Postgres.

- **Boundary transparency + structural caps** — `ettle standup --show-atoms`
  prints exactly the typed atoms that cross (the privacy surface) before
  surfacing knots; atoms are now structurally capped (subject/content length,
  whitespace collapsed to one clause) so the boundary is partly enforced, not
  only trusted. Per-person distillation runs in parallel (latency is the "no
  meeting" competitor), and the Anthropic client retries 429/5xx (SDK-native,
  `WithMaxRetries(4)`) so a transient rate-limit doesn't abort a whole run.

- **Cause-vs-consequence boundary rule** — the `Distill` system prompt now encodes
  the contextual-integrity transmission principle the leak eval surfaced: a fact can
  be both coordination-relevant and private, so when a note gives a REASON for a
  change in availability / priority / commitment, the distiller emits the change and
  its coordination impact but treats the personal cause (health, attrition, family,
  finances, morale, opinions about colleagues) as private by default — and a
  personal fact merely appearing in a private note is not consent to broadcast it.
  Found empirically: the leak eval's one failure (an attrition reason fused to a
  legitimate knowledge-transfer ask) was **model-invariant** (haiku = sonnet, ~12%),
  i.e. a boundary-policy gap, not a model-capability gap; the rule closes it to **0%
  leak with utility unchanged at 100%** on both tiers (a single live run each, on a
  small synthetic corpus — evidence, not a reproducible property). (Still model judgment, not
  verified redaction — see SECURITY.md; the deterministic secret-scanner below is
  the structural backstop under it.)

- **Structural secret-scanner** (`internal/ettlemesh/scrub.go`) — the deterministic
  half of the privacy boundary, under the semantic prompt rule above. A post-distill
  pass redacts anything *shaped* like a secret before the atom crosses — known token
  prefixes (`ghp_`, `sk-ant-`, `xoxb-`, `AKIA`, …), connection strings with inline
  credentials, PEM private-key blocks, high-entropy blobs — regardless of what the
  model chose to emit. It redacts the span (coordination survives, the atom is never
  dropped) and is loud on stderr, never silent. `scrubSecret` is pure and unit-tested
  (no API key); the high-entropy catch-all is gated on a mixed alphabet so it won't
  nuke long words or pure-hex commit SHAs. The boundary is now honestly two-layer —
  structural (certain, for secret-shaped content) and semantic (judgment, leak-eval
  guarded) — and SECURITY.md/CONCEPT/BENCHMARKS now name the genuinely unsolved
  property both layers miss: *longitudinal* reconstruction across many
  individually-clean atoms, which the per-atom leak rate cannot see.

- **Per-person privacy override** — a note can declare `private: <phrases>` in its
  frontmatter (e.g. `private: relocating to Lisbon, comp adjustment`), and those
  phrases feed *both* boundary layers through the same per-person path `role`
  already rides: a suppress-list in the `Distill`/`InferImplicit` prompts (the
  semantic ask) and a deterministic case-insensitive redaction in
  `scrub.go` (`scrubUserPhrases`, the structural backstop — loud on stderr, span
  redacted, atom never dropped). This turns the "documented seam, not built" line
  in SECURITY.md into a built feature. Opt-in and inert when absent (no `private:`
  → no-op). Structural half is pure and unit-tested (no API key); the regression
  guard is a leak case (`testdata/leak/private-override.json`) whose marked
  phrases must not cross — live leak run stays 0%/100% with it included.

- **Bounded semantic re-roll on tool-call failure** (`callTool`) — a model that
  returns a response carrying no usable tool call (no `tool_use` block, or a
  `tool_use` whose input doesn't match the schema) is now re-rolled up to 3 times
  before failing, instead of aborting the whole run on the first garble. This is
  the stochastic-failure twin of the SDK's transport retry: transport/context
  errors stay terminal (already SDK-retried, not multiplied), only the re-rollable
  semantic miss is re-sampled, and after the bounded budget the loud-fail error
  still surfaces (never a silent "all clear"). Makes a cheaper model usable when it
  garbles the schema intermittently — observed concretely as haiku returning the
  `infer_assumptions` inferences field as a string rather than an array, which used
  to abort a whole `--ab` run. Unit-tested with a sequenced fake messager
  (garble-then-recover, fail-after-budget, transport-not-retried).

- **First real-data eval corpus** (`testdata/dupbug/`) — the duplication knot,
  validated against real bug-tracker data instead of synthetic fixtures.
  Confirmed `RESOLVED DUPLICATE` pairs pulled from the **public Mozilla Bugzilla
  REST API** are anonymized and reworded into standup-style notes (raw responses
  stay in a gitignored cache; only the derived notes are committed — provenance
  in `PROVENANCE.md`). **Eight real duplicate pairs across three corpora**, many
  of them the hard *root-cause-vs-symptom* case where the two reporters describe
  the same bug in different words (a fontconfig crash signature vs "googling a
  font crashes the tab"; a GTK default-action regression vs "Enter does nothing")
  — exactly what a verbatim matcher misses — plus a surface-similar distractor (a
  cosmetic print-dialog bug that must not fuse into the print-broken pair).
  Single-shot on sonnet recovers **all 8** duplications and keeps the distractor
  out of the firm duplication. The A/B (single-shot vs 3-sample voting) is
  reported honestly as **underpowered**: across 8 labels the two conditions
  disagree on only one (voted 7/8, single 8/8), so the McNemar discordance is too
  small to test — *not* a sample-count problem but a sign the conditions agree on
  clear-cut duplicates, where voting's noise-damping has nothing to fix. (An
  earlier single-corpus run where voting dropped a real duplication did not
  replicate at scale — it was one stochastic draw, not an effect.) Honest framing
  kept loud: these are artifacts, not reasoning-in-progress — a retrospective
  detector test, not thesis validation.

- **Privacy-boundary leak eval** (`ettle eval --leak`, `internal/eval/leak.go`) —
  the orthogonal harness: it measures whether the typed-atom boundary *leaks*,
  rather than whether the detector finds the right knots. Synthetic notes
  (`testdata/leak/*.json`) carry planted private facts that must NOT cross — a comp
  number, a plaintext credential, a medical reason, a private opinion of a named
  teammate — each with markers whose appearance in a crossed atom counts as a leak;
  the run distills each note and reports the **leak rate**. A per-case **must-cross**
  check guards the trivial defense (emit nothing → zero leaks, zero utility): it
  flags over-redaction as a failure instead of success. The matcher is deliberately
  **liberal** (substring) so it over-counts a leak before it under-counts one — the
  safe bias for a privacy claim. Scoring is pure and unit-tested (no API key); only
  the live `Distill` spends budget. Turns the privacy boundary from an assertion
  (structural caps) into a measured number.

- **Calibration harness** (`internal/eval`, `ettle eval`) — scores the detector's
  precision/recall against a **committed synthetic corpus** (`testdata/eval/*.json`)
  so the accuracy claim is inspectable, not gitignored. The corpus now carries
  **plausible-but-wrong distractors** (`Real=false` — single-person open questions
  like "which payment provider?" that a miscalibrated detector might wrongly assert
  as a cross-person knot); a FIRM knot that matches one is reported as a **named
  trap the detector fell for**, not just a bare false positive. `--ab` runs
  single-shot vs multi-sample voting with a McNemar test that is now **pooled
  across corpora** — per-corpus the discordant N is always too small to reach the
  reliability gate, so a per-corpus test could never find significance regardless
  of the effect. Fixed the voting clustering it exercises: `SameKnot` uses a
  Jaccard threshold (was: any one shared keyword) and `voteKnots` uses
  order-invariant union-find (was: order-dependent first-match).

- **L1 live-session capture** (`internal/capture`, `ettle capture`) — distills a
  person's real Claude Code session transcript (their stated intent from prompts
  + the work they committed via Edit/Write/Bash; exploration like Read/Grep and
  subagent sidechains are skipped) into the same digest a hand-written note would
  be. `ettle standup session.jsonl` runs the whole pipeline on **live
  reasoning-in-progress**, not after-the-fact artifacts — the thesis the design
  rests on. The digest stays local; only the distilled atoms cross. Synthetic
  session fixtures in `testdata/sessions/`.

- **`ettle standup`** — distills each participant's notes into typed atoms,
  reconciles them (pairwise + team-wide + a single-party self pass) into
  coordination knots, and surfaces only what's relevant to each human (`--me`).
  Routes FIRM knots as "worth a look", SOFT (inference-backed) as "worth a
  question".
- **Useful at N=1** — a single-party **self-assumption** pass (`ReconcileSelf`)
  surfaces an assumption a person's own later work has quietly made false; the
  pairwise/team passes are blind to it by construction. Deduped against the
  cross-person knots (shared `SameKnot` matcher) so a team-wide divergence isn't
  also reported privately.
- **Multi-sample voting** (`--samples K`, `ReconcileVoted`) — re-runs the
  reconcile passes K times and keeps only knots recurring across a majority,
  turning the stochastic detector's run-to-run noise into a confidence signal
  (each surviving knot carries `Votes`/`Samples`, kept separate from
  Confidence). Clustering uses the same `SameKnot` matcher, so a knot relabeled
  collision→decision-rights across runs still votes as one. Default `K=1` is the
  original single-run cost.
- **Transport seam** — in-process (default, zero infrastructure) and a NATS
  distributed bus (`-tags nats`, TLS + credentials enforced off localhost).
  - The NATS adapter uses **JetStream** (retained stream): a publish-before-collect
    flow over core pub/sub would race and silently drop a peer's atoms;
    retention removes the race. Covered by an embedded-server integration test
    (in CI) and a live three-process docker run.
- **Crux seam** — contested knots route to a gemot deliberation (TLS + bearer
  token, refuses anonymous off localhost) or an infra-free inline either/or.
  Validated live against gemot 0.13.1: a decision-rights knot produced a scored
  crux + binding compromise. gemot poll default 90s → 180s (`--gemot-timeout`)
  after its multi-round analysis outran the old timeout.
- **Safeguards** — `--me` validated against the roster; collected-vs-published
  participant count asserted (no silent partial "all clear"); resolver errors
  surfaced; output-truncation warning; prompt-injection guard in the prompts.
- **Local stack** — `deploy/docker-compose.yml`: NATS (JetStream) + gemot
  (demo mode) in one `docker compose up`, run ettle against it with
  `--insecure-local`.
- **Scaling design** — `docs/SCALING.md`: the anti-runaway firewalls for the
  future continuous loop (L3 emits knots not atoms; surprise-gated emit; O(1)
  shared reconcile; per-agent budget). The production hook path is gated on them.
- **Project** — MIT LICENSE, SECURITY.md, architecture diagram + example run,
  synthetic fixture, parser/loader + NATS tests, CI (both build configs),
  git-tag-derived `ettle version`.
