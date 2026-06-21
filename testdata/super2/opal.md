name: opal
role: data-engineering

Building the ETL that lands raw analytics events into the warehouse this week —
a streaming consumer that reads the events topic, cleans the records, and writes
partitioned tables for downstream querying. It's pipeline and warehouse work; no
UI, no API surface.

Once it's flowing, the metrics service can query clean tables instead of raw
logs. Setting up the job and the schema this week, backfill after.
