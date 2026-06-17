name: luca
role: linux-widget

Filing what looks like a recent regression: under Wayland I can't rearrange tabs
by dragging them with the mouse anymore — no matter the kernel, the profile, or
the system user. Horizontal tab layout. Under forced XWayland the drag works but
the cursor shows a "not available" icon. Moving tabs with keyboard shortcuts and
dragging a tab out into a new window both still work — it's specifically
mouse-drag re-ordering within the tab strip that's dead.

mozregression narrows it to a single autoland changeset. Expected tabs to move
freely when dragged. Going to keep narrowing unless someone's already on it.
