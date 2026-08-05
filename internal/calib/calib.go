// Package calib reads the accumulated verdict log and reports what it can and
// cannot support about the detector's cut points. It is the READ half of the
// calibration loop; nothing here writes a threshold.
//
// That split is deliberate and it is the design invariant, not a staging decision.
// A loop that moves its own cut points from its own surfaced-and-judged tangles is a
// machine-speed feedback loop over people's coordination behaviour, which CONCEPT.md
// rules out. So this reports, a human decides, and the constants stay in mesh.go
// where they are readable and reviewable.
//
// Two properties of the log bound what any report over it can honestly claim, and
// both are structural rather than "not enough data yet":
//
//  1. The log LEANS negative, and rows written before `ettle confirm` existed lean
//     harder. The horizon used to invite only `not_real` and `handled`, and those
//     were the only two a hooks-only install could record, so a `real` verdict cost
//     a human something and bought nothing. Both halves are fixed — the ask offers
//     all three and confirming stops ettle asking again — but the payoffs are still
//     not equal: muting ends a nuisance, confirming ends a question. Expect fewer
//     `real` rows than hits, and read a missing `real` arm as evidence about the
//     surfaces rather than about the detector. A kind with no `real` rows can bound
//     the false-alarm rate above its bar and cannot estimate the hit rate at all,
//     which is half of what moving a bar needs.
//
//  2. Nothing below the drop floor is representable. A tangle under
//     ettlemesh.DropFloor() is never surfaced, so no human ever sees it and no
//     verdict about it can exist. `not_real` rows can show the floor is too LOW.
//     No row in this file can ever show it is too HIGH — a real tangle the floor
//     killed leaves no trace here. Raising it is measurable; lowering it is a
//     judgement call that needs a different instrument.
//
// Reporting both, every run, is the point. A calibration tool that prints a
// confident number over a one-sided sample is how a project talks itself into a
// threshold it did not measure.
package calib

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"sort"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/mcpserver"
)

// minPerKind is the row count below which this refuses to characterise a kind at
// all. It is a floor, NOT a power calculation: the effect size the loop would be
// trying to detect (how far a bar should move, and how much that changes the
// surfaced set) is exactly what nobody has measured yet, so a real minimum cannot
// be computed. Chosen to be small enough that it is obviously not the binding
// constraint and large enough that a handful of rows cannot look like evidence.
const minPerKind = 20

// Verdicts, mirroring what ettle_respond and `ettle mute` write.
const (
	VerdictReal    = "real"
	VerdictNotReal = "not_real"
	VerdictHandled = "handled"
)

// Bucket is one recurrence band and the verdicts recorded inside it. Recurrence is
// Votes/Samples — the signal the firm bar thresholds on.
type Bucket struct {
	Lo      float64 `json:"lo"`
	Hi      float64 `json:"hi"`
	Real    int     `json:"real"`
	NotReal int     `json:"not_real"`
	Handled int     `json:"handled"`
}

// KindReport is what the log says about one tangle kind.
type KindReport struct {
	Kind    string `json:"kind"`
	Real    int    `json:"real"`
	NotReal int    `json:"not_real"`
	Handled int    `json:"handled"`
	Rows    int    `json:"rows"`

	// WithRecurrence counts rows carrying Votes/Samples. Only ettle_respond running
	// against the server that surfaced the tangle has those; a cross-session verdict
	// (and every `ettle mute`) records the kind and nothing else. A row without them
	// still counts toward Rows and can never inform a bar.
	WithRecurrence int `json:"with_recurrence"`

	FirmBar float64  `json:"firm_bar"`
	Buckets []Bucket `json:"buckets,omitempty"`

	// Blocked names why this kind cannot inform its bar yet, empty if it can.
	Blocked string `json:"blocked,omitempty"`
}

// Report is the whole read.
type Report struct {
	Path       string       `json:"path"`
	Rows       int          `json:"rows"`
	Malformed  int          `json:"malformed"`
	DropFloor  float64      `json:"drop_floor"`
	MinPerKind int          `json:"min_per_kind"`
	Kinds      []KindReport `json:"kinds"`
}

// Read parses the verdict log at path and summarises it. A missing file is not an
// error — it is the normal state of a fresh install, and reporting "no verdicts yet"
// is more useful than a stat error. A malformed line is counted and skipped rather
// than fatal: the log is append-only and written by more than one process, so a torn
// final line is a thing that happens and must not cost the other rows.
func Read(path string) (*Report, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Report{Path: path, DropFloor: ettlemesh.DropFloor(), MinPerKind: minPerKind}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return read(f, path)
}

func read(r io.Reader, path string) (*Report, error) {
	rep := &Report{Path: path, DropFloor: ettlemesh.DropFloor(), MinPerKind: minPerKind}
	byKind := map[string]*KindReport{}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var l mcpserver.Label
		if err := json.Unmarshal(line, &l); err != nil || l.Kind == "" {
			rep.Malformed++
			continue
		}
		rep.Rows++
		k := byKind[l.Kind]
		if k == nil {
			k = &KindReport{Kind: l.Kind, FirmBar: ettlemesh.FirmBarFor(l.Kind)}
			byKind[l.Kind] = k
		}
		k.Rows++
		switch l.Verdict {
		case VerdictReal:
			k.Real++
		case VerdictNotReal:
			k.NotReal++
		case VerdictHandled:
			k.Handled++
		default:
			// A verdict this package does not know is still a row someone recorded.
			// Counting it in Rows and nowhere else keeps the arms honest.
		}
		if l.Samples > 0 {
			k.WithRecurrence++
			addToBucket(k, float64(l.Votes)/float64(l.Samples), l.Verdict)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	rep.Kinds = make([]KindReport, 0, len(byKind))
	for _, k := range byKind {
		k.Blocked = blockedReason(*k)
		sortBuckets(k)
		rep.Kinds = append(rep.Kinds, *k)
	}
	sort.Slice(rep.Kinds, func(i, j int) bool { return rep.Kinds[i].Kind < rep.Kinds[j].Kind })
	return rep, nil
}

// bucketEdges bands recurrence in fifths. The bands are for reading, not for
// estimating — the bar is a point and the data is what it is; grouping only makes a
// distribution legible to a person deciding whether it separates.
var bucketEdges = []float64{0, 0.2, 0.4, 0.6, 0.8, 1.0}

func addToBucket(k *KindReport, recurrence float64, verdict string) {
	lo, hi := bandFor(recurrence)
	for i := range k.Buckets {
		if k.Buckets[i].Lo == lo {
			bump(&k.Buckets[i], verdict)
			return
		}
	}
	b := Bucket{Lo: lo, Hi: hi}
	bump(&b, verdict)
	k.Buckets = append(k.Buckets, b)
}

func bandFor(r float64) (lo, hi float64) {
	if r < 0 {
		r = 0
	}
	if r > 1 {
		r = 1
	}
	for i := 0; i < len(bucketEdges)-1; i++ {
		if r < bucketEdges[i+1] || i == len(bucketEdges)-2 {
			return bucketEdges[i], bucketEdges[i+1]
		}
	}
	return 0, bucketEdges[1]
}

func bump(b *Bucket, verdict string) {
	switch verdict {
	case VerdictReal:
		b.Real++
	case VerdictNotReal:
		b.NotReal++
	case VerdictHandled:
		b.Handled++
	}
}

func sortBuckets(k *KindReport) {
	sort.Slice(k.Buckets, func(i, j int) bool { return k.Buckets[i].Lo < k.Buckets[j].Lo })
}

// blockedReason names why a kind cannot inform its bar, or returns empty if it can.
// The structural check comes FIRST and is the one that will fire: without both a
// `real` arm and a `not_real` arm there is no discrimination to measure at any
// sample size, so reporting "collect more rows" would be wrong advice — what is
// missing is a kind of row, not a number of them.
//
// `handled` deliberately counts toward neither arm. It says the tangle was real and
// is now dealt with, which is a POSITIVE for the detector and a reason to stop
// surfacing; folding it into `real` would inflate the hit rate with rows recorded
// for a different reason, and folding it into `not_real` would be simply false.
func blockedReason(k KindReport) string {
	switch {
	case k.Real == 0 && k.NotReal == 0:
		return "no real/not_real verdicts — only `handled` and unknown rows, which say nothing about whether the bar is in the right place"
	case k.Real == 0:
		return "no `real` verdicts — the log can bound false alarms above the bar and cannot estimate hits, so the bar cannot move on this evidence"
	case k.NotReal == 0:
		return "no `not_real` verdicts — nothing here says the bar is too low"
	case k.WithRecurrence == 0:
		return "no row carries Votes/Samples — every verdict came from a surface that did not hold the recurrence signal the bar thresholds on"
	case k.Rows < minPerKind:
		return "too few rows to characterise"
	}
	return ""
}
