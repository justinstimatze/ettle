name: hugo
role: search

Tuning search relevance for the Q3 release — reweighting the ranking signals so
recent and in-stock items rank higher, plus a synonym list for the top failed
queries. It's all inside the search service: the ranking config and an offline
eval to make sure I don't regress. No schema or API changes; the response shape
is identical.

Targeting the Q3 freeze like everyone. The work is self-contained to search; I'm
not blocked on and not blocking anyone. Just need enough time to run the
relevance eval before cutoff.
