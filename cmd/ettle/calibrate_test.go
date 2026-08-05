package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justinstimatze/ettle/internal/calib"
)

func writeLog(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "labels.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// The day-one output. A fresh install has no verdicts, and saying so — plus how they
// start accruing — is the whole job; an error or an empty page would both be worse.
func TestCalibrateOnAFreshInstallSaysSoAndSaysHowToStart(t *testing.T) {
	out := captureStdout(t, func() {
		if err := runCalibrate([]string{"--labels", filepath.Join(t.TempDir(), "none.jsonl")}); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{"no verdicts recorded yet", "ettle_respond", "ettle mute --wrong"} {
		if !strings.Contains(out, want) {
			t.Errorf("a fresh install should mention %q:\n%s", want, out)
		}
	}
}

// The structural limits print on every run, empty log included. They are properties
// of what gets recorded, so a full log is exactly as subject to them — a reader who
// only sees this page once should not have to go find that out elsewhere.
func TestTheStructuralLimitsPrintEvenWithNoData(t *testing.T) {
	out := captureStdout(t, func() {
		if err := runCalibrate([]string{"--labels", filepath.Join(t.TempDir(), "none.jsonl")}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "drop floor") || !strings.Contains(out, "leaves no trace") {
		t.Errorf("the drop-floor limit should always print:\n%s", out)
	}
	if !strings.Contains(out, "lean negative") {
		t.Errorf("the one-sidedness warning should always print:\n%s", out)
	}
}

// `ettle calibrate` reads and reports. It must not touch the log it was pointed at —
// a tool that mutates the ground truth it is measuring is worse than no tool.
func TestCalibrateDoesNotWriteToTheLog(t *testing.T) {
	line := `{"key":"collision|ivo+mara","verdict":"real","by":"ivo","ts":"2026-08-05T00:00:00Z","kind":"collision","votes":4,"samples":5}`
	p := writeLog(t, line)
	before, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	captureStdout(t, func() {
		if err := runCalibrate([]string{"--labels", p}); err != nil {
			t.Fatal(err)
		}
	})
	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("calibrate rewrote the log:\nbefore %q\nafter  %q", before, after)
	}
}

// --json is the agent-facing surface, so the blocking reason has to survive into it.
// A machine reading only the counts would see 22 rows and conclude there was data.
func TestJSONCarriesTheBlockingReason(t *testing.T) {
	lines := make([]string, 0, 22)
	for i := 0; i < 22; i++ {
		lines = append(lines, `{"key":"collision|ivo+mara","verdict":"not_real","by":"ivo","ts":"2026-08-05T00:00:00Z","kind":"collision","votes":1,"samples":5}`)
	}
	out := captureStdout(t, func() {
		if err := runCalibrate([]string{"--labels", writeLog(t, lines...), "--json"}); err != nil {
			t.Fatal(err)
		}
	})
	var rep calib.Report
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("--json should emit parseable JSON: %v\n%s", err, out)
	}
	if rep.Rows != 22 {
		t.Errorf("got %d rows, want 22", rep.Rows)
	}
	if len(rep.Kinds) != 1 || rep.Kinds[0].Blocked == "" {
		t.Fatalf("22 one-sided rows must carry a blocking reason: %+v", rep.Kinds)
	}
	if !strings.Contains(rep.Kinds[0].Blocked, "`real`") {
		t.Errorf("the reason should name the missing arm, got %q", rep.Kinds[0].Blocked)
	}
}
