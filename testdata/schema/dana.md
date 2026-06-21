name: dana
role: analytics

Building a weekly orders report this week — counts by day, revenue, and a
breakdown by order status. It's a read-only query against a replica of the
`orders` table feeding into the BI tool; I create no tables and run no
migrations, just SELECTs.

If the status field shifts I'll adjust the query, but I'm downstream of whatever
the schema ends up being. No write path, nothing for anyone to collide with on my
side. Aiming to have the report live by Friday.
