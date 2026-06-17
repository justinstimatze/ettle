name: carol
role: notifications

Building out the notifications service this week. To avoid hammering the
user-service on every send, I'm going to stand up a user-lookup cache in the
notifications repo — keep a local copy of the fields we need keyed by user id.

Should have the cache layer done in a couple of days. I'll own it since it's in
my repo, and it can be the shared cache other services use too — seems silly for
everyone to roll their own.

Working toward the Friday launch like everyone else. The only open dependency is
the safety review call, which I think is still unscheduled — not blocking me yet
but it's on the critical path for the launch sign-off.
