name: evan
role: printing-output

Chasing a macOS printing problem on 152. When I print a web page and choose
"Print using system dialog…", then pick one of the PDF entries in the macOS
dialog's dropdown, Firefox ignores it and sends the job to a physical printer
instead of running any of the PDF workflows (Open in Preview, Save as PDF, Save
as PostScript). The in-app "Save to PDF" path still works fine — it's
specifically the system-dialog PDF options that are broken.

Planning to dig into how we hand the job to the macOS print system. Whoever owns
the Mac print path, ping me, I don't want to duplicate the investigation.
