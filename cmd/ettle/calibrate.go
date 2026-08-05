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
		out("from an agent session, or `ettle confirm` / `ettle mute --wrong` /\n")
		out("`--handled` from a shell.\n")
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
			continue
		}
		out("%s", renderSuggestion(k))
	}
	out("%s", structuralNote(rep))
	return string(b)
}

// renderSuggestion says where the evidence puts the cut and what moving there would
// cost, in the two currencies a person actually trades between: false alarms on the
// horizon, and real tangles demoted to a question.
//
// It reports an interval because ties are the normal case on discrete recurrence, and
// it never phrases the result as an instruction. Which end of a tie to take depends
// on whether a false alarm or a miss is worse for this team, and that is theirs.
func renderSuggestion(k calib.KindReport) string {
	s := k.Suggest
	if s == nil {
		return "    ✓ readable, but no labelled recurrences to sweep\n"
	}
	var b []byte
	out := func(format string, a ...any) { b = append(b, fmt.Sprintf(format, a...)...) }

	// No cut beats chance: the verdicts and the recurrence are unrelated for this
	// kind. There is still a best-scoring cut — there always is — and printing it
	// would dress a coin flip as a measurement.
	if !s.Separates {
		out("    ✗ recurrence does not separate these verdicts (best Youden's J %.2f, chance is 0.00)\n", s.J)
		out("      real and not_real are recorded at the same recurrences, so no cut point\n")
		out("      does better than asserting everything. Either the signal is wrong for this\n")
		out("      kind, or something other than recurrence is deciding — neither is fixed by\n")
		out("      moving the bar.\n")
		return string(b)
	}

	if s.Lo == s.Hi {
		out("    → the labelled rows separate best at %.2f", s.Lo)
	} else {
		out("    → every cut from %.2f to %.2f separates these rows equally well", s.Lo, s.Hi)
	}
	out(" (Youden's J %.2f)\n", s.J)
	out("      at that cut:  %d asserted right, %d asserted wrong, %d real demoted to soft\n",
		s.AtCut.TP, s.AtCut.FP, s.AtCut.FN)
	if s.SameAsCurrent {
		out("      the bar in force (%.2f) splits these rows identically — nothing here argues for moving it.\n", k.FirmBar)
		return string(b)
	}
	out("      at %.2f today: %d asserted right, %d asserted wrong, %d real demoted to soft\n",
		k.FirmBar, s.AtCurrent.TP, s.AtCurrent.FP, s.AtCurrent.FN)
	out("      whether to move is a trade: a lower bar asserts more and is wrong more often.\n")
	out("      the constant is firmVoteFractionByKind in internal/ettlemesh/mesh.go.\n")
	return string(b)
}

// structuralNote prints the two limits that hold no matter how much data arrives.
// It is unconditional on purpose: they are properties of what gets recorded, so a
// full log is exactly as subject to them as an empty one, and a reader who only
// ever sees this page should not have to go find that out.
func structuralNote(rep *calib.Report) string {
	return fmt.Sprintf(`
Two limits, independent of how many rows accrue:

  · The payoffs are not equal: muting ends a nuisance, confirming ends a question.
    Expect fewer `+"`real`"+` rows than there were hits, and harder for rows written
    before `+"`ettle confirm`"+` existed — until then the ask named only the two
    negative verdicts and a hooks-only install could record nothing else. Read a
    missing `+"`real`"+` arm as evidence about the surfaces, not the detector.

  · Nothing below the drop floor (%.2f recurrence) is ever surfaced, so no verdict
    about it can exist. These rows can show the floor is too low. Nothing here can
    show it is too high — a real tangle the floor killed leaves no trace.

Below %d rows for a kind this declines to characterise it. That number is a floor,
not a power calculation.
`, rep.DropFloor, rep.MinPerKind)
}
