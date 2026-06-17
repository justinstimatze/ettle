# Security

## Reporting

Report vulnerabilities privately to **justin@justinstimatze.com**. Please don't
open a public issue for a security problem until it's been addressed.

## Threat model (design-stage PoC)

ettle is pre-production. Two surfaces matter most:

- **The transport carries sensitive coordination state.** Typed atoms (and, for
  contested knots, the crux) cross between machines. The atom bus (NATS) is
  reached over TLS with credentials; gemot is reached over TLS with a per-agent
  bearer token, and the client refuses to connect off-localhost without one
  (localhost-tokenless is an explicit `--insecure-local` opt-in for dev only).
  Deploy gemot (≥ v0.13.1) with `GEMOT_REQUIRE_AUTH=1` on a private network
  (WireGuard / Tailscale / VPC) behind a TLS-terminating proxy (Caddy/nginx),
  and mint one key per agent on the gemot host (pre-minted at deploy, not
  self-issued) with `gemot admin create-api-key --email agent-X --credits N`.
  See gemot's `docs/private-deployment.md` and
  [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

- **The privacy boundary has two layers — one certain, one judgment.** "Only
  typed atoms cross", and an atom's content is free-form text the model writes from
  the raw note, so treat the atom contents — not the raw note — as the privacy
  surface. Two layers guard that surface, and it is worth being precise about which
  is which:
  - **Structural (deterministic, certain for what it covers).** A post-distill
    scanner (`internal/ettlemesh/scrub.go`) redacts anything *shaped* like a
    secret before the atom crosses — known token prefixes (`ghp_`, `sk-ant-`,
    `xoxb-`, `AKIA`, …), connection strings with inline credentials, PEM
    private-key blocks, high-entropy blobs — regardless of what the model chose to
    emit. It redacts the span (coordination survives) and is loud on stderr, never
    silent. This is a guarantee *for the structures it recognizes*, and only those.
    It runs on **every** atom that crosses — both stated (`Distill`) and inferred
    (`InferImplicit`) — through one shared chokepoint (`sealAtom`), so the two
    atom-producing paths cannot diverge on what gets scanned.
  - **Semantic (model judgment, measured but not proven).** The `Distill` prompt
    carries a contextual-integrity rule: a fact can be both coordination-relevant
    and private, so emit the *consequence* of a change (availability, timeline,
    transfer urgency) but not its private *cause* (health, attrition, family,
    finances, morale, opinions about colleagues), and a personal fact merely
    appearing in a private note is not consent to broadcast it. This is the layer
    that catches leaks with no fixed structure. It is *judgment, not verified
    redaction*: the leak eval (`ettle eval --leak`) measures it at 0% on a
    synthetic corpus, but a measured rate on synthetic cases is evidence, not a
    proof, and the rate is only as good as the corpus.

  Both layers are bounded by the structural caps (subject ≤ 80 chars, content ≤
  ~220 chars, single clause) and inspectable via `ettle standup --show-atoms`,
  which prints exactly what would cross before anything surfaces. A **per-person
  override** lets someone mark phrases as never-share: a note declares
  `private: relocating to Lisbon, comp adjustment` in its frontmatter, and those
  phrases feed *both* boundary layers — a suppress-list in the `Distill` prompt
  (the semantic ask) and a deterministic case-insensitive redaction in
  `internal/ettlemesh/scrub.go` (the structural backstop, loud on stderr when it
  fires). Same defense-in-depth as the secret scanner under the cause-vs-consequence
  rule, but the patterns come from the user. The regression guard is a leak case
  (`testdata/leak/private-override.json`) whose marked phrases must not cross.

- **The hard, unsolved limit is longitudinal, not per-atom.** Every guarantee
  above is *per-atom*: does this one atom, in isolation, leak a planted secret?
  The leak eval measures exactly that. But the real privacy property is whether a
  teammate's accumulated model of a person — the L2 boundary built up over *N*
  rounds of individually-clean atoms — lets them *reconstruct* a fact no single
  atom ever stated. "Out Tuesday", then "pairing Wednesday to hand off the auth
  service", then "won't pick up the Q3 roadmap" each pass the per-atom check and
  together reconstruct an attrition the boundary was supposed to hold. This is the
  genuinely unsolved property; it is **named, not defended**. Building a
  longitudinal-inference defense is deliberately unbuilt (it lives with the
  unbuilt calibration loop — see [docs/CONCEPT.md](docs/CONCEPT.md) status), and
  the metric that would measure it is a stub in
  [docs/BENCHMARKS.md](docs/BENCHMARKS.md), not a number we can quote.

- **Participant notes are untrusted input to an LLM, including cross-principal.**
  A note is distilled by a model into atoms that *other people's* agents and gemot
  then act on. The system prompts treat notes as data and atom *authorship* is
  stamped in code (not by the model) — but prompt injection is a real, open
  surface, and the dangerous case is multi-author: Alice's note can be engineered
  to shape what Bob's surfaced output (or a gemot deliberation topic) says. The
  multiplayer premise *is* the multi-principal case, so "don't run over notes you
  don't trust" is a weak defense — until atom shape is structurally constrained and
  outputs aren't auto-acted-upon, safety rests on a human reading what surfaces.

- **The atom bus is a shared stream, not per-recipient confidential.** Any team
  credential that can publish can also subscribe and replay every teammate's atoms
  (NATS JetStream retains them). The `--me` filter is presentation, not access
  control. Per-recipient confidentiality would need per-subject ACLs or
  per-recipient encryption — neither exists yet.

## Secrets

`ANTHROPIC_API_KEY` and any gemot/NATS credentials live in a local `.env` /
creds file, never committed (`.env`, `*.creds` are gitignored). Nothing logs or
echoes a key or token.
