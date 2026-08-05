package calib

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
)

// row builds one verdict log line. votes<0 means "no recurrence features", the shape
// a cross-session verdict and every `ettle mute` writes.
func row(kind, verdict string, votes, samples int) string {
	s := `{"key":"` + kind + `|ivo+mara","verdict":"` + verdict + `","by":"ivo","ts":"2026-08-05T00:00:00Z","kind":"` + kind + `"`
	if samples > 0 {
		s += `,"votes":` + itoa(votes) + `,"samples":` + itoa(samples)
	}
	return s + "}"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func readLines(t *testing.T, lines ...string) *Report {
	t.Helper()
	rep, err := read(strings.NewReader(strings.Join(lines, "\n")), "test")
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func kindOf(t *testing.T, rep *Report, kind string) KindReport {
	t.Helper()
	for _, k := range rep.Kinds {
		if k.Kind == kind {
			return k
		}
	}
	t.Fatalf("no report for kind %q in %+v", kind, rep.Kinds)
	return KindReport{}
}

// A missing log is the normal state of a fresh install, not a failure. Reporting
// "nothing yet" is the whole point of the command on day one.
func TestAMissingLogIsNotAnError(t *testing.T) {
	rep, err := Read(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err != nil {
		t.Fatalf("a missing verdict log should read as empty, got %v", err)
	}
	if rep.Rows != 0 || len(rep.Kinds) != 0 {
		t.Errorf("expected an empty report, got %+v", rep)
	}
	if rep.DropFloor != ettlemesh.DropFloor() {
		t.Errorf("the report must carry the engine's floor, got %v want %v", rep.DropFloor, ettlemesh.DropFloor())
	}
}

// The headline finding this command exists to make visible: verdicts arrive
// one-sided, and a kind with no `real` row cannot move its bar at any sample size.
// "Collect more rows" would be the wrong advice — what's missing is a KIND of row.
func TestAKindWithNoRealVerdictIsBlockedNoMatterHowManyRows(t *testing.T) {
	lines := make([]string, 0, 50)
	for i := 0; i < 50; i++ {
		lines = append(lines, row(ettlemesh.KindCollision, VerdictNotReal, 3, 5))
	}
	k := kindOf(t, readLines(t, lines...), ettlemesh.KindCollision)

	if k.Rows != 50 || k.NotReal != 50 {
		t.Fatalf("expected 50 not_real rows, got %+v", k)
	}
	if k.Blocked == "" {
		t.Fatal("50 one-sided rows must not read as calibratable")
	}
	if !strings.Contains(k.Blocked, "`real`") {
		t.Errorf("the block should name the missing arm, got %q", k.Blocked)
	}
	if strings.Contains(k.Blocked, "too few") {
		t.Errorf("volume is not the problem at 50 rows; got %q", k.Blocked)
	}
}

// `handled` says the tangle was real and is now dealt with. It is neither arm: as a
// hit it would inflate the rate with rows recorded for another reason, as a false
// alarm it would be false. A log of nothing but `handled` teaches the bar nothing.
func TestHandledCountsTowardNeitherArm(t *testing.T) {
	lines := make([]string, 0, 30)
	for i := 0; i < 30; i++ {
		lines = append(lines, row(ettlemesh.KindDuplication, VerdictHandled, 4, 5))
	}
	k := kindOf(t, readLines(t, lines...), ettlemesh.KindDuplication)

	if k.Handled != 30 || k.Real != 0 || k.NotReal != 0 {
		t.Fatalf("handled must not land in either arm: %+v", k)
	}
	if k.Blocked == "" {
		t.Error("a kind with only `handled` rows cannot inform its bar")
	}
}

// A verdict recorded from a surface that never held the recurrence signal still
// counts as a row someone took the trouble to record, and still cannot inform a bar
// that thresholds on recurrence. Both halves matter, so both are reported.
func TestRowsWithoutRecurrenceCountButCannotInformTheBar(t *testing.T) {
	k := kindOf(t, readLines(t,
		row(ettlemesh.KindStaleAssumption, VerdictReal, 0, 0),
		row(ettlemesh.KindStaleAssumption, VerdictNotReal, 0, 0),
	), ettlemesh.KindStaleAssumption)

	if k.Rows != 2 {
		t.Errorf("both rows should count, got %d", k.Rows)
	}
	if k.WithRecurrence != 0 {
		t.Errorf("neither row carries recurrence, got %d", k.WithRecurrence)
	}
	if !strings.Contains(k.Blocked, "Votes/Samples") {
		t.Errorf("the block should name the missing signal, got %q", k.Blocked)
	}
}

// The log is append-only and written by more than one process, so a torn line is a
// thing that happens. It must cost that line and no other.
func TestATornLineCostsOnlyItself(t *testing.T) {
	rep := readLines(t,
		row(ettlemesh.KindCollision, VerdictReal, 4, 5),
		`{"key":"collision|ivo+mara","verdi`,
		"",
		row(ettlemesh.KindCollision, VerdictNotReal, 1, 5),
	)
	if rep.Rows != 2 {
		t.Errorf("two good rows should survive, got %d", rep.Rows)
	}
	if rep.Malformed != 1 {
		t.Errorf("the torn line should be counted once, got %d", rep.Malformed)
	}
}

// With both arms, recurrence present, and enough rows, a kind reads as characterisable
// — and the verdicts land in the recurrence bands they were recorded at, which is the
// distribution a human needs to see to judge whether the bar separates them.
func TestBothArmsWithRecurrenceReadAsCharacterisable(t *testing.T) {
	lines := make([]string, 0, 40)
	for i := 0; i < 20; i++ {
		lines = append(lines, row(ettlemesh.KindCollision, VerdictReal, 5, 5))    // recurrence 1.0
		lines = append(lines, row(ettlemesh.KindCollision, VerdictNotReal, 1, 5)) // recurrence 0.2
	}
	k := kindOf(t, readLines(t, lines...), ettlemesh.KindCollision)

	if k.Blocked != "" {
		t.Fatalf("both arms with recurrence and 40 rows should be readable, blocked: %q", k.Blocked)
	}
	if k.FirmBar != ettlemesh.FirmBarFor(ettlemesh.KindCollision) {
		t.Errorf("the report must show the engine's live bar, got %v", k.FirmBar)
	}
	var low, high Bucket
	for _, b := range k.Buckets {
		if b.Lo == 0.2 {
			low = b
		}
		if b.Hi == 1.0 {
			high = b
		}
	}
	if low.NotReal != 20 || low.Real != 0 {
		t.Errorf("the false alarms recurred at 0.2 and belong in that band: %+v", low)
	}
	if high.Real != 20 || high.NotReal != 0 {
		t.Errorf("the hits recurred at 1.0 and belong in the top band: %+v", high)
	}
}

// The per-kind bar comes from the engine, so decision-rights (which the diagnostic
// batch set lower than the default) reports its own number rather than the default.
// A report carrying its own copy would keep printing the old value after a change.
func TestTheBarIsReadFromTheEngineNotDuplicated(t *testing.T) {
	rep := readLines(t,
		row(ettlemesh.KindDecisionRights, VerdictReal, 2, 5),
		row(ettlemesh.KindCollision, VerdictReal, 4, 5),
	)
	dr := kindOf(t, rep, ettlemesh.KindDecisionRights)
	co := kindOf(t, rep, ettlemesh.KindCollision)
	if dr.FirmBar == co.FirmBar {
		t.Fatalf("decision-rights has a lower bar than the default; both read %v", dr.FirmBar)
	}
	if dr.FirmBar != ettlemesh.FirmBarFor(ettlemesh.KindDecisionRights) {
		t.Errorf("got %v, want the engine's %v", dr.FirmBar, ettlemesh.FirmBarFor(ettlemesh.KindDecisionRights))
	}
}
