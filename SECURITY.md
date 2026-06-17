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

- **Distillation is not a redaction guarantee.** The privacy boundary ("only
  typed atoms cross") is enforced by a model *instruction*, not by code. An atom's
  content is free-form text the model writes from the raw note; a sensitive
  sentence can be distilled into a coordination-relevant atom and published. Treat
  the atom contents — not the raw note — as the privacy surface. Two mitigations
  ship today: `ettle standup --show-atoms` prints exactly what would cross before
  surfacing anything, and atoms are **structurally capped** (subject ≤ 80 chars,
  content ≤ ~220 chars, whitespace collapsed to a single clause) so a verbose or
  injection-coaxed distillation is bounded in how much it can carry. Deeper
  redaction (a second-pass PII/secret filter) remains roadmap.

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
