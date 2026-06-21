name: priya
role: frontend

Building the new analytics dashboard that surfaces the daily-active-users
numbers. It's a pure consumer of the rollup API — I render whatever the
`/metrics/dau` endpoint returns, with a date picker and a plan-tier filter. No
database access from my side; I never touch the `events` table or any `user_id`
directly, I just display the aggregated counts.

Mostly fighting CSS this week. Hoping the endpoint is stable by Friday so I can
wire the live data in instead of the mock fixtures I'm developing against.
