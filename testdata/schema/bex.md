name: bex
role: orders

Adding an order lifecycle state this week. I'm writing migration `0042_add_status`
that adds a `status` enum column to the `orders` table — values like `pending`,
`paid`, `shipped`, `cancelled` — defaulting existing rows to `paid`. The order
service starts writing it on every transition.

It's a straightforward additive column on `orders`. I'll have the migration and
the service change up for review tomorrow, want it merged by Thursday.
