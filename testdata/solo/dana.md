name: dana
role: payments

Working solo on the payments rewrite this week — no one else is in this part of
the code, so this is just my own plan.

Early in the week I sketched the refund flow assuming the billing service keeps
its synchronous `POST /charge` contract — the whole refund path calls it inline
and waits for the result, because that's how it's always worked. The retry logic
and the user-facing "refund complete" message both depend on getting an answer
back in the same request.

Then on Wednesday I decided to move billing behind the new async event queue:
charges get enqueued and a webhook confirms them later. It's the right call —
the synchronous path was timing out under load.

Still need to ship the refund flow by Friday. Polishing the retry logic now.
