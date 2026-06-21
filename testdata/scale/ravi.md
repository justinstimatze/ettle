name: ravi
role: platform

Adding per-tenant rate limiting at the edge this week. I'm editing the gateway
config — `gateway/routes.yaml` — to insert a `rate_limit` block on the
`/api/*` location and wire it to the new Redis counter. It's a focused change:
one new stanza in the location block, plus the Redis connection settings at the
top of the file.

Should be a quick PR. The only fiddly part is ordering — the rate-limit stanza
has to sit before the upstream proxy_pass or it won't apply. Landing it Tuesday.
