# Changelog

## Unreleased

- **A project can point at its own Linear workspace, via a named key profile.** A
  Linear member key is scoped to one workspace, and ettle had exactly one key slot per
  machine: `linearBusFor` reads `LINEAR_API_KEY` from the process environment, and the
  only source a hook-launched process has is the single global `~/.config/ettle/env`,
  loaded before any command knows which room it is in. Someone working across two
  workspaces could not point two projects at them. Now `.ettle-room` can carry
  `profile = work`, whose keys live in `~/.config/ettle/env.d/work` and layer over the
  global file — one profile serves however many projects share a workspace. The line
  is a name and not a secret, so it stays as safe to commit as `room`; `ETTLE_PROFILE`
  overrides it for a teammate who names theirs differently. Precedence is explicit
  environment, then profile, then global file. A project with no `profile` line
  behaves exactly as before.
- **Pointing at the wrong workspace is now refused instead of silently duplicating the
  room.** The old failure was the bad kind: a key from another workspace does not
  error, it simply cannot see `ettle-<room>`, so `resolveProject` would **create a
  second project of that name** in the wrong workspace — a real bus with a teammate
  who never sees it. `ettle init` records which workspace a room was found in, and a
  later run holding a different key is refused by name. The check runs only on the
  create branch, so a normal read costs nothing, and it also refuses when the
  workspace cannot be determined at all — "could not check, carry on" would restore
  the exact bug. Escalation inherits it, since it resolves the same project. No
  recorded workspace means no expectation, so every existing install is unaffected.
- **`saveIdentity` no longer erases the recorded workspace.** It marshaled a fresh map
  of its own two fields rather than the record, so in `ettle init` the workspace was
  written and erased nine lines later — leaving the guard permanently inert with the
  build green. Both writers now read the record first and persist the struct.
- **`ettle escalate` reads `LINEAR_TEAM_ID` after the room resolves, not as a flag
  default.** A flag default is evaluated before `linearRoomFor` runs, which is what
  loads the project's profile, so a second-workspace project silently got the global
  team id.
- **`ettle init --profile <name>`** writes the line, reports the active profile and
  its path, reports the workspace it resolved, and fails a profile that is named but
  absent — falling back to the global keys quietly is how a project ends up talking to
  the wrong workspace.
- Docs: a "more than one workspace" section in `docs/LINEAR_SETUP.md` with the guard's
  two honest limits, and a corrected `ANTHROPIC_API_KEY` row — it said "one per room",
  but the hook path needs one per person (`ettle capture` and `ettle horizon` both
  hard-fail without it) and only the key-free `ettle mcp` path needs none.

## v0.5.0 — 2026-08-05

- **Docs stop calling the calibration loop unbuilt, and stop calling it built.** The
  README banner, the Status section, the architecture diagram, CONCEPT.md and
  CONTRIBUTING all said the correction half does not run. Two of its three parts now
  do: verdicts are captured with the recurrence they answered, and `ettle calibrate`
  reads them back. What does not run is anything that acts — no threshold moves, no
  per-pair model corrects, and per-pair trust is still global-per-kind. Part of that
  gap is permanent by invariant and part is genuinely unbuilt, and the docs now say
  which is which rather than collapsing both into "not built yet".
- **`ettle calibrate` now says where the evidence puts a cut point, and refuses to
  when it does not.** It used to report a kind as readable and then say nothing about
  where the bar should go. It sweeps the labelled recurrences and reports the interval
  of cut points that separate `real` from `not_real` best — an interval and not a
  point, because recurrence is votes over a small sample, ties are the normal case,
  and collapsing one to its midpoint would invent precision the rows do not carry.
  The score is Youden's J rather than accuracy, because the log over-represents
  `not_real` by construction and an accuracy-maximising cut drifts toward asserting
  nothing just because most rows are negative.
- **Two ways it declines.** When no cut beats chance it says so instead of naming the
  best-scoring one — there is always a best-scoring cut, and printing it would dress a
  coin flip as a measurement; the finding is that recurrence is not what decides these
  verdicts, which moving the bar cannot fix. And it compares the bar in force to the
  suggestion by the SPLIT each produces, not by whether the number falls inside the
  interval: candidate cuts are the observed recurrences, so a bar below all of them
  can still classify every row identically, and reporting that as a reason to move
  would be the tool inventing work. Both were live bugs caught by running it against
  separable and overlapping fixtures rather than only the unit tests.
- **A verdict typed at the shell now carries the recurrence it was answering.** Only
  the MCP server could attach that before — it holds the surfaced set in memory, so
  an agent answering the horizon it just read wrote a learnable row, while `ettle
  confirm` and `ettle mute` wrote the kind and zero. That is backwards from where the
  rows come from: the default install is hooks-only, no MCP server, so the surface
  producing the most verdicts was producing the ones `ettle calibrate` counts and can
  never use. The horizon now records what it showed, keyed by room — which tangles
  you see depends on `--me`, what their recurrence was does not — and the shell
  verdicts read it back. A tangle absent from the last horizon still records zero
  rather than a guess, because `ettle calibrate` cannot tell an invented row from a
  real one. Held-back tangles are never recorded: the floor means they were not
  shown, so no verdict can be answering them.
- **Saying a tangle is real now does something, so the verdict log stops arriving
  one-armed.** `not_real` and `handled` mute — the nuisance ends, which is a payoff a
  human can feel — while `real` left the tangle exactly as it was, asked about again
  every session. A verdict that costs something and buys nothing is one nobody
  records, and that is a sampling bias in the only ground truth the calibration loop
  will ever have. `real` now confirms: the tangle stays on the horizon, because it is
  a live conflict and hiding it would be the opposite of what confirming means, and
  it stops being asked about. Mute means stop showing me this; confirm means stop
  asking me about this.
- **`ettle confirm`** is the shell half, for the same reason `ettle mute` is: the
  default install is hooks-only, so until now the only way to record a `real` verdict
  at all was the MCP server. Someone on that install could tell ettle it was wrong
  and had no way to tell it that it was right.
- **The horizon's ask names all three verdicts and disappears when they are all
  answered.** It used to invite only `not_real` and `handled` — which is where the
  bias came from — and it asked again every session regardless. `ettle_respond`'s
  `clear` now withdraws a confirmation as well as a mute, because a human taking back
  an answer means the answer, not the store it happened to land in.
- **`ettle calibrate` reads the verdict log and says what it does not support.** The
  cut points have been hand-set from the `eval --separability` batch since they were
  introduced, with a note that a loop would learn them from accumulated human
  verdicts. The verdicts have been accruing in a schema built for exactly that
  (`Kind`, `Votes`, `Samples`, `Firm` on every row) and nothing has ever read them
  back, so whether any had accrued was unknown by construction. This reads them,
  groups by kind, crosses verdict against recurrence, and reports per kind whether
  the evidence can inform that kind's bar. It writes nothing — not the log, not the
  constants. A loop that moved its own thresholds from its own surfaced-and-judged
  tangles is the machine-speed feedback loop CONCEPT.md rules out, so the read and
  the write are separate on purpose and not for now.
- **Two limits print on every run, including an empty one**, because both are
  properties of what gets recorded rather than of how much: the horizon asks for a
  verdict when a tangle is *wrong or already handled* and confirming a good one
  changes nothing the human can see, so the log leans negative and a kind with no
  `real` arm cannot move its bar at any sample size; and nothing below the drop
  floor is ever surfaced, so these rows can show the floor is too low and no row
  in this file can ever show it is too high. A kind is blocked on the structural
  check before the volume check, so 50 one-sided rows report the missing arm rather
  than asking for more of the same.
- **The bars come from the engine** (`ettlemesh.FirmBarFor` / `DropFloor`, newly
  exported) rather than being restated in the reporter, so a report cannot keep
  printing 0.25 the day someone changes the gate.

## v0.4.1 — 2026-08-05

- **A contested tangle now arrives in the session already staged as the choice.**
  `internal/mcpserver` never touched `internal/crux`, so the resolver stage was
  CLI-only and a values call reached an agent as an undifferentiated question — the
  one thing v0.4.0's "carries the whole engine" claim got wrong. `ettle_horizon` now
  attaches a `crux` to firm decision-rights and team-wide-divergence tangles, and to
  nothing else, because staging a choice for something bindable puts friction exactly
  where it doesn't belong. The default resolver is `crux.Inline` — no service, no key
  — so this holds on every install. Every crux carries the instruction not to take the
  decision, and an unreachable resolver reports itself in the branch text rather than
  failing the whole horizon; the choice does not stop being the humans' because a
  service was down.
- **`ettle mcp --gemot <url>`** swaps that resolver for a real deliberation, the same
  seam and the same `ETTLE_GEMOT_TOKEN` the CLI uses. Worth naming what this does not
  cover: ettle's server dials gemot over HTTP itself, so this is gemot-the-service. A
  gemot loaded as a stdio MCP server inside your own session is reachable by your
  agent and not by ettle, and no bridge between the two is built.

## v0.4.0 — 2026-08-05

- **The MCP surface now carries the whole engine.** ettle is agent-first — the CLI is
  for completeness — but three things were reachable only from a shell, and an agent
  that has to remember to shell out is an agent that won't. `ettle_mirror` shows a
  person what the team's directed models believe about them and which of those beliefs
  their later work has already made stale; `ettle_drift` is the emit side, which
  changes route to whom instead of being broadcast; `ettle_room_status` is presence off
  the bus. `ettle_respond` gains a `clear` verdict — the undo for a mute — which writes
  no label, because "I muted that by mistake" is not a claim about whether the detector
  was right and the calibration log should only hold claims that are.
- **L2 from a session needs no notes and no key.** The CLI reaches the directed-model
  layer by distilling two directories of notes, which a session has neither of. But the
  bus is already past distillation, and the mesh core only ever wanted
  `map[person][]Atom` per round — so the two rounds are the atoms the bus held when the
  session first read it against what it holds now, and `ettle_mirror`/`ettle_drift`
  make no model call at all (`internal/mcpserver/reflect.go`). The baseline is captured
  lazily on the first bus read, not at startup (a Linear round-trip in front of session
  start is exactly what the horizon cache exists to avoid), and never rewritten — a
  baseline that chased the current state would report every mirror clean.
- **`ettle init` stays the one shell-only command**, because it creates the room the
  server connects to. Noted in README and `docs/SURFACES.md` rather than left implicit.
- **Docs caught up with the lead path.** Two README paragraphs and `docs/ARCHITECTURE.md`
  still read as though the git-repo bus were how a team goes multiplayer — ARCHITECTURE
  called NATS "the default distributed rail" — and `docs/DEPLOY.md`, the doc about
  running this for a team, had no Linear or GitHub tier at all. That rail is now Tier 1
  there, with the folder and git tiers at 1b/1c.
- **`docs/ADOPTION.md` was making three claims the code no longer supports.**
  Requirement 3 said nothing enters the shared layer but a participant's own session,
  which `ettle pull` contradicts: the consenting act is writing in the room, not running
  the binary, and the inference that buys is now stated where it can be argued with.
  Requirement 4 said the L2 model was unreadable by its subject, which `ettle mirror`
  fixed. Requirement 6 promised a clean exit the append-only git bus cannot give — the
  property that makes that tier's identity non-spoofable is the one that keeps what you
  wrote in `git log`.

## v0.3.5 — 2026-08-05

- **`ettle mute` now records the verdict, and refuses to guess it.** The MCP path
  (`ettle_respond`) has always captured a `Label` — verdict, kind, recurrence, firm
  tier — which is the ground truth the calibration loop is meant to consume. The
  shell mute wrote the silence and dropped the reason, so a verdict entered from a
  terminal vanished. It now writes the same JSONL log through `mcpserver.RecordLabel`,
  at the same `ETTLE_LABELS_PATH`, and requires `--wrong` (a false alarm) or
  `--handled` (real and dealt with) rather than defaulting: those are opposite claims
  about whether the detector was right, and conflating them silently poisons the only
  data the loop will have. Recurrence features stay zero on a shell verdict, because
  only the server that surfaced the tangle held them.
- The horizon block leads with `ettle_respond` and keeps the shell form as the
  fallback, matching how ettle is actually driven.

## v0.3.4 — 2026-08-05

- The horizon block named only the shell mute. It leads with `ettle_respond` now.

## v0.3.3 — 2026-08-05

- **A wrong tangle had no off switch on the install we tell people to use.** The mute
  store had exactly one writer — the MCP server's `ettle_respond` — and
  `ettle init --install-hooks` wires the hooks and not the MCP server, so anyone on
  the default path who got a wrong tangle watched it re-inject at the top of every
  session until the underlying atoms happened to change. `ettle mute <kind> <people>`
  is that off switch: it reads the way a horizon line does (`ettle mute duplication
  ivo mara`, case and comma tolerant, exact key accepted), suppresses on every bus,
  and `--clear` undoes it — a mute that could only be added would be its own trap.
  The horizon block now tells the agent the command exists, so it can offer it at the
  moment the wrong tangle lands. `tanglestate.Key` lowercases the kind alongside the
  parties, so a hand-typed `Duplication` mutes the tangle instead of a phantom.
- **`committed:` read as git.** In a tool whose atoms come out of coding sessions, the
  presence view labelling a commitment `committed:` invited exactly the wrong reading.
  It's `committed to:` now.

## v0.3.2 — 2026-08-05

- **The horizon told the agent that an adopter couldn't see their own tangle.** Every
  un-escalated tangle was flagged `not yet shared`, glossed as "one the other person
  can't see" — true only for someone who doesn't publish to the bus. A teammate who
  does gets the same tangle in their own horizon at the start of their next session,
  escalated or not, so the flag turned "they already have this" into an offer to go
  post it at them. The tag now splits: `each side sees it` when every party is on the
  bus (escalate only to make it one shared Linear artifact instead of several private
  views), `not yet shared` when someone isn't (escalation is the only way they hear of
  it). An unknown participant list reads as un-shared, so a silent teammate is never
  reported as informed. The legend explains only the tags actually on screen.

## v0.3.1 — 2026-08-05

- **`ettle room list` told people in a working room that they had none.** It read only
  the git-repo bus registry — a directory that exists because leat rooms are the only
  ones ettle has to clone — so anyone on Linear or GitHub got "no rooms yet" while
  standing in a room with four people publishing to it. It now leads with this
  project's room from `.ettle-room`, then every room this machine has an identity for
  on any bus, then the git-repo ones; the empty case points at `ettle init` rather than
  only at the no-platform path. The identity file records its room spec so the registry
  can name what it lists (the filename is sanitized, so it can't), and files written
  before that are backfilled the first time a command resolves them.

## v0.3.0 — 2026-08-05

The set-and-forget release: ettle stopped being a thing you run. `ettle init` sets a
team up in one command on a Linear project or a private repo's GitHub Discussions,
and from then on sessions publish themselves and the tangles that involve you surface
at the start of the next one. **Anyone who installed before this tag has none of it** —
`go install …@latest` was still serving v0.2.1, which predates init, the hooks, the
GitHub bus, and the project room file.

- **`make ci` runs a prose vocabulary gate** (`calque vocab-check` against
  `.calque/vocab-allowlist.txt`), so a second word competing with an existing one
  fails the build instead of surfacing in review weeks later. Skips itself when
  calque isn't installed.
- **Muting was silently broken on every bus except Linear.** The per-room tangle stores
  keyed off the *Linear room*, so on a `github://` or leat room the key fell through
  to a shared `"default"` bucket — every non-Linear room on the machine muting each
  other's tangles — and the injected SessionStart horizon skipped mute-suppression
  entirely because it was gated behind "is this Linear?". Since muting is the only
  thing that stops a wrong tangle re-surfacing every session, that made the
  wrong-tangle failure unfixable on exactly the bus a GitHub team uses. The stores now
  key by the transport **spec**, so every room gets its own bucket and muting applies
  everywhere; escalation stays Linear-only, so `escalated` is still nil elsewhere and
  a non-Linear horizon shows no share tags. `mcpserver.Serve` takes the two keys
  separately (`stateKey` for the stores, `linRoom` for the escalate target) rather
  than conflating them.
- **`ettle init` needs no room name inside a GitHub repo.** Naming a room was
  arbitrary startup friction, and worse than arbitrary: two teammates typing
  different names each sit alone in a room they think the other is in. Bare `ettle
  init` now derives `github://<owner>/<repo>` from the `origin` remote, so everyone
  runs the same command and lands in the same room. A non-GitHub remote derives
  nothing rather than guessing.
- **A GitHub room no longer installs the two Linear-only hooks.** `pull-hook` reads
  Linear agent activities, so wiring it — plus a `PostToolUse` matcher on a Linear
  MCP server the team doesn't run — meant two hooks that could only ever no-op.

- **Setup is drivable by an agent, found by walking the path as one.** Three things
  broke for a coding agent told "install and set up ettle," none of which show up
  when a human does it:
  - **`ettle <cmd> --help` exited 1** with a spurious `ettle: flag: help requested`
    line, because Go's flag package prints the usage itself and returns
    `flag.ErrHelp`. An agent discovering the tool reads that as a broken command
    rather than as documentation. One `exitOn` helper now handles every subcommand's
    exit, so `--help` is exit 0 with just the usage.
  - **The report pointed at `docs/LINEAR_SETUP.md`**, a repo-relative path that does
    not exist for the person most likely to need it — whoever ran `go install` and has
    no clone. It is a URL now.
  - **`ettle init --json`** emits the whole report as structured data, so an agent can
    branch on which key is missing instead of parsing English. Same facts as the prose
    rendering, not a reduced machine mode.

- **A GitHub bus: `github://<owner>/<repo>[/<room>]`, a private repo's Discussions.**
  The Linear docs-as-bus shape for a team that lives in GitHub instead — the room is a
  Discussion titled `ettle/<room>`, each participant owns one comment carrying their
  envelope, replace-current so N people cost N comments. It needs **no new secret**:
  the credential `gh auth login` already stored is enough, which is the whole reason
  it exists beside the leat bus, which needs a separate repo created, cloned, and
  seeded first. `ettle init github://acme/widgets/crew` sets it up with the same
  report as the Linear path.
  - **It refuses a PUBLIC repository, and there is no override flag.** A public repo's
    Discussions are readable by anyone on the internet, and the bus carries every
    participant's intents, commitments, and assumptions. A Linear project is
    workspace-scoped and a private repo's Discussion is collaborator-scoped —
    comparable audiences; a public repo is a categorically different one. The check
    runs at construction, so a repo flipped public later fails the next publish loudly
    rather than leaking quietly. The residual risk it cannot close — a private repo
    made public afterwards, taking its history along — is named in the package doc
    rather than papered over.
  - Identity rides a `<!-- ettle:<name> -->` marker, not the comment author: `ettle
    pull` publishes a non-adopter's atoms under *their* identity using the puller's
    token, so the author can never be the check. Same limit LinearBus documents for
    its title identity, and a mismatched in-content claim is overridden and warned.
  - Comments without the marker are left strictly alone, so a teammate replying in the
    thread is never parsed as atoms.
  - **Roles 2 and 3 have no GitHub equivalent yet.** `pull` and `escalate` ride
    Linear's agent activities; GitHub has no counterpart, so a GitHub team gets the
    bus and the in-session whisper, and reaching a non-adopter is unbuilt.
- **`ettle init` reports the room's audience on Linear.** Linear has no
  internet-public project — `Team.visibility` has `public`/`restricted`/`private`,
  where "public" means the whole *workspace*, not the world — so unlike the GitHub
  path there is nothing to refuse. What there is, is a reader who should know which
  colleagues can see what they are about to publish, so init now names the owning
  teams and their visibility in plain words (`LinearBus.Audience`).
- **The init report survives a failure partway through.** It was buffered and printed
  at the end, so a run that died on, say, an unwritable `--dir` threw away every check
  it had already gathered — exactly the diagnosis the person needed. It now flushes on
  every exit path.

- **`ettle init <room>` — the Linear + Claude Code setup, in one command.** Everything
  was built and the assembly was six manual steps, only one of which (the OAuth
  app-actor token) was documented anywhere but an error string. `ettle init` now
  verifies the environment and **says what each missing key costs you** rather than
  failing on the first one — a missing `LINEAR_AGENT_TOKEN` reads as "escalation is
  off," not as a broken install. It resolves or creates the room's Linear project by
  actually building the transport (an env var being set proves nothing about whether
  the key works), reports who is already publishing there, writes the project's
  `.ettle-room` pointer, and with `--install-hooks` merges the four Claude Code hooks
  into `~/.claude/settings.json` — backing up the previous file, skipping anything
  already wired (including a hand-tuned entry that added flags), and never joining a
  group someone else's hooks live in. A new [docs/LINEAR_SETUP.md](docs/LINEAR_SETUP.md)
  walks each of the four keys, including the `actor=app` OAuth flow escalation needs
  and what ettle touches in a workspace.
- **The hooks no longer name a room, so one global config serves every project.**
  A hook bundle in `~/.claude/settings.json` fires in every session of every project,
  which meant a hard-coded `--room` was wrong everywhere but one repo. The room now
  travels with the project in a `.ettle-room` file that every command walks up to find
  (`cmd/ettle/roomfile.go`), so `ettle horizon`, `ettle escalate`, and `ettle pull` take
  no flags inside a project, and the three `-hook` commands are **silent no-ops** outside
  one — a non-ettle repo produces no output and exit 0 rather than a usage error in every
  session. Explicit flags still win. The file holds only the room, deliberately: identity
  is kept per-machine (`ettle init --me`), because a committed `me = alice` would publish
  Bob's atoms under Alice's name.
- **README brought back to what the code does.** The status section still described the
  Linear emit half as deliberately unbuilt after `ettle escalate` shipped, and said
  nothing at all about capture, horizon injection, or the hooks — so the whole
  set-and-forget loop existed only in `docs/SURFACES.md`. The status paragraph now
  describes the loop that runs, and the quickstart leads with the team setup instead of
  ending at a demo over note files. `.env.example` grew the three Linear variables it
  had never mentioned.

- **The injected SessionStart horizon is now agent-framed and at parity with the MCP
  tool.** The cached block `ettle horizon-hook` injects used to read as a note to a
  human. It now addresses its actual reader — the agent — as a standing instruction
  ("You are alice's ettle agent … when their work touches one, raise it; don't dump
  the list"), and carries the same tangle state as `ettle_horizon`: muted tangles are
  suppressed (with an honest count), and each un-shared cross-person tangle is flagged
  `not yet shared` so the agent knows exactly which ones to offer escalating. Tags
  show only for a Linear room (escalation is Linear-only); a leat/in-proc horizon is
  unchanged. Keyed through `internal/tanglestate`, so the injected block, `ettle
  escalate`, and the MCP tools all agree on what's escalated and what's muted.
- **MCP operator tools: drive escalation from inside a session, and make a tangle go
  away when it's handled.** The agent (me) operates ettle through MCP tools, not by
  remembering CLI commands, so three things landed on the `ettle mcp` surface:
  - **`ettle_escalate`** posts one tangle (by the `key` from `ettle_horizon`) as a
    Linear elicitation on the room's coordination issue — so when I notice a
    collision a teammate can't see, I offer it and, on yes, escalate that one tangle
    inline. Enabled only for a Linear room with `LINEAR_AGENT_TOKEN` set.
  - **`ettle_horizon` tags each tangle `escalated`** and **suppresses muted tangles**
    (with an honest `muted` count), so I offer to escalate only what a non-adopter
    can't already see and stop re-raising what's resolved.
  - **`ettle_respond` now acts:** a `not_real` or `handled` verdict **mutes** the
    tangle so it stops re-surfacing and won't be escalated (the calibration loop that
    consumes verdicts is still unbuilt — muting makes the verdict do something now).
  A new shared `internal/tanglestate` package keys a tangle the same way everywhere and
  holds the per-room escalated/muted sets, so a tangle escalated by `ettle escalate`
  (CLI) is recognized as escalated by `ettle_horizon` (MCP), and a mute from either
  side is honored by both. Verified: the tool is gated on the app token, the write
  path is the same live-proven `LinearAgentWriter`, and the handler/mute/suppress
  logic is unit-tested.
- **`ettle escalate` — surface a coordination tangle to a teammate who won't install
  ettle, on Linear.** The emit half of the Linear agent path and the one command
  that writes *onto* Linear. It reconciles the room's atoms, takes the **firm
  cross-person** tangles (firm is the calibration gate — a tangle below the recurrence
  bar never posts), and surfaces each **new** one as a native agent elicitation on
  the room's **single coordination issue** ("ettle coordination" in `ettle-<room>`),
  **never a feature ticket**. The teammate replies inline and `ettle pull` brings it
  back — the loop closes. Authenticates as the OAuth **app actor**
  (`LINEAR_AGENT_TOKEN`, Bearer; the member key can read agent activities but not
  post them — a new `bearer` auth mode on the Linear backend). Idempotent per room:
  a tangle already posted is skipped. **Opt-in and deliberate** — the default install
  never posts; escalation is the move you make on purpose to reach a non-adopter
  (whisper-first — see [docs/SURFACES.md](docs/SURFACES.md)). Verified live: alice's
  Redis plan vs bob's stateless assumption reconciled to a firm collision and posted
  as an elicitation on the coordination issue; re-run was a no-op.
- **Horizon injection: the tangles relevant to you appear in your session at start,
  unprompted.** `ettle horizon --room <room> --me <you>` reconciles the atoms
  capture/pull already put on the bus into the coordination tangles involving you —
  no note files, just the live bus. `ettle horizon-hook` wires that to a Claude Code
  **SessionStart** hook: because reconcile is a model call, the hook injects a
  **cached** rendered horizon instantly (never blocks, no per-start cost) and spawns
  a **detached** `ettle horizon --cache` to refresh it for next time — the cache
  self-warms from use. Surfaced **privately** into your own session; nothing is ever
  posted (whisper-first — see [docs/SURFACES.md](docs/SURFACES.md)). The refresh is
  debounced per identity so session churn doesn't spam reconciles. This is the
  surface half of set-and-forget; with capture (send) and pull (receive) the loop
  closes with no command to run.
- **Auto-capture: a session puts you on the bus with no command to run.** `ettle
  capture --room <room>` distills *this* Claude Code session's own reasoning
  **locally** (reusing `internal/capture` + the detector) and publishes the atoms
  as you — raw prose never crosses, only typed atoms. `ettle capture-hook` fires it
  from a Claude Code **SessionEnd** hook (and optionally **Stop** for mid-session
  freshness), spawned **detached** so it never blocks the agent and **debounced** so
  a per-turn Stop collapses to the occasional distill. A session that distills to no
  atoms publishes nothing, so an empty session never erases your atoms. This is the
  send half of set-and-forget; with `pull-hook` (the receive half) the loop closes
  with nothing to remember. `ettle capture <transcript>` with no `--room` still just
  prints the digest. Identity is `--me`, else the room's agent, else `$USER`. See
  [`hooks/`](hooks/) and [docs/SURFACES.md](docs/SURFACES.md).
- **`ettle pull` — a teammate who never installs ettle can contribute through
  Linear's native agent UI.** When someone replies to ettle in a Linear agent
  session, `ettle pull --room <room>` reads that reply (a plain member key — no
  OAuth app token), distills it **locally** under that teammate's identity, and
  publishes the atoms to the room. So a non-ettle coworker becomes a first-class
  voice in reconcile without running anything or holding a key of their own. Raw
  prose never touches the bus — distillation stays on the machine that pulls, the
  same privacy boundary as everywhere else. It runs automatically before a
  `linear://` standup's `Collect` (non-fatal, and cursor-bounded so a quiet pull is
  one cheap query returning nothing new), so nobody has to remember it; the
  standalone command is for explicit runs. Deliberately deferred: the *emit* half
  (surfacing ettle's own tangles as Linear elicitations, which needs the OAuth
  app-actor token) and a hosted webhook relay — the receive path is member-key
  polling, no always-on server.
- **`ettle pull-hook` + an example Claude Code hook config** so pull runs
  recurringly without anyone remembering it. Wired to `SessionStart` and a
  `PostToolUse` matcher on the Linear MCP tools (see [`hooks/`](hooks/)), it fires
  whenever a session touches Linear; it spawns pull **detached** (never blocks the
  agent) and **debounces** so a burst of tool calls collapses to one pull. The
  standalone `ettle pull` and the auto-pull-before-a-`linear://`-standup path
  remain; this is the always-on-without-a-server trigger.

- **A Linear-backed transport (`linear://<room>`), for teams already on Linear +
  Claude Code.** The room is a Linear project, each participant owns one document,
  and that document's content is their current envelope — so the bus is a project
  the team already has rather than a git repo to stand up. Set `LINEAR_API_KEY`
  (and `LINEAR_TEAM_ID` the first time, to create the project); it slots in behind
  the existing `transport.Transport` seam next to `file://`, `leat://`, and `nats`.
  Storage is replace-current (`documentUpdate` overwrites in place), so the
  footprint is N documents for N people — bounded like `DirBus`, with no per-emit
  accumulation. Measured, not assumed: API `documentUpdate` accrued zero visible
  revision-history snapshots over 12 rapid writes, so no doc-rotation is needed.
  Honest limit: one room token means Linear's actor is the token owner, not the
  participant, so identity rides the document title (`ettle/<slug>`, authoritative
  on read) and the envelope, without Linear-actor corroboration — leat's git-author
  check stays strictly stronger. As a guest on their platform the client sends a
  `User-Agent` identifying ettle and surfaces a 429 as a distinct rate-limit error.

## v0.2.1 — 2026-07-22

The onboarding release. v0.2.0 shipped a key-free teammate path that could not
actually be reached by a teammate without a key — anyone installing `@latest`
before this tag gets that build, so this is the version to point a new coworker at.

- **The key-free teammate path actually works now — found by dry-running the tool
  as a new coworker** (clean `GOBIN`, a separate `HOME` per machine, a stand-in
  git remote, nothing from the checkout on `PATH`). Three defects, all inside the
  first four minutes:
  - `ettle mcp` **required an API key to start**, which made the key-free path
    unreachable by exactly the person it was built for. Client-side distillation
    exists so a teammate never needs a key; `ettle_emit` with `atoms` and
    `ettle_respond` make no model call at all. The key is now optional: without
    one the server serves that half and the model-calling tools return
    `mcpserver.ErrNoKey`, which names the alternative (`ettle_distill` + `atoms`)
    rather than reporting a bare missing key.
  - `ettle mcp` **had no `--room`** and kept the horizon in a process-local map,
    so two people on two machines never saw each other's atoms over MCP — the
    distributed path and the key-free path did not intersect anywhere. The
    horizon now rides the same `transport.Transport` seam the CLI uses
    (`ettle mcp --room <name>`, or `--transport`), with a fold-by-participant at
    the read side so a re-emit overwrites on an append-only bus. Default is
    unchanged: in-process, this server only.
  - `ettle room join` **printed the one command guaranteed to fail** for a
    keyless joiner (`use it: ettle standup --room …`). It now branches on whether
    a key is present and leads with what will actually work. `ettle room` with no
    args gained a real usage block.

  Verified end to end after the fix: a keyless "Bob" drove `ettle mcp --room`,
  emitted client-distilled atoms into a git-backed room, and the key-holding
  "Alice" reconcile caught the resulting collision (5/5 samples) across the bus.

- **README install path made self-consistent, and the forge metadata refreshed.**
  The quickstart now leads with `go install …@latest`, states the substitution in
  the direction the examples are actually written (`ettle` *for* `go run
  ./cmd/ettle`, not the reverse), and shows the installed-binary form in the
  `ettle room` block — the distributed flow needs no bundled `testdata/`, so that
  is the block a teammate actually copies. `claude mcp add` is given both ways.
  GitHub description, topics, and homepage updated: the old description predated
  rooms and the key-free path, and `homepage` was unset.

## v0.2.0 — 2026-07-22

The distributed release: ettle stopped being a single-host demo. A team can now
join a room with one command over a private git repo, see who's present, and —
new in this release — take part without an API key of their own. The core object
was also renamed `tangle` → `tangle`.

- **Client-side distillation — a teammate no longer needs an API key to take part.**
  `ettle_emit` now accepts already-typed `atoms` as an alternative to raw `notes`
  (exactly one of the two; supplying both is an error rather than a silent
  precedence rule), and a new **`ettle_distill` MCP prompt** hands the caller's own
  agent the distillation rules so it can produce those atoms locally. An agent that
  already holds its human's notes and has its own model — anyone driving ettle from
  Claude Code or Cursor — can now contribute with **no key and no server-side model
  call**; only whoever runs `ettle_horizon` needs one. One key per room, not one per
  person, which was the single biggest barrier to a team trying this.
  **This strengthens the privacy boundary rather than relaxing it.** The boundary was
  never between a person and their own agent; it is between that person and the team.
  Distilling client-side makes "raw notes never cross" *structural* — they never leave
  the person's machine — instead of a promise the server asks to be trusted on. The
  semantic half of the boundary (the contextual-integrity prompt) now runs somewhere
  unverifiable, so the deterministic half still runs server-side on arrival: new
  `ettlemesh.SealAtoms` puts caller-supplied atoms through the *same* chokepoint
  (structural caps, secret-shape scanner, per-person privacy override) that
  server-side distillation uses, drops unknown types, and **forces attribution** —
  `atomIn` has no `from` field at all, so a client cannot put words in a teammate's
  mouth by construction rather than by validation. The prompt itself is shared:
  `ettlemesh.DistillSystemPrompt` is now exported and used by both paths, so they
  cannot drift on what may cross.
  Tests: `TestEmitAcceptsClientDistilledAtomsWithoutAModelCall` (a reconciler whose
  `Distill` panics proves the key-free path makes no model call),
  `TestEmitSealsClientAtomsAndForcesAttribution`,
  `TestEmitScrubsSecretsInClientSuppliedAtoms` (a client-sent credential is still
  redacted before storage), `TestEmitRejectsBothOrNeither`,
  `TestDistillPromptCarriesTheSameBoundaryRules`.
  *Not* built: MCP **sampling**, which would have been the obvious way to borrow the
  caller's model. Claude Code has never implemented it as a client
  ([anthropics/claude-code#1785](https://github.com/anthropics/claude-code/issues/1785),
  open since June 2025) and
  [SEP-2577](https://modelcontextprotocol.io/seps/2577-deprecate-roots-sampling-and-logging)
  (Final, 2026-04-14) deprecates `sampling/createMessage` protocol-wide. Client-side
  distillation reaches the same goal using only current, non-deprecated MCP.

- **Prior art: Dust's "pods" (`docs/PRIOR_ART.md` §10).** A close read of one primary
  source (an MLOps Community Podcast episode with a Dust co-founder) rather than a
  search pass — the clearest public statement of ettle's single-player→multiplayer
  diagnosis by a team shipping the *opposite* answer: pooled shared state (a shared
  filesystem across humans, agents, and sessions) instead of bounded per-person state
  with cross-person reconciliation. Records the load-bearing disagreement (coordination
  failure is caused by distributed private information, not long task horizons — which
  is why *useful at N=1* holds), the two self-refutations in the interview
  (filesystem-concurrency → reaching for git; anthropomorphic-tools → building
  agent-native infra anyway), and two things worth taking (bidirectional access as an
  explicit invariant; the "fog of AI" three-month-direction planning posture).

- **Verdict labels now record the tangle's recurrence (calibration capture polish).**
  `ettle_respond` already logged a human verdict (`real` / `not_real` / `handled`) on
  a surfaced tangle; the captured `Label` now also carries the tangle's **recurrence
  (`Votes`/`Samples`) + kind + firm/soft tier** — the exact per-kind feature a future
  calibration loop would threshold on, which was previously discarded at capture time.
  Populated by joining the verdict to the horizon the same server just surfaced (a
  cross-session verdict keeps the kind, recovered from the key, with zero recurrence).
  Backward-compatible (`omitempty`, so pre-enrichment log lines still parse). **The
  learning loop itself is deliberately NOT built** — with no real users there are no
  accrued labels yet, so it would only fit a synthetic corpus; this change just stops
  the feature being lost so the data is learnable if it accrues. Labels stay **local**
  per machine (pooling across a team would leak coordination-judgment metadata).
  Tests: `TestRespondEnrichesLabelFromHorizon`, `TestLabelBackwardCompatThinLine`.

- **`ettle room status` — the presence view (L0 co-presence).** Shows who's in a
  room and what each person is currently working on, read straight off the bus (the
  atoms standup already published) with **no tangle detection and no model call**.
  Participants are sorted, atoms framed by type (intent → "working on", commitment →
  "committed", dependency → "depends on", assumption → "assuming"), each with a coarse
  freshness cue (active / today / yesterday / Nd ago). To make freshness survive the
  leat path, the leat adapter now stamps `EmittedAt` on publish if unset (display-only,
  never used for ordering — leat's per-lane seq is authoritative). The render is a pure
  function (`renderRoomStatus`, clock injected) and unit-tested (`TestRenderRoomStatus`).
  This is the co-presence layer the project had skipped on the way to tangle detection: a
  room is useful — "what is my crew's agents doing right now" — before any reconciliation.

- **`ettle room` — one-command join for distributed mode.** Collapses the leat
  setup ceremony (clone a git repo, seed a HEAD, remember three env vars + an
  absolute path) into `room init <git-url>` (first person — creates/seeds/pushes)
  and `room join <git-url>` (everyone else — clones), with `room list`. The git URL
  is the invite. A room is saved under `<user-config>/ettle/rooms/<name>/` (a
  `config.json` + the managed clone), and `standup --room <name>` resolves that
  room's repo/agent/remote — so day-to-day use needs no `LEAT_*` env vars or
  `--transport` string. `--as <id>` sets your lane id (default `$USER`, coerced to a
  valid leat id by the shared `transport.SanitizeID`). Local-only rooms (no git URL)
  work for single-host/testing. Tested (`TestRoomInitLocalAndBus` round-trips an
  envelope through a resolved room; `TestRoomNameFromURL`). The leat sanitizer is now
  the exported single-source `transport.SanitizeID` (was `sanitizeChan`), reused by
  the room command so the id rule lives in one place.

- **leat transport — a distributed atom bus over a private git repo, no server**
  ([github.com/justinstimatze/leat](https://github.com/justinstimatze/leat)). `--transport
  leat://<repoDir>` rides leat, a git repo used as an append-only, per-author-lane message
  bus (durable, cross-machine, identity-hardened, `git log` = the audit trail). ettle is a
  *consumer* of leat — the canonical Go impl of a shared git-transport wire contract owned by
  mcp-dispatch; ettle does not own or reimplement the transport. The adapter
  (`internal/transport/leat.go`, always compiled — leat is stdlib-only, so unlike NATS it
  needs no build tag) maps one LWW record per participant (`Type=atom`, `Chan=room`,
  `Key=participant`, `Body=`the marshaled Envelope) and folds `Collect`'s latest-per-`(From,Key)`
  atom records back into Envelopes. Wired via the build-tag-independent `dirBusFor` hook
  (`leat://` alongside `file://`, single source so it can't drift across the nats/!nats `busFor`
  copies). Config from env: `LEAT_AGENT` (lane id, required), `LEAT_REMOTE` (push/fetch remote),
  `ETTLE_TEAM` (room channel). leat adds only itself to `go.mod` (no transitive deps). Tested
  (`TestLeatRoundTrip`: publish→collect round-trip + LWW-per-participant, hermetic over a local
  git repo). README/DEPLOY now lead the distributed story with leat (NATS demoted to the heavier
  alternative). See [DEPLOY.md](docs/DEPLOY.md) Tier 1c.

- **Subject-gated inference (stage 0b) — inferred atoms don't cross to the team by
  default** ([docs/LEGIBILITY.md](docs/LEGIBILITY.md)). 1a-1 measured the inference pass
  fabricating sensitive de-novo claims ("the speaker is leaving") and *asserting* them.
  So an inferred atom — a claim about a person they never stated — is now **held at the
  boundary**: in the CLI standup (where `distillAll` runs `Distill` + `InferImplicit`;
  the MCP `emit` path runs `Distill` only, so there's no inference channel there),
  `personResult` keeps inferred atoms separate from stated, and by default the inferred
  ones **do not cross the transport** (`bus.Publish` sends stated atoms only). They are
  surfaced to their own subject instead — "inferred about you, held back from the team;
  confirm before it travels." `--share-inferred` opts back into the old flow-to-team
  behavior; the eval path recombines stated+inferred so detection measurement is
  unchanged. This is the enforcement the 1a-1 measurement justified — the de-novo claim
  is held before crossing, not flagged after. Held-back inferred atoms stay **legible**
  (no silent drops, the stage-0a discipline): `--me` shows the subject their own to keep
  or kill; **team view** (no `--me`, no single subject) shows a *count only* — never
  whose or what, which would leak the very claims being gated; and `--show-atoms` labels
  each inferred atom honestly by whether it actually crosses (held-back vs
  `--share-inferred`). Default stays gating-ON by principle: the measurement plus the
  contextual-privacy invariant put the burden on the *modeler*, not the modeled. Tested
  (`TestSurfaceInferredAboutMe`: subject detail, team-view count, `--me`-not-aggregate).
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
  default vs `--by-observer`). Also folded `printTangle`/`printAsk`'s duplicated
  vote-suffix into one `voteSuffix` (calque dual-path, score 0.44).
- **Label capture (stage 0c-2) — `ettle_respond` records the human verdict**
  (`internal/mcpserver`; [docs/LEGIBILITY.md](docs/LEGIBILITY.md)). A new MCP tool lets
  a person's agent answer a cross-person tangle from `ettle_horizon` — `real` /
  `not_real` / `handled` — keyed by the tangle's wording-independent `key` (now on every
  `tangleView`). Each verdict is appended as a `Label` (`{key, verdict, by, note, ts}`)
  to a local JSONL (`ETTLE_LABELS_PATH`, default `ettle-labels.jsonl`, gitignored).
  This is the **active-learning label stream** stage 2's calibration loop will consume
  — written now so the data accrues before the loop exists (a detector flag-rate is
  only calibratable against confirmations from people who saw the work). It records
  **only**: no binding, no horizon mutation — humans stay the deciders. Label sink is
  an interface (file by default; tests inject memory). Tested:
  `TestRespondCapturesLabel` (capture + verdict/field validation, no-capture on
  reject), `TestTangleKeyStableAndCrossCallMatch` (order/case-stable key).
- **Interrogative register (stage 0c) — cross-person tangles are posed as questions,
  not asserted** ([docs/LEGIBILITY.md](docs/LEGIBILITY.md)). The detector has no
  ground truth for a cross-person conflict, and recurrence is test-retest *stability*,
  not validity — so it has no standing to assert one. The CLI `surface` now routes
  **self tangles** (a person's own drift, which they can verify) to an asserted "worth a
  look" lane and **every cross-person tangle** to a "worth checking together (a question,
  not a claim)" lane — "[possible collision] … Real, or already handled?" — ordered
  firm-first; contested ones still pre-stage their either/or. The MCP `horizon` marks
  each cross-person `tangleView` `question:true` so agent consumers present it as a
  question too. Grounded in mixed-initiative design (act when confident+positive-sum,
  ask otherwise) and trust calibration (communicate true uncertainty, don't overclaim).
  The Firm-and-bindable act-lane for cross-person tangles opens later, *earned per kind*
  against the calibration label (stage 2) — so this register is also the active-learning
  query front-end that loop will need. Deterministically tested (`TestSurfaceActAskRouting`).
- **Legible abstention (stage 0a) — the coupling check stops dropping silently**
  (`GroundTangles` now returns `(kept, suppressed)`; [docs/LEGIBILITY.md](docs/LEGIBILITY.md)).
  A clear horizon that silently hid a suppressed call trains the human to stop
  watching — the exact failure a structured adversarial pressure-test (legibility /
  extraction-skepticism lenses) flagged. Tangles the coupling check judges
  *not a real conflict* are now surfaced
  **off the agenda**, in a "held back — shown in case that's wrong" section (CLI
  `surface`) / a `held_back` field + summary tail (MCP `horizon`), filtered to `me`.
  Coupling-check kills are *listed* (high-recurrence, a human might overrule them);
  the abstention-floor drops (≤1/5 samples, noise by design) surface as a single
  quiet **aggregate count** ("+N below the confidence floor, not shown") so the
  notice doesn't get trained into the ignore pile — `ReconcileVoted`/`voteTangles` now
  return that count alongside the kept tangles. Deterministically tested
  (`TestSurfaceHeldBack` captures both the listed section and the floor line;
  `TestDropFloor` asserts the count; `applyGroundingVerdicts` returns the suppressed
  set). First increment of the
  legibility program drafted in `docs/LEGIBILITY.md` (the response to the panel:
  turn the model's output from a private assertion into a legible, contestable
  signal). No detection-accuracy change — the eval still scores only kept tangles.
- **Cross-person coupling check — generalizes the collision direction-check to
  duplication + teamwide-divergence** (`GroundTangles`/`groundableTangles` in
  `internal/ettlemesh/ground.go`). The collision pass (below) closed the *collision*
  vector, but a `--samples 5` re-measure found the **same root error** — two people
  bridged on a shared topic word while working in *independent scopes* — surviving
  voting under two **other** kinds: a fake `[duplication] alice,cleo` (a user-lookup
  cache and a Grafana metrics dashboard read as redundant work) and a fake
  `[teamwide-divergence] alice,bob,cleo` (cleo's unscheduled internal maintenance
  swept into a product launch deadline), together **0.40 FIRM cross-boundary
  tangles/run** on `superposition-userservice-vs-infra`. The pass now asks a
  kind-appropriate **coupling** question of each cross-person collision/duplication/
  teamwide tangle: collision → do both *edit the same artifact*; duplication → are both
  *building the same deliverable twice*; teamwide → does the named assumption actually
  *govern every party* and do they hold it *differently*. decision-rights is excluded
  (a who-decides truth condition the coupling question would misjudge). Measured
  (haiku, `--samples 5`): userservice-vs-infra FIRM cross-boundary **0.40 → 0.00**
  (CI 0.00–0.00, both fabs gone); **real-tangle recall held 1.00 across kinds** — real
  teamwide (calendar K1), real duplication (duplicate-util K1), real collision
  (schema-collision K1) all kept at precision 1.00; labeled fakes duplicate-util D1
  (CI test-retry vs HTTP backoff) and shared-deadline-null D1 (agreed Q3 freeze)
  dropped. To keep each kind's instruction undiluted, the pass makes **one focused
  call per kind present** (collision / duplication / teamwide) rather than one merged
  3-kind prompt — cost is +1 model call per additional distinct kind. The same change
  numbers each prompt's tangles by their **full-slice index**, fixing a latent
  verdict-mismap that silently failed to drop a fabrication whenever a
  self/decision-rights tangle preceded a groundable one (fail-open kept it).
  Re-smoke-tested after the split: userservice-vs-infra FIRM still **0.00**, real
  teamwide (calendar K1) and real duplication (duplicate-util K1) recall held **1.00**.
  **Caveat:** the pass is a *single probabilistic judge call*, not a deterministic
  gate — it lowers fabrication probability but a borderline fab still flickers firm
  run-to-run (frontend-vs-data's mabel/opal collision, calendar's "review" D1); n=5
  can't claim a stable per-corpus rate, and that flicker (finding #5) is accepted for
  now. Default ON across `standup`, `eval`, and the **MCP horizon**; disable with
  `--no-ground`.
- **Collision direction-check — closes the residual fabrication the floor couldn't
  reach, now ON by default** (`GroundTangles` in `internal/ettlemesh/ground.go`). The
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
  `internal/ettlemesh/mesh.go`, applied in `voteTangles`) closes the bulk of the
  cross-group fabrication the robustness battery surfaced. A voted tangle recurring
  below the floor (0.25 of samples — strictly under the lowest per-kind firm bar, so
  it can never drop a tangle the firm bar would assert) is dropped entirely: not
  asserted, not asked. It catches the fabrication *tail* (separability: fabricated
  cross-group tangles recur ≤~0.17 of runs), which is most fabrications, at **zero
  clear-tangle recall cost**. Measured (haiku, `--samples 5`): on the worst corpus
  `superposition-frontend-vs-data` FIRM (asserted) cross-boundary fabrication fell
  **2.60 → 0.50 tangles/run** (~80%); on `superposition-userservice-vs-infra`,
  **0.40 → 0.00**. Real-tangle recall held 1.00 on all eight clear-tangle corpora;
  pooled real-tangle false positives halved (4 → 2). The only recall casualty is
  auth-migration K2, a flickery `decision-rights` tangle already lost to detection
  flicker pre-floor — an accepted miss under the **"lighter agenda, not no meeting"**
  framing (precision is the goal; missing a flickery tangle just leaves it on the human
  agenda). Residual: high-recurrence *polysemy* misreads (e.g. `mabel↔opal` both on
  "analytics") survive — the floor structurally can't reach a 0.5-recurrence tangle;
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
  reconciles the team's atoms into firm/soft tangles filtered to `me`, and
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
- **Per-kind firm bar** — recurrence-voting ranks tangles firm (assert) vs soft
  (ask), and the bar is now per-kind: a genuinely flickery `decision-rights` tangle
  asserts at a lower recurrence (0.3) than the default (0.5), staying clear of the
  fabrication floor. The hand-set seed of the Phase-3 calibration loop. (The
  separability diagnostic established recurrence-frequency, not model confidence,
  is what discriminates real tangles from fabricated ones.)
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
  longer splits a multibyte rune across the boundary; `voteTangles` confidence no longer
  double-counts a run that names one divergence in both the pairwise and team-wide
  pass; a bare `name:` header no longer blanks a participant into the `--me ""`
  full-team sentinel; and the gemot poll loop honors parent-context cancellation
  instead of spinning to its local deadline. Doc-honesty corrections from the same
  panel: CONCEPT/README no longer state the semantic layer as "enforced" or "0% leak"
  as a settled property (it is model judgment, measured on a synthetic corpus);
  `EXAMPLE_RUN` no longer shows a 0.5 tangle as soft (the code routes ≥0.5 to firm); the
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
  surfacing tangles; atoms are now structurally capped (subject/content length,
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

- **First real-data eval corpus** (`testdata/dupbug/`) — the duplication tangle,
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
  rather than whether the detector finds the right tangles. Synthetic notes
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
  as a cross-person tangle); a FIRM tangle that matches one is reported as a **named
  trap the detector fell for**, not just a bare false positive. `--ab` runs
  single-shot vs multi-sample voting with a McNemar test that is now **pooled
  across corpora** — per-corpus the discordant N is always too small to reach the
  reliability gate, so a per-corpus test could never find significance regardless
  of the effect. Fixed the voting clustering it exercises: `SameTangle` uses a
  Jaccard threshold (was: any one shared keyword) and `voteTangles` uses
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
  coordination tangles, and surfaces only what's relevant to each human (`--me`).
  Routes FIRM tangles as "worth a look", SOFT (inference-backed) as "worth a
  question".
- **Useful at N=1** — a single-party **self-assumption** pass (`ReconcileSelf`)
  surfaces an assumption a person's own later work has quietly made false; the
  pairwise/team passes are blind to it by construction. Deduped against the
  cross-person tangles (shared `SameTangle` matcher) so a team-wide divergence isn't
  also reported privately.
- **Multi-sample voting** (`--samples K`, `ReconcileVoted`) — re-runs the
  reconcile passes K times and keeps only tangles recurring across a majority,
  turning the stochastic detector's run-to-run noise into a confidence signal
  (each surviving tangle carries `Votes`/`Samples`, kept separate from
  Confidence). Clustering uses the same `SameTangle` matcher, so a tangle relabeled
  collision→decision-rights across runs still votes as one. Default `K=1` is the
  original single-run cost.
- **Transport seam** — in-process (default, zero infrastructure) and a NATS
  distributed bus (`-tags nats`, TLS + credentials enforced off localhost).
  - The NATS adapter uses **JetStream** (retained stream): a publish-before-collect
    flow over core pub/sub would race and silently drop a peer's atoms;
    retention removes the race. Covered by an embedded-server integration test
    (in CI) and a live three-process docker run.
- **Crux seam** — contested tangles route to a gemot deliberation (TLS + bearer
  token, refuses anonymous off localhost) or an infra-free inline either/or.
  Validated live against gemot 0.13.1: a decision-rights tangle produced a scored
  crux + binding compromise. gemot poll default 90s → 180s (`--gemot-timeout`)
  after its multi-round analysis outran the old timeout.
- **Safeguards** — `--me` validated against the roster; collected-vs-published
  participant count asserted (no silent partial "all clear"); resolver errors
  surfaced; output-truncation warning; prompt-injection guard in the prompts.
- **Local stack** — `deploy/docker-compose.yml`: NATS (JetStream) + gemot
  (demo mode) in one `docker compose up`, run ettle against it with
  `--insecure-local`.
- **Scaling design** — `docs/SCALING.md`: the anti-runaway firewalls for the
  future continuous loop (L3 emits tangles not atoms; surprise-gated emit; O(1)
  shared reconcile; per-agent budget). The production hook path is gated on them.
- **Project** — MIT LICENSE, SECURITY.md, architecture diagram + example run,
  synthetic fixture, parser/loader + NATS tests, CI (both build configs),
  git-tag-derived `ettle version`.
