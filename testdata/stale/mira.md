name: mira
role: data-platform

Shipping the new daily-active-users rollup this week. It reads straight off the
`events` table and groups by `user_id` — I'm treating `user_id` as a stable
bigint key, joining it back to `users.id` to attach plan tier. The aggregation
job is written and passing on last week's snapshot; I just need to point it at
prod and schedule it for Thursday.

Once it's landed I'll backfill ninety days so the dashboard has history on day
one. Not touching anything else in the pipeline — this is a read-only consumer
of `events`, so it should be a clean drop-in.
