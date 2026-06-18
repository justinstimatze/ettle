# Dao — CI pipeline, flaky test triage

Triaging the flaky tests that have been failing CI intermittently. Biggest win
this week was turning on the build cache in the CI runners — the dependency
layer is now cached between runs, so the pipeline went from ~12 min to ~5 min.
Still chasing two flakes in the integration suite that look like timing races,
not real failures.

No deadline attached; this is ongoing maintenance. It's all in the CI config
and the test harness — doesn't touch any product code or anyone else's work.
