name: mara
role: print-ui

Picking up a small polish item on the macOS system print dialog: the "Page
Headers" and "Page Footers" dropdown menus are spaced too tightly together —
they need a little gap between them. Repro is just Command-P, then "Print using
the system dialog…", and look at the header/footer area.

It's purely a layout nit in that one section of the dialog's chrome. Low
priority — I'll tidy the spacing when I'm next in that dialog's CSS.
