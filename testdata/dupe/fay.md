name: fay
role: team-notifications

Our webhook delivery keeps failing on transient network blips, so I'm building a
retry helper with exponential backoff and jitter — `Retry` in
`notifications/internal/net`, with a max-attempts cap and `Retry-After` handling.
It'll wrap the HTTP client we use to POST to subscriber endpoints.

It's the kind of thing that probably should live in a shared lib eventually, but
I need it now so I'm putting it in the notifications repo. Should be done in a
couple of days.
