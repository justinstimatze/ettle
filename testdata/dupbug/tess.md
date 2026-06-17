name: tess
role: gtk-widget

Looking at a GTK file-dialog regression in 152: the file picker doesn't have
Open/Save wired as its default action. With the portal picker disabled and "ask
where to save every time" on, pressing Ctrl+S and then Enter in the filename
field does nothing — Enter isn't bound to the save action anymore.

Investigating the GTK widget side to restore the default-action binding.
