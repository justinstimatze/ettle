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

- **Participant notes are untrusted input to an LLM.** A note is distilled by a
  model into atoms that other people's agents and gemot then act on. The system
  prompts treat notes as data, not instructions, and atom authorship is stamped
  in code (not by the model) — but prompt injection is an open surface at this
  stage. Don't run ettle over notes from parties you don't trust.

## Secrets

`ANTHROPIC_API_KEY` and any gemot/NATS credentials live in a local `.env` /
creds file, never committed (`.env`, `*.creds` are gitignored). Nothing logs or
echoes a key or token.
