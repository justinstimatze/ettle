name: jana
role: backend

Heads-down on the orders export feature this week — generating CSV and Parquet
dumps of completed orders for the data team to ingest. It's a new endpoint and a
background job; contained to the orders service.

One thing I keep going back and forth on for myself: whether to stream the export
or build it in memory and upload. Leaning streaming for the big tenants. That's
my own call to make inside this feature, not blocking on anyone. Targeting a demo
Friday.
