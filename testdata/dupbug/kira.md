name: kira
role: linux-widget

Digging into a Wayland drag-and-drop bug on the GTK widget side: a repeated tab
re-order fails. The root cause looks like compositors only expose a single
wl_data_device, so the second drag-and-drop in a session has nothing left to
bind to and silently fails.

I'm prototyping a fix that stops relying on a fresh data device per drag. Should
have a patch up for the GTK widget code this week.
