name: reed
role: data-engineering

Modeling revenue in the warehouse — building the `fct_revenue` table that joins
charges to plans and tenants so finance can self-serve. It's dbt models and tests
against warehouse tables already populated by the billing export. No application
code, no frontend.

The goal is one trusted revenue number instead of three spreadsheets. Getting the
core model and tests in this week, then I'll add the finance-facing views.
