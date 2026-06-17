name: noor
role: graphics-text

Tracking a spike of Linux crashes since the fontconfig 2.18.0 upgrade landed in
151 — a couple hundred crash reports across dozens of installs in two days, all
landing in the font code (libfontconfig). Looks like something in how we hand
fonts to fontconfig broke with the new version.

Digging into the crash signature now to find what changed in the font path.
