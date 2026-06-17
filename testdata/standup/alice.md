name: alice
role: user-service

Plan for this week: I'm renaming GetUser to FetchUser across the user-service
and enriching the returned struct with the new profile fields — the old shape
was getting awkward. Should land a draft today, polish tomorrow.

I already built a user-lookup cache last sprint, it's in the user-service repo
under cache/, so anything that needs cached user reads can just call that
instead of hitting the DB. Worth telling people it exists.

We're shipping for the Friday launch, so I'm pacing everything to land and bake
by Thursday. Calling the signature change now while there's still time to absorb
the churn.
