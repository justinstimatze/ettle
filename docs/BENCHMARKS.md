# ettle — benchmark datasets (candidates and the honest method)

Today's eval (`ettle eval`) is a **synthetic smoke test**: a handful of hand-built
scenarios with planted frictions (see [PRIOR_ART.md](PRIOR_ART.md) and the README
status). That is enough to sanity-check the detector and nothing more. Real
validation needs **logged human coordination** — situations where people
coordinated (or failed to), recorded for research. This page catalogs the
candidates, how each maps to ettle's knot kinds, and the honest caveats. None is
wired up yet; this is a research note, not a claim.

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
| **Duplicate bug reports** — Lazar et al. (Eclipse, Mozilla, NetBeans, OpenOffice); [GitBugs](https://arxiv.org/html/2504.09651) (~150k reports across GitHub/Jira/Bugzilla) | **duplication** | Public, labeled | The cleanest, most tractable real benchmark: labeled duplicate pairs map 1:1 to ettle's duplication knot and are directly scoreable. **Start here.** |
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

## First step

Wire the **duplicate bug report** pairs as a real `ettle eval` corpus for the
duplication knot — it is public, labeled, maps 1:1, and is directly scoreable,
so it converts the weakest claim (synthetic-only validation) into a real one for
at least one knot kind with the least work. Everything else (AMI decision
detection, an STC retrospective, Enron annotation) is a larger build.
