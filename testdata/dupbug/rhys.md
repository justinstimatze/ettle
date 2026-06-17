name: rhys
role: ui-widgets

Chasing a widget bug: a moz-button with an accesskey gets its visible label
corrupted when its data-l10n-id updates at runtime. When we toggle a button
between localized states, the rendered label text comes out garbled instead of
re-localizing cleanly.

Working the accesskey + l10n update path to see where the label gets mangled.
