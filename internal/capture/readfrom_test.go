package capture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeTranscript(t *testing.T, n int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "t.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for i := 1; i <= n; i++ {
		// A real human prompt is STRING content; array content is a tool result coming
		// back, which Read deliberately skips.
		rec := map[string]any{"type": "user", "message": map[string]any{
			"role": "user", "content": "prompt " + string(rune('a'+i-1))}}
		b, _ := json.Marshal(rec)
		if _, err := f.Write(append(b, '\n')); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// The point of the offset: a long session is not re-digested in full every couple of
// minutes, so the cost of a capture stops climbing with the session's length.
func TestReadFromSkipsAlreadyDistilledLines(t *testing.T) {
	path := writeTranscript(t, 5)

	all, total, err := ReadFrom(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Fatalf("total should count every line, got %d", total)
	}
	rest, total2, err := ReadFrom(path, 3)
	if err != nil {
		t.Fatal(err)
	}
	if total2 != 5 {
		t.Errorf("total is the file's length regardless of the offset, got %d", total2)
	}
	if len(rest.Prompts) >= len(all.Prompts) {
		t.Errorf("reading from an offset should yield fewer prompts: %d vs %d",
			len(rest.Prompts), len(all.Prompts))
	}
	if len(rest.Prompts) == 0 {
		t.Error("the turns after the offset are the whole point and must survive")
	}
}

func TestReadFromStartsOverWhenTheTranscriptShrank(t *testing.T) {
	// A fresh session reusing a path, or a truncated file. Silently publishing nothing
	// would lose the session; starting over merely costs a re-distill.
	path := writeTranscript(t, 2)
	s, total, err := ReadFrom(path, 99)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(s.Prompts) == 0 {
		t.Errorf("an offset past the end must re-read the whole file, got total=%d prompts=%d",
			total, len(s.Prompts))
	}
}

func TestReadIsReadFromZero(t *testing.T) {
	path := writeTranscript(t, 4)
	a, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := ReadFrom(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Prompts) == 0 {
		t.Fatal("the fixture produced no prompts — the test would pass comparing 0 to 0")
	}
	if len(a.Prompts) != len(b.Prompts) {
		t.Errorf("Read must stay exactly ReadFrom(path, 0): %d vs %d", len(a.Prompts), len(b.Prompts))
	}
}
