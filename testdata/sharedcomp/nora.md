name: nora
role: technical-writer

Documenting the auth service endpoints this week — writing the public reference
for `/login`, `/refresh`, and `/logout`, with request/response examples and the
error table. It's prose in the `docs/` site; I read the handlers to get the
shapes right but I don't change any code.

The goal is that an integrating team can wire up auth without reading the source.
No deadline pressure, just want it accurate. Pinging people only to confirm I've
described the flows correctly.
