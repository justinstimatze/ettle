name: ivo
role: api-platform

Error format is a platform-wide API contract, so it's owned by API review — and
I've actually been drafting our error standard for the next platform guild
meeting. There are real decisions in it (do we use problem+json or our own
envelope, how we map error codes) that shouldn't be settled service-by-service.

I want anything that changes how services return errors to go through the API
review process first, so we end up with one standard instead of whoever-moved-
first's choice. Putting the draft up for comment this week.
