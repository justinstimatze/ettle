name: hana
role: backend

I'm standardizing how all our services return errors — moving everyone onto
RFC 7807 `application/problem+json` with a consistent error envelope. I've got
the shared middleware written and I'm starting to roll it out across the
services this week, beginning with orders and billing.

This has bugged me for ages — every service invents its own error shape and
clients hate it. I'm just going to drive it through and convert services one by
one. Should have the first few done by end of week.
