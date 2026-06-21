name: evan
role: team-checkout

Writing a retry wrapper for our outbound HTTP calls this week — the payment
provider flakes intermittently and we keep eating transient 503s. It's a
`retryWithBackoff` helper in `checkout/pkg/httpx`: exponential backoff, jitter,
a max-attempts cap, and respect for `Retry-After`. I'll wrap the provider client
with it.

Pretty general — anyone making flaky outbound calls could use it, but I'm
scoping it to checkout for now to keep the PR small. Landing this week.
