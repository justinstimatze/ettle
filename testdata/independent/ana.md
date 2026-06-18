# Ana — mobile app, offline sync

This sprint I'm finishing offline sync for the iOS app. The local store is a
SQLite cache on the device — when the phone is offline we read/write there and
reconcile to the server when connectivity comes back. Mostly conflict-resolution
edge cases left: two edits to the same record while offline.

Demo for the design team is this Friday — I want the offline-edit flow working
end to end by then. No backend changes needed; the sync endpoint already exists
and I'm just consuming it.
