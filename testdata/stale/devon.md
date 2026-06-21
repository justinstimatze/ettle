name: devon
role: core-services

Big one this sprint: migrating `events.user_id` off the raw bigint and onto the
new UUID account identifier. The old integer IDs are getting recycled across
tenants and it's started causing cross-account bleed in analytics, so the column
type is changing from bigint to a 36-char string and the values are being
rewritten. About half the writers have cut over already; I'm doing the rest and
the backfill this week.

Anything reading `events.user_id` as an integer or joining it to `users.id`
directly is going to break — the join key moves to `users.account_uuid`. I'll
send a heads-up once the writers are all flipped, probably Wednesday.
