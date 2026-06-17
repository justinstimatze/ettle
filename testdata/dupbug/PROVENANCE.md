# duplicate-bugs corpus — provenance

The first **real-data** `ettle eval` corpora (everything else under `testdata/`
is synthetic). They test the **duplication** knot: two people independently
reporting the same underlying bug should reconcile into one duplication knot,
while a surface-similar but substantively different bug should *not*.

Three corpus files, **eight real duplicate pairs** total plus one distractor:

- `duplicate-bugs.json` — 2 pairs (print-to-PDF, Wayland tabs) + the `mara`
  distractor. The showcase corpus.
- `duplicate-bugs-rootcause.json` — 3 pairs chosen for *root-cause-vs-symptom*
  divergence (the two reporters describe the same bug in very different words).
- `duplicate-bugs-closeworded.json` — 3 pairs worded more similarly.

Run all three pooled: `ettle eval --ab --model claude-sonnet-4-6
testdata/dupbug/*.json` (the A/B McNemar pools discordant pairs across corpora).

## Source

[Mozilla Bugzilla](https://bugzilla.mozilla.org) — pulled from the public REST
API (`/rest/bug`), which is built for programmatic access; the bug records used
here are publicly visible (no security-restricted bugs). The ground-truth
"duplicate" relationship is Bugzilla's own `RESOLVED DUPLICATE` / `dupe_of`
link — a real triager's adjudication, not one we invented.

## What was done to the data

Each source bug became one **anonymized, reworded** standup-style note:

- Reporter identities removed and replaced with invented first names (dana,
  evan, kira, luca, mara). The notes are *not* verbatim bug text — they are
  paraphrases into first-person "here's what I'm working on" form, trimmed to
  the coordination-relevant content (user-agent strings, attachments, and
  changeset hashes dropped).
- Only the derived notes are committed. The raw API responses live in the
  gitignored `local/dupbug/` cache; raw third-party bug text is not committed.

These are small, transformed excerpts of public records used as a research
benchmark fixture.

## Mapping (note → source bug)

| Note     | Source bug | Summary                                                          | Relationship |
|----------|------------|------------------------------------------------------------------|--------------|
| dana.md  | 2047850    | macOS system print dialog produces no PDF (FF 152)               | **K1 pair** — canonical |
| evan.md  | 2048130    | dup_of 2047850 — system-dialog PDF options route to a printer    | **K1 pair** — duplicate |
| kira.md  | 2046911    | Wayland tab D&D re-order fails (root cause: one `wl_data_device`) | **K2 pair** — canonical |
| luca.md  | 2047217    | dup_of 2046911 — "cannot rearrange tabs on Wayland" (symptom)    | **K2 pair** — duplicate |
| mara.md  | 2026024    | macOS print-dialog header/footer spacing (cosmetic CSS nit)      | **distractor** — same UI surface, different bug |

`duplicate-bugs-rootcause.json` (root-cause vs symptom):

| Note     | Source bug | Summary                                                          | Relationship |
|----------|------------|------------------------------------------------------------------|--------------|
| noor.md  | 2041887    | crash in `libfontconfig` after the fontconfig 2.18.0 upgrade     | **W1 pair** — canonical (root cause) |
| petra.md | 2045589    | dup_of 2041887 — "googling 'calibri font' crashes the tab"       | **W1 pair** — duplicate (symptom) |
| rhys.md  | 2022326    | `moz-button` label corrupts when its `l10n-id` updates at runtime | **W2 pair** — canonical (root cause) |
| sol.md   | 2047065    | dup_of 2022326 — "broken button label after switching languages" | **W2 pair** — duplicate (symptom) |
| tess.md  | 2047865    | GTK file picker has no Open/Save default action (FF 152)          | **W3 pair** — canonical (root cause) |
| umi.md   | 2048141    | dup_of 2047865 — "Enter does nothing in the save dialog"          | **W3 pair** — duplicate (symptom) |

`duplicate-bugs-closeworded.json` (more similarly worded):

| Note     | Source bug | Summary                                                          | Relationship |
|----------|------------|------------------------------------------------------------------|--------------|
| vik.md   | 2016945    | Firefox plays YouTube video itself instead of deferring          | **Y1 pair** — canonical |
| wren.md  | 2045449    | dup_of 2016945 — "YouTube ignores autoplay setting"              | **Y1 pair** — duplicate |
| xan.md   | 2047195    | restoring a single Session-Restore entry opens a blank tab       | **Y2 pair** — canonical |
| yara.md  | 2047469    | dup_of 2047195 — "middle-click from session restore → blank tabs" | **Y2 pair** — duplicate |
| zane.md  | 2001667    | settings search highlight tooltips are misplaced                 | **Y3 pair** — canonical |
| ora.md   | 2046340    | dup_of 2001667 — "find-in-page in settings points to nonsense"   | **Y3 pair** — duplicate |

(Bugs are at `bugzilla.mozilla.org/show_bug.cgi?id=<id>`; raw API responses are
cached gitignored under `local/dupbug/`. The *root-cause-vs-symptom* pairs — K2,
W1, W2, W3 — are the pedagogically interesting ones: one reporter frames the root
cause, the other the user-visible symptom, the same bug in words a verbatim
matcher would miss and the detector catches. Two candidate pairs were *dropped*
during curation because the triager's `dupe_of` link joined genuinely different
symptoms — a questionable label is not laundered into ground truth.)

The **distractor** (mara) shares heavy surface vocabulary with the K1 pair —
"macOS", "Print using the system dialog…" — but is a cosmetic spacing bug, not
the print-broken regression. Correct behaviour is to *not* fuse it into the K1
duplication. In runs to date the detector keeps it out of the firm duplication
and at most raises it as a soft "worth a question" divergence.

## Honest caveat (the documented core mismatch)

These are **artifacts, not reasoning-in-progress**. A good score here shows the
detector can recover a *documented* duplicate from the surrounding text — it does
**not** validate the live in-session capture path ettle's thesis rests on. Treat
this as a retrospective detector test, not a thesis validation. See
[../../docs/BENCHMARKS.md](../../docs/BENCHMARKS.md) ("Artifacts ≠
reasoning-in-progress").
