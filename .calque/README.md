# .calque — the prose vocabulary gate

`make ci` runs [`calque vocab-check`](https://github.com/justinstimatze/calque),
which fails when a compound term appears often enough to be load-bearing but is
not in `vocab-allowlist.txt`. It exists because of a specific failure, not as
general tidiness.

**What happened.** ettle's unit of coordination was renamed from *knot* to
**tangle**, and `cbedf79` removed the old word from the tree entirely — no alias,
no two-vocabulary changelog. Months later a session put "knot" back 234 times
across five commits: a whole package, a transport method, the block injected into
every session, the README. Nothing in the build noticed, because a renamed concept
compiles perfectly under either name. The reader is the only thing that breaks, and
they break silently, by being confused about whether a knot and a tangle are two
things.

**How to use it.** A new warning is a question, not a verdict: either the term is
real house vocabulary, in which case add the slug to `vocab-allowlist.txt`, or it is
drift — a second word for something already named — in which case fix the prose. Do
not add a slug to silence a gate you have not read.

Re-seed the list from scratch with `calque vocab-check --bootstrap >
.calque/vocab-allowlist.txt`, and check what a term costs before adding it with
`calque vocab-report`. The gate skips itself when calque is not installed, so it
never blocks a contributor who does not have it.
