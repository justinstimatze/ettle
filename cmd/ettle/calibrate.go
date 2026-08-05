package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/justinstimatze/ettle/internal/calib"
	"github.com/justinstimatze/ettle/internal/mcpserver"
)

// `ettle calibrate` reads the verdict log and says what it supports. It writes
// nothing — not the log, not the cut points in mesh.go. See internal/calib for why
// the read and the write are separated on purpose rather than for now.
//
// The output most installs will get is "no verdicts yet", and that is the useful
// answer: it makes the accrual visible instead of leaving the loop's input to be
// assumed. The second-most-common output is a kind blocked for want of a `real`
// verdict, which is the one-sidedness of the ask showing up as a number.

func runCalibrate(args []string) error {
	fs := flag.NewFlagSet("calibrate", flag.ContinueOnError)
	path := fs.String("labels", "", "verdict log to read (default: $ETTLE_LABELS_PATH, else ./ettle-labels.jsonl)")
	asJSON := fs.Bool("json", false, "emit the report as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p := *path
	if p == "" {
		p = mcpserver.LabelsPath()
	}

	rep, err := calib.Read(p)
	if err != nil {
		return err
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	fmt.Print(renderCalibration(rep))
	return nil
}

func renderCalibration(rep *calib.Report) string {
	var b []byte
	out := func(format string, a ...any) { b = append(b, fmt.Sprintf(format, a...)...) }

	out("verdict log: %s\n", rep.Path)
	if rep.Rows == 0 {
		out("\nno verdicts recorded yet.\n\n")
		out("Verdicts accrue when someone answers a surfaced tangle — `ettle_respond`\n")
		out("from an agent session, or `ettle mute --wrong` / `--handled` from a shell.\n")
		out("Until then there is nothing to calibrate against, and the cut points in\n")
		out("internal/ettlemesh/mesh.go stand as the diagnostic batch set them.\n")
		out("%s", structuralNote(rep))
		return string(b)
	}

	out("%d row(s)", rep.Rows)
	if rep.Malformed > 0 {
		out(", %d unparseable and skipped", rep.Malformed)
	}
	out("\n")

	for _, k := range rep.Kinds {
		out("\n  %s — firm bar %.2f\n", k.Kind, k.FirmBar)
		out("    real %d · not_real %d · handled %d   (%d of %d carry recurrence)\n",
			k.Real, k.NotReal, k.Handled, k.WithRecurrence, k.Rows)
		for _, bk := range k.Buckets {
			out("      recurrence %.1f–%.1f: real %d · not_real %d · handled %d\n",
				bk.Lo, bk.Hi, bk.Real, bk.NotReal, bk.Handled)
		}
		if k.Blocked != "" {
			out("    ✗ %s\n", k.Blocked)
		} else {
			out("    ✓ both arms present with recurrence — readable; the bar is still yours to move, in mesh.go\n")
		}
	}
	out("%s", structuralNote(rep))
	return string(b)
}

// structuralNote prints the two limits that hold no matter how much data arrives.
// It is unconditional on purpose: they are properties of what gets recorded, so a
// full log is exactly as subject to them as an empty one, and a reader who only
// ever sees this page should not have to go find that out.
func structuralNote(rep *calib.Report) string {
	return fmt.Sprintf(`
Two limits, independent of how many rows accrue:

  · Confirming a good tangle changes nothing the human sees, so `+"`real`"+` verdicts
    are recorded by choice while `+"`not_real`"+` ones are recorded to stop a nuisance.
    Expect the log to lean negative, and read a missing `+"`real`"+` arm as the ask
    being one-sided rather than as the detector being wrong.

  · Nothing below the drop floor (%.2f recurrence) is ever surfaced, so no verdict
    about it can exist. These rows can show the floor is too low. Nothing here can
    show it is too high — a real tangle the floor killed leaves no trace.

Below %d rows for a kind this declines to characterise it. That number is a floor,
not a power calculation.
`, rep.DropFloor, rep.MinPerKind)
}
