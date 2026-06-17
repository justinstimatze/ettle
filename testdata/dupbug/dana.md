name: dana
role: mac-printing

Filing a regression I hit this week: on macOS, printing through the system print
dialog stopped working in 152. If I click "Print using the system dialog" — or
set print.prefer_system_dialog to true — and try to produce a PDF, nothing
happens at all: no file is printed, nothing is saved. I expected a PDF and got
nothing.

Confirmed it on a fresh profile and on two separate machines, so it isn't my
local setup. This is new in 152. Going to bisect the printing path next.
