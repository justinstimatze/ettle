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
	// The markdown-metacharacter round-trip guard: the exact shape that silently
	// rejected every ettle-dumpling participant before the ```json fence (06565fd).
	// If Linear ever changes what its normalizer does to a fenced block, this goes
	// red against the LIVE API instead of a room quietly emptying again.
	markdownProbe := "branch marsjustin/cur-1403 *bold* [link] `code` ~tilde~ and a \"quoted\" span"
	if err := b.Publish(ctx, Envelope{Participant: "carol", Atoms: []ettlemesh.Atom{atom(markdownProbe)}}); err != nil {
		t.Fatal(err)
	}
	envs, err := b.Collect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if w := b.Warnings(); len(w) != 0 {
		t.Fatalf("live collect warned — a markdown-mangled envelope failed to parse: %v", w)
	}
	byWho := map[string]string{}
	for _, e := range envs {
		if len(e.Atoms) > 0 {
			byWho[e.Participant] = e.Atoms[0].Subject
		}
	}
	if len(byWho) != 3 || byWho["alice"] != "cache-v2" || byWho["bob"] != "auth" {
		t.Fatalf("live round trip wrong: %+v", byWho)
	}
	if byWho["carol"] != markdownProbe {
		t.Fatalf("markdown probe did not round-trip byte-identical against the live API:\nwant %q\ngot  %q", markdownProbe, byWho["carol"])
	}
}

// PublishDocument is the escape hatch a separate project (pennon) uses to write
// its own content verbatim, with no atom envelope. The one invariant that
// matters: it cannot collide with ettle's own atom documents, which all live
// under the "ettle/" prefix Collect filters on.
func TestPublishDocumentWritesVerbatimNoEnvelope(t *testing.T) {
	f := newFakeDocStore()
	b := newLinearBusOn(f)
	ctx := context.Background()
	if err := b.PublishDocument(ctx, "pennon/venus", "raw content, not JSON"); err != nil {
		t.Fatal(err)
	}
	if got := f.docs["pennon/venus"]; got != "raw content, not JSON" {
		t.Fatalf("content should be stored verbatim, got %q", got)
	}
	// The point of a non-ettle title: Collect must never surface it as an atom.
	envs, err := b.Collect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 0 {
		t.Fatalf("a non-ettle document leaked into Collect: %+v", envs)
	}
}

func TestPublishDocumentRefusesTheEttlePrefix(t *testing.T) {
	f := newFakeDocStore()
	b := newLinearBusOn(f)
	err := b.PublishDocument(context.Background(), "ettle/alice", "trying to overwrite an atom document")
	if err == nil {
		t.Fatal("want an error: a caller here must not be able to touch ettle's own atom titles")
	}
	if _, ok := f.docs["ettle/alice"]; ok {
		t.Fatal("the refused write must not have reached the store")
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

// linearMarkdownDocStore is a fakeDocStore that reproduces what the live Linear API
// actually does to a Document's content, measured 2026-09-16 by writing a document
// through documentCreate and reading it back through document(id){content}:
// a backslash is INSERTED before each of * [ ] ` ~ and DELETED from every \" —
// except inside a fenced code block, which is passed through untouched.
//
// The plain fakeDocStore above stores content verbatim, so every test using it
// passed while six live participants' envelopes were being rejected in production.
// A fake that is more faithful than the real backend cannot go red for the one
// failure this transport actually has.
type linearMarkdownDocStore struct{ docs map[string]string }

func newLinearMarkdownDocStore() *linearMarkdownDocStore {
	return &linearMarkdownDocStore{docs: map[string]string{}}
}

func mangleLikeLinear(content string) string {
	if strings.HasPrefix(strings.TrimSpace(content), "```") {
		return content // fenced: the normalizer leaves it alone
	}
	var b strings.Builder
	for i := 0; i < len(content); i++ {
		c := content[i]
		if c == '\\' && i+1 < len(content) && content[i+1] == '"' {
			continue // the \" backslash is dropped
		}
		if strings.IndexByte("*[]`~", c) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	return b.String()
}

func (f *linearMarkdownDocStore) upsert(_ context.Context, title, content string) error {
	f.docs[title] = mangleLikeLinear(content)
	return nil
}

func (f *linearMarkdownDocStore) list(_ context.Context) ([]storedDoc, error) {
	out := make([]storedDoc, 0, len(f.docs))
	for t, c := range f.docs {
		out = append(out, storedDoc{Title: t, Content: c})
	}
	return out, nil
}

func (f *linearMarkdownDocStore) close() error { return nil }

// An envelope must survive Linear's markdown normalization, atoms included. Before
// the fence, Publish wrote bare JSON whose "atoms":[ came back as "atoms":\[ and
// Collect rejected every document in the room as unparseable.
func TestLinearBusEnvelopeSurvivesMarkdownNormalization(t *testing.T) {
	f := newLinearMarkdownDocStore()
	b := newLinearBusOn(f)
	ctx := context.Background()
	if err := b.Publish(ctx, Envelope{
		Participant: "saturn@justin",
		Role:        "backend",
		Atoms:       []ettlemesh.Atom{atom("a*b [c] `d` ~e~ and a \"quoted\" span")},
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	got, err := b.Collect(ctx)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if w := b.Warnings(); len(w) != 0 {
		t.Fatalf("collect warned on its own published envelope: %v", w)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 envelope back, got %d — the room reads as empty", len(got))
	}
	if len(got[0].Atoms) != 1 {
		t.Fatalf("want 1 atom, got %d", len(got[0].Atoms))
	}
	if got[0].Participant != "saturn@justin" {
		t.Errorf("participant = %q, want saturn@justin", got[0].Participant)
	}
}

// The mangling fake has to actually mangle, or the test above proves nothing.
func TestMangleLikeLinearBreaksBareJSONAndSparesAFence(t *testing.T) {
	bare := `{"atoms":["x"],"s":"say \"hi\""}`
	if mangleLikeLinear(bare) == bare {
		t.Fatal("the fake left bare JSON untouched; it cannot reproduce the defect")
	}
	var v any
	if err := json.Unmarshal([]byte(mangleLikeLinear(bare)), &v); err == nil {
		t.Fatal("mangled bare JSON still parses; the fake is not faithful to Linear")
	}
	fenced := fenceEnvelope(bare)
	if mangleLikeLinear(fenced) != fenced {
		t.Fatal("the fake mangled a fenced block; Linear does not")
	}
	if unfenceEnvelope(mangleLikeLinear(fenced)) != bare {
		t.Fatal("fence did not round-trip through the fake")
	}
}

// A document written by an ettle older than the fence, or hand-authored, still reads.
func TestLinearBusReadsUnfencedLegacyContent(t *testing.T) {
	f := newFakeDocStore()
	f.docs["ettle/mercury@justin"] = `{"participant":"mercury@justin","atoms":[],"v":1}`
	b := newLinearBusOn(f)
	got, err := b.Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(got) != 1 || got[0].Participant != "mercury@justin" {
		t.Fatalf("legacy unfenced document did not read back: %+v (warnings %v)", got, b.Warnings())
	}
}
