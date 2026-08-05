package transport

import (
	"context"
	"testing"
)

// fakeReplySource feeds canned activities to the reader with no network, so the
// filter (prompt-only) and cursor logic in Pull are testable in isolation.
type fakeReplySource struct {
	acts     []rawActivity
	more     bool
	gotSince string
}

func (f *fakeReplySource) fetch(_ context.Context, since string) ([]rawActivity, bool, error) {
	f.gotSince = since
	return f.acts, f.more, nil
}

func TestPullKeepsOnlyHumanPromptsAndAdvancesCursor(t *testing.T) {
	f := &fakeReplySource{acts: []rawActivity{
		// ettle's own emit — newest; must advance the cursor but never be a reply.
		{Typename: "AgentActivityElicitationContent", Body: "ettle asked something", Author: "ettle-probe", CreatedAt: "2026-08-05T00:41:33.601Z"},
		// a human reply — the one thing Pull should return.
		{Typename: promptContentType, Body: "  use the shared cache  ", Author: "Bob", IssueID: "IWS-33", SessionID: "sess-1", CreatedAt: "2026-08-05T00:40:00.000Z"},
		// a thought — dropped.
		{Typename: "AgentActivityThoughtContent", Body: "thinking", Author: "ettle-probe", CreatedAt: "2026-08-05T00:39:00.000Z"},
		// a prompt with an empty body — dropped (nothing to distill).
		{Typename: promptContentType, Body: "   ", Author: "Carol", CreatedAt: "2026-08-05T00:38:00.000Z"},
	}}
	r := newLinearAgentReaderOn(f)

	replies, cursor, more, err := r.Pull(context.Background(), "2026-08-01T00:00:00.000Z")
	if err != nil {
		t.Fatal(err)
	}
	if f.gotSince != "2026-08-01T00:00:00.000Z" {
		t.Errorf("source got since %q, want the passed cursor", f.gotSince)
	}
	if len(replies) != 1 {
		t.Fatalf("want exactly 1 human reply, got %d: %+v", len(replies), replies)
	}
	got := replies[0]
	if got.Author != "Bob" || got.Body != "use the shared cache" {
		t.Errorf("reply not the trimmed human prompt: %+v", got)
	}
	if got.IssueID != "IWS-33" || got.SessionID != "sess-1" {
		t.Errorf("reply lost its session/issue context: %+v", got)
	}
	// Cursor advances to the newest activity of ANY kind (the elicitation), so
	// ettle's own emits aren't re-fetched next pull.
	if cursor != "2026-08-05T00:41:33.601Z" {
		t.Errorf("cursor = %q, want the newest activity's createdAt", cursor)
	}
	if more {
		t.Errorf("more should be false for a short page")
	}
}

func TestPullEmptyLeavesCursorAndReportsMore(t *testing.T) {
	f := &fakeReplySource{acts: nil, more: true}
	r := newLinearAgentReaderOn(f)
	replies, cursor, more, err := r.Pull(context.Background(), "2026-08-04T12:00:00.000Z")
	if err != nil {
		t.Fatal(err)
	}
	if len(replies) != 0 {
		t.Errorf("want no replies, got %d", len(replies))
	}
	if cursor != "2026-08-04T12:00:00.000Z" {
		t.Errorf("cursor should be unchanged when nothing was fetched, got %q", cursor)
	}
	if !more {
		t.Errorf("more should propagate from the source")
	}
}

func TestNewerRFC3339(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"2026-08-05T00:00:01Z", "2026-08-05T00:00:00Z", true},
		{"2026-08-05T00:00:00Z", "2026-08-05T00:00:01Z", false},
		{"2026-08-05T00:00:00Z", "", true}, // first run: any timestamp is newer than empty
		{"", "", false},
		{"2026-08-05T00:00:00Z", "2026-08-05T00:00:00Z", false}, // equal is not strictly newer
	}
	for _, c := range cases {
		if got := newerRFC3339(c.a, c.b); got != c.want {
			t.Errorf("newerRFC3339(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
