package transport

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
)

// fakeDocStore is an in-memory docStore so the LinearBus logic (title scheme,
// replace-current, identity-from-title, non-ettle skip) is testable with no
// network.
type fakeDocStore struct{ docs map[string]string }

func newFakeDocStore() *fakeDocStore { return &fakeDocStore{docs: map[string]string{}} }

func (f *fakeDocStore) upsert(_ context.Context, title, content string) error {
	f.docs[title] = content
	return nil
}

func (f *fakeDocStore) list(_ context.Context) ([]storedDoc, error) {
	out := make([]storedDoc, 0, len(f.docs))
	for t, c := range f.docs {
		out = append(out, storedDoc{Title: t, Content: c})
	}
	return out, nil
}

func (f *fakeDocStore) close() error { return nil }

func TestLinearBusPublishCollectRoundTrip(t *testing.T) {
	f := newFakeDocStore()
	b := newLinearBusOn(f)
	ctx := context.Background()
	if err := b.Publish(ctx, Envelope{Participant: "Alice", Role: "backend", Atoms: []ettlemesh.Atom{atom("cache")}}); err != nil {
		t.Fatal(err)
	}
	// Stored under the ettle/<slug> title.
	if _, ok := f.docs["ettle/alice"]; !ok {
		t.Fatalf("want a document titled ettle/alice, got titles %v", keys(f.docs))
	}
	envs, err := b.Collect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 {
		t.Fatalf("want 1 envelope, got %d", len(envs))
	}
	e := envs[0]
	// Display casing survives because the in-content slug matches the title slug.
	if e.Participant != "Alice" {
		t.Errorf("participant = %q, want preserved 'Alice'", e.Participant)
	}
	if len(e.Atoms) != 1 || e.Atoms[0].Subject != "cache" {
		t.Errorf("atoms not round-tripped: %+v", e.Atoms)
	}
	if e.EmittedAt == "" {
		t.Error("EmittedAt should be set by Publish")
	}
}

func TestLinearBusReplaceCurrentNotAppend(t *testing.T) {
	f := newFakeDocStore()
	b := newLinearBusOn(f)
	ctx := context.Background()
	_ = b.Publish(ctx, Envelope{Participant: "alice", Atoms: []ettlemesh.Atom{atom("first")}})
	_ = b.Publish(ctx, Envelope{Participant: "alice", Atoms: []ettlemesh.Atom{atom("second")}})

	envs, _ := b.Collect(ctx)
	if len(envs) != 1 {
		t.Fatalf("replace-current: want 1 envelope, got %d", len(envs))
	}
	if envs[0].Atoms[0].Subject != "second" {
		t.Errorf("want latest atoms 'second', got %q", envs[0].Atoms[0].Subject)
	}
}

func TestLinearBusTitleIdentityOverridesSpoof(t *testing.T) {
	f := newFakeDocStore()
	// A document titled for alice whose content claims to be bob — the title wins.
	spoof, _ := json.Marshal(Envelope{Participant: "bob", Atoms: []ettlemesh.Atom{atom("x")}})
	f.docs["ettle/alice"] = string(spoof)

	b := newLinearBusOn(f)
	envs, err := b.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 || envs[0].Participant != "alice" {
		t.Fatalf("title identity should win: got %+v", envs)
	}
	if w := b.Warnings(); len(w) != 1 || !strings.Contains(w[0], "claims participant") {
		t.Errorf("want a spoof warning, got %v", w)
	}
}

func TestLinearBusIgnoresNonEttleDocs(t *testing.T) {
	f := newFakeDocStore()
	f.docs["Team onboarding notes"] = "not an ettle document" // no ettle/ prefix
	f.docs["ettle/alice"] = mustEnvelope(t, "alice", "cache") // ours
	b := newLinearBusOn(f)
	envs, err := b.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 || envs[0].Participant != "alice" {
		t.Fatalf("should ignore the hand-authored doc, keep only ettle/alice: got %+v", envs)
	}
	if w := b.Warnings(); len(w) != 0 {
		t.Errorf("a non-ettle doc should be skipped silently, not warned: %v", w)
	}
}

// TestLinearLive exercises the real GraphQL backend end to end against a throwaway
// workspace, then cleans up after itself. Skipped unless ETTLE_LINEAR_LIVE=1 and
// LINEAR_API_KEY (+ LINEAR_TEAM_ID) are set, so `make ci` never hits the network.
func TestLinearLive(t *testing.T) {
	if os.Getenv("ETTLE_LINEAR_LIVE") != "1" {
		t.Skip("set ETTLE_LINEAR_LIVE=1 (plus LINEAR_API_KEY, LINEAR_TEAM_ID) to run the live Linear test")
	}
	key := strings.TrimSpace(os.Getenv("LINEAR_API_KEY"))
	team := strings.TrimSpace(os.Getenv("LINEAR_TEAM_ID"))
	if key == "" || team == "" {
		t.Fatal("live test needs LINEAR_API_KEY and LINEAR_TEAM_ID")
	}
	room := "livetest-" + time.Now().UTC().Format("150405")
	b, err := NewLinearBus(key, room, team, "test", Workspace{})
	if err != nil {
		t.Fatal(err)
	}
	store := b.store.(*linearDocStore)
	ctx := context.Background()
	// Clean up the project (cascades its documents) no matter how the test ends.
	defer func() {
		var m struct {
			ProjectDelete struct {
				Success bool `json:"success"`
			} `json:"projectDelete"`
		}
		if err := store.do(ctx, `mutation($id:String!){ projectDelete(id:$id){ success } }`,
			map[string]any{"id": store.projectID}, &m); err != nil {
			t.Logf("cleanup: projectDelete failed (delete %q by hand): %v", room, err)
		}
	}()

	if err := b.Publish(ctx, Envelope{Participant: "alice", Atoms: []ettlemesh.Atom{atom("cache")}}); err != nil {
		t.Fatal(err)
	}
	if err := b.Publish(ctx, Envelope{Participant: "bob", Atoms: []ettlemesh.Atom{atom("auth")}}); err != nil {
		t.Fatal(err)
	}
	// Replace alice in place — Collect must still see exactly two people.
	if err := b.Publish(ctx, Envelope{Participant: "alice", Atoms: []ettlemesh.Atom{atom("cache-v2")}}); err != nil {
		t.Fatal(err)
	}
	envs, err := b.Collect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byWho := map[string]string{}
	for _, e := range envs {
		if len(e.Atoms) > 0 {
			byWho[e.Participant] = e.Atoms[0].Subject
		}
	}
	if len(byWho) != 2 || byWho["alice"] != "cache-v2" || byWho["bob"] != "auth" {
		t.Fatalf("live round trip wrong: %+v", byWho)
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func mustEnvelope(t *testing.T, participant, subject string) string {
	t.Helper()
	b, err := json.Marshal(Envelope{Participant: participant, Atoms: []ettlemesh.Atom{atom(subject)}})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
