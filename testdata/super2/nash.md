name: nash
role: frontend

Reworking the billing settings screen — letting users update their card, see
invoices, and change plan without contacting support. It's a frontend change
against the existing billing API; I add a couple of views and some form
validation. No API changes needed, the endpoints already exist.

Mostly a UX cleanup of a page everyone complains about. Targeting a review by end
of week.
