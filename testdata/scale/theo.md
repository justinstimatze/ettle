name: theo
role: backend

Refactoring the logging helper so we emit structured JSON instead of the
free-text lines we have now. It's a single internal package — `internal/log` —
and the call sites just keep using the same `log.Info` signature, so it's a
contained change. Downstream, our log shipper gets nicer fields to index on.

I'll add a `trace_id` field while I'm in there since it's cheap. Targeting a
review by Thursday. Nothing else in my lane touches anyone's routes or configs.
