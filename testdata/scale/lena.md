name: lena
role: platform

Cleaning up the gateway routing this sprint — `gateway/routes.yaml` has grown a
mess of overlapping `location` blocks and the match order is doing the wrong
thing for a couple of paths. I'm reordering the blocks so the most specific
paths match first and collapsing three near-duplicate `/api/` entries into one.

It's a big reshuffle of that one file, so I want to land it in a single clean PR
before anyone else stacks changes on top of the old ordering. Aiming for
Wednesday.
