name: mara
role: platform

Pulling pricing out into its own standalone service (the pricing-extract work).
The new service has soaked well and is healthy, so I've moved the deletion of the
old in-process pricing package up — deleting it THIS Friday, before the freeze
starts, not next week. Want it gone before the migration locks main.
