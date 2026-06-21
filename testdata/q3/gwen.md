name: gwen
role: payments

Adding Apple Pay as a checkout option for the Q3 release. It's contained to the
payments service and the web checkout: a new payment-method handler, the Apple
merchant setup, and a button on the existing checkout page. No change to the
order model or anything upstream of payment capture.

Same Q3 freeze as everyone — I'm aiming to be done with a buffer before the
cutoff. The Apple review timeline is my only real risk, and that's external. My
work doesn't touch mobile or search.
