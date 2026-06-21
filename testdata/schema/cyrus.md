name: cyrus
role: fulfillment

Cleaning up the orders schema — the `orders.state` column is a free-text string
and it's a mess, so I'm writing migration `0041_rename_state` that renames it to
`status` and tightens it to an enum. Fulfillment reads it to decide what to pick
and pack, so I'm updating those queries to the new name in the same PR.

Both the migration and the fulfillment query changes are ready; I want to land
the rename before anything else stacks on the old `state` column. Targeting
Wednesday.
