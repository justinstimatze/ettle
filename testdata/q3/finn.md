name: finn
role: mobile

Landing the new push-notification opt-in flow for the Q3 release. It's a mobile
client change: a permission-priming screen, the OS prompt, and storing the
choice. Backend already has the notification endpoint, so I'm only touching the
iOS and Android apps.

Q3 cutoff is the big driver — everything for the release has to be in by the
freeze. I'm pacing to land mine with a week of buffer for QA. Nothing of mine
reaches outside the mobile apps.
