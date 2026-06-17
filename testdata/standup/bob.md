name: bob
role: billing

Wiring up the new billing reconciliation job. It walks every active account and
calls GetUser in a tight loop to pull the email + plan for each one. The
signature's stable so I'm coding straight against it — no wrapper, just the
direct call, keeps it fast.

Assuming I've got until the end of next week for this; the public launch date is
the deadline I'm working back from and that gives me comfortable runway.

Blocked on one thing: I need confirmation on whether we're going with provider-A
or provider-B for the payment integration before I can finalize the webhook
handler. Whoever owns that call, I need it soon.
