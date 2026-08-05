# .calque — the prose vocabulary gate

`make ci` runs [`calque vocab-check`](https://github.com/justinstimatze/calque),
which fails when a compound term appears often enough to be load-bearing but is not
in `vocab-allowlist.txt`.

A warning is a question, not a verdict: either the term is real house vocabulary, in
which case add the slug to the list, or it is drift — a second word for something
already named — in which case fix the prose. Don't add a slug to silence a gate you
haven't read.

Re-seed the list with `calque vocab-check --bootstrap > .calque/vocab-allowlist.txt`;
`calque vocab-report` shows what a term costs before you add it. The gate skips itself
when calque isn't installed, so it never blocks a contributor without it.

Scope worth knowing: calque reads prose with fenced and inline code stripped, so it
sees neither identifiers nor `code` spans.
