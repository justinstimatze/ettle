name: iris
role: sre

Adding observability to the auth service — Prometheus metrics on login latency
and failure rate, plus a Grafana dashboard and an alert if the error rate spikes.
It's all instrumentation: a metrics middleware wrapper and a dashboard JSON, no
change to the auth logic or its API.

Once it's live we'll actually be able to see auth health instead of guessing.
Should have the dashboard up by end of week. I'm not modifying any handler
behavior, just measuring it.
