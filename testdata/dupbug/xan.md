name: xan
role: session-restore

Filing a Session Restore bug: from about:sessionrestore after a crash, if I
expand "View Previous Tabs" and double-click (or middle-click) a single tab
entry to restore just that one, I get a blank tab instead of the page. Restoring
the whole session works fine; it's the single-entry click that breaks.

Looks like the per-entry restore isn't carrying the URL through.
