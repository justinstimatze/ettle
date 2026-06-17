# ettle — benchmark datasets (candidates and the honest method)

Today's eval (`ettle eval`) is a **synthetic smoke test**: a handful of hand-built
scenarios with planted frictions (see [PRIOR_ART.md](PRIOR_ART.md) and the README
status). That is enough to sanity-check the detector and nothing more. Real
validation needs **logged human coordination** — situations where people
coordinated (or failed to), recorded for research. This page catalogs the
candidates, how each maps to ettle's knot kinds, and the honest caveats. None of
the *detection* corpora below is wired up yet; this is a research note, not a claim.

> **One benchmark of a different kind is already built.** The *privacy-boundary*
> claim — that distillation does not leak — is measured today by
> `ettle eval --leak testdata/leak/*.json` (`internal/eval/leak.go`): synthetic
> notes with planted secrets (a comp number, a credential, a medical reason, a
> private opinion), distilled, with the leak rate and a must-cross utility guard
> reported. It follows the operationalized contextual-integrity method
> ([PRIOR_ART.md](PRIOR_ART.md) §2), and it sidesteps the artifacts-vs-reasoning
> mismatch below entirely, because a leak is measurable on a single note with no
> ground-truth coordination outcome required. It is still synthetic and the matcher
> is deliberately liberal; it is a first real number, not a finished benchmark.
>
> **What this leak number does *not* measure (the real privacy property).** The
> leak eval is *per-atom*: it asks whether one distilled note leaks a planted
> secret in isolation. The harder, unmeasured property is *longitudinal* — whether
> a teammate's L2 model, accumulated over *N* rounds of individually-clean atoms,
> lets them reconstruct a fact no single atom ever stated ("out Tuesday" + "pairing
> to hand off auth" + "not taking the Q3 roadmap" ⇒ attrition). A real metric would
> run a multi-round scenario, accumulate the crossed atoms into a per-recipient
> model, and ask a probe model to reconstruct a held-out secret from the
> *aggregate*, scoring reconstruction confidence rather than substring presence.
> That is a stub, not built — it belongs with the unbuilt calibration loop, and
> until it exists the 0% leak rate is an honest per-atom number, not a
> whole-boundary guarantee (see [SECURITY.md](../SECURITY.md)).

## The core mismatch (read this first)

ettle's whole thesis is that the signal is **reasoning-in-progress**, not
after-the-fact artifacts. But almost every public coordination dataset *is* an
artifact — an email thread, a resolved issue, a meeting transcript. So a real
benchmark is necessarily **retrospective**:

> Take a *documented* coordination outcome (a known duplicate pair, a coordination
> gap that caused a defect, a decision reached in a meeting), reconstruct the
> state *before* it resolved, feed that to ettle, and ask: would it have surfaced
> the knot ahead of time?

This tests the detector against real human coordination, but it tests it on the
wrong *form* of input (artifacts, not live sessions). Meeting corpora are the
closest to live reasoning; issue/email corpora are the furthest but carry the
clearest ground truth. Be explicit about which gap any given benchmark closes.

## Candidates, by knot kind

| Dataset | Knot kind it tests | Availability | Notes / caveats |
|---|---|---|---|
| **Duplicate bug reports** — Lazar et al. (Eclipse, Mozilla, NetBeans, OpenOffice); [GitBugs](https://arxiv.org/html/2504.09651) (~150k reports across GitHub/Jira/Bugzilla) | **duplication** | Public, labeled | The cleanest, most tractable real benchmark: labeled duplicate pairs map 1:1 to ettle's duplication knot and are directly scoreable. **Wired** from the Mozilla Bugzilla REST API — see [First step](#first-step--done). |
| **Socio-technical congruence** — [Cataldo & Herbsleb](https://herbsleb.org/web-pubs/pdfs/Cataldo-Coordination-2013.pdf) (CSCW'06; coordination-breakdown studies) | **collision / decision-rights** | Method + archival repo data | This is the academic measurement of ettle's *exact* thesis: coordination *requirements* (who must coordinate, derived from task dependencies) vs. actual communication → gaps predict defects and delay. A coordination gap that caused a documented defect is a real, un-seeded collision knot. |
| **[AMI](https://groups.inf.ed.ac.uk/ami/corpus/) + [ICSI](https://groups.inf.ed.ac.uk/ami/icsi/) meeting corpora** | **teamwide-divergence / decisions** | Public, CC BY 4.0 | Closest thing to multi-party reasoning-in-progress, with dialogue-act, decision, and action-item annotations. Caveats: these are *spoken meetings* (ettle's pitch is "no meeting"); AMI is partly scenario-scripted (role-played design tasks); action-item annotation is subjective (ICSI inter-rater κ≈0.36). |
| **[DeliData](https://arxiv.org/abs/2108.05271)** (500 group dialogues, 14k utterances, Wason task) | **crux / consensus** | Public ([delibot.xyz](https://delibot.xyz)) | Groups deliberating to consensus, annotated with deliberation cues. Maps to the contested-knot → crux path, not to cross-person collision detection. |
| **Enron email corpus** (prepared/released by the **CALO** project at SRI; ~500k emails, ~150 users incl. senior management) | real hierarchical org coordination, at scale | Public, free | The canonical real-org corpus — execs with reports coordinating. *Unlabeled* for "knots," so it needs annotation before it's a benchmark; aging (some argue it's overused). A CALO byproduct — see [PRIOR_ART.md](PRIOR_ART.md). |
| **[Avocado Research Email Collection](https://catalog.ldc.upenn.edu/LDC2015T03)** (LDC2015T03; 938k emails + 27k meeting schedules + 326k attachments; 279 accounts of a defunct IT company, anonymized) | real org coordination + scheduling | LDC-licensed (two agreements; restricted/costly) | Richer than Enron (emails *and* meeting schedules), already anonymized — but the license makes it a poor fit for an open repo's reproducible benchmark. |
| **Government FOIA email dumps** — Clinton State Dept (FOIA reading room), [Jeb Bush gubernatorial](https://www.politifact.com/factchecks/2015/aug/31/jeb-bush/jeb-bush-says-he-has-released-all-his-emails/) (~280k) | hierarchical principal/staff coordination | Public records | Hierarchy is present, but the data is redacted, politically charged, and messy — high annotation cost, low signal-to-noise for coordination knots. |

(The **CALO Meeting Assistant** also produced a multimodal meeting corpus, but its
current public availability is unclear; AMI/ICSI are the clean public meeting bet.)

## Honest caveats for any of these

- **Artifacts ≠ reasoning-in-progress.** Every dataset here is downstream of the
  live signal ettle claims to use. A good score on retrospective artifacts does
  *not* prove the in-session capture path works; it proves the detector can find a
  documented knot when handed the surrounding text.
- **Survivorship / selection bias.** Logs capture coordination that *broke* and
  got recorded (a shipped duplicate, a post-mortem'd defect). The coordination
  that silently *succeeded* — the thing ettle most wants to claim it enables — is
  largely unlogged. So these benchmarks measure recall on failures, not the value
  of friction avoided.
- **Post-hoc labels.** "This was a duplicate" / "this decision was made" are
  annotated after the fact, sometimes with low inter-rater agreement (AMI/ICSI
  action items). Treat label quality as a variable, not ground truth.
- **The privacy irony.** ettle is a *privacy-boundary* tool. Validating it on real
  people's leaked private email (Enron exists because of litigation; FOIA dumps
  because of records law) is ethically pointed. Prefer the labeled-technical sets
  (duplicate bugs, STC repo data) and the consented/CC-licensed research corpora
  (AMI, ICSI, DeliData) for anything published; treat the leak corpora as, at
  most, internal exploration, anonymized and never re-published.

## First step — done

The **duplicate bug report** corpus is wired:
[`testdata/dupbug/`](../testdata/dupbug/) (provenance in
[`PROVENANCE.md`](../testdata/dupbug/PROVENANCE.md)). It pulls confirmed
`RESOLVED DUPLICATE` pairs from the **public Mozilla Bugzilla REST API**,
anonymizes and rewords each into a standup-style note, and curates **eight real
duplication knots across three corpora** plus one surface-similar distractor (a
cosmetic print-dialog bug that must *not* fuse into the print-broken pair). Run
it:

```
ettle eval --ab --model claude-sonnet-4-6 testdata/dupbug/*.json
```

Result (sonnet): **single-shot recall 8/8** — every real duplication surfaces,
including the hard *root-cause-vs-symptom* pairs where the two reporters describe
the same bug in different words (a `libfontconfig` crash signature vs "googling a
font crashes the tab"; a GTK default-action regression vs "Enter does nothing in
the save dialog") — exactly what a verbatim matcher misses. Precision is high
(one extra firm coordination knot that isn't one of the curated *duplication*
labels — the honest cost of a duplication-focused label set, not a detector
error), and the distractor stays out of the firm duplication. These are **counts
on a tiny corpus, not a precision/recall measurement**: at 8 positive labels a
Wilson 95% interval on recall=1.0 has a lower bound near 0.63, so "8/8" is an
encouraging anecdote with a wide true-rate band, not a benchmarked accuracy
number (the banner's "not validated" caveat is the operative framing).

The **A/B is reported as underpowered, with the honest reason stated**: across
the 8 labels, single-shot (8/8) and 3-sample voting (7/8) disagree on only one,
so the McNemar discordance (N=1) is too small to test — `p=1.000, no claim`.
That is *not* a sample-count problem. There is also a **structural ceiling**:
single-shot already scores 8/8, so voting's recall can only match or fall below
it — the "voted-only win" cell of the McNemar table is pinned at zero by the data,
not by chance. The test can therefore detect *voting hurts* but is blind to
*voting helps*, which is the direction the A/B most wants to probe; demonstrating
the helpful direction needs a corpus with headroom (single-shot below ceiling),
i.e. the borderline corpus noted below. Voting exists to damp the detector's
run-to-run noise; on clear-cut duplications the detector is already confident, so
both conditions recover the same knots and there is nothing for voting to fix. An
earlier single-corpus run where voting dropped a real duplication did **not**
replicate at this scale — it was one stochastic draw, not an effect. Voting would
only earn its cost on *borderline* knots where the detector wobbles, which this
corpus deliberately does not contain; measuring that is the natural next corpus.
This remains a **retrospective artifact test** (see the caveat above), not
validation of the live capture path.

Everything else (AMI decision detection, an STC retrospective, Enron annotation)
is a larger build.
