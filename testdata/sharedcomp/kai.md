name: kai
role: security

Rotating the JWT signing keys in the auth service this week — moving us off the
single long-lived key to a rotating keypair with a 30-day overlap window. It's
entirely internal to the auth service: the token format on the wire doesn't
change, no endpoint changes, callers see nothing different. Pure key-management
hygiene.

I'll deploy the new key issuer behind a flag, verify both keys validate during
the overlap, then retire the old one next month. Nobody else needs to touch
anything.
