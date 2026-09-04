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

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	all, size, err := ReadFrom(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	// The offset is a BYTE offset, so a capture seeks past what it already read rather
	// than reading and discarding it — the difference between O(new) and O(file) on a
	// session running for hours.
	if size != fi.Size() {
		t.Fatalf("the returned offset should be the file size, got %d want %d", size, fi.Size())
	}
	rest, size2, err := ReadFrom(path, fi.Size()/2)
	if err != nil {
		t.Fatal(err)
	}
	if size2 != fi.Size() {
		t.Errorf("the size is the file's, regardless of where the read started, got %d", size2)
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
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	s, size, err := ReadFrom(path, fi.Size()*10)
	if err != nil {
		t.Fatal(err)
	}
	if size != fi.Size() || len(s.Prompts) == 0 {
		t.Errorf("an offset past the end must re-read the whole file, got size=%d prompts=%d",
			size, len(s.Prompts))
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
