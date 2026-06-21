# Provenance — duplicate-util corpus

**Synthetic.** Three fictional engineers (evan, fay, glen), authored by hand for
this repo. No real person, project, or data.

## What it tests (a dedicated duplication, with a same-word decoy)

Duplication appears inside `standup-rename` (two caches) but never as the sole
focus. This corpus isolates it on the most common real instance: two engineers on
different teams independently writing the *same general-purpose helper* — an HTTP
retry-with-backoff wrapper — under different names in different repos. Neither
knows about the other; the right outcome is one shared lib, not two.

- **K1 (real).** Evan's `retryWithBackoff` (checkout) and Fay's `Retry`
  (notifications) are the same utility down to the feature list (exponential
  backoff, jitter, max-attempts, Retry-After). Different names, same work.

- **D1 (decoy).** Glen adds CI test-retry in the pipeline YAML. The token "retry"
  matches, but rerunning flaky tests is a different domain entirely — no shared
  code, no duplicated effort. Surfacing it is the over-emit trap of matching on a
  word rather than the work.

## How to read the result

```
go run ./cmd/ettle eval testdata/eval/duplicate-util.json
```

Recall recovers K1; precision holds if Glen (D1) is not firm. The discrimination
is same-work vs same-word — duplication is about the thing being built, not the
vocabulary describing it.
