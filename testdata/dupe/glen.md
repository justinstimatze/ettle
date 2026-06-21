name: glen
role: platform-ci

Fighting flaky CI this week. A handful of integration tests fail intermittently
and block merges, so I'm adding automatic test retries in the CI config — a
failed test reruns up to twice before it's marked red, and I'm tagging the worst
offenders to fix properly later.

It's purely a CI pipeline change (the GitHub Actions YAML and a small wrapper
script); no application code, no HTTP, no shared library. Just trying to stop
the merge queue from jamming on known-flaky tests.
