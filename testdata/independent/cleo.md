# Cleo — internal analytics dashboards

Upgrading our Grafana from 9 to 11 for the internal analytics dashboards. The
migration is mostly fixing the panels that use deprecated query syntax against
the metrics database (our Prometheus/Thanos store — not any product database).

Plan to do the cutover during the low-traffic window. It only affects the
internal dashboards the eng team looks at; no customer-facing surface and no
shared code with the product. Should be done whenever I get a clear afternoon —
no hard date.
