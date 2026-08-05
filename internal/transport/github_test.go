package transport

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
)

// fakeCommentStore is an in-memory commentStore so the GitHubBus logic (marker
// scheme, replace-current, identity-from-marker, non-ettle skip) is testable with
// no network.
type fakeCommentStore struct{ comments map[string]string }

func newFakeCommentStore() *fakeCommentStore {
	return &fakeCommentStore{comments: map[string]string{}}
}

func (f *fakeCommentStore) upsert(_ context.Context, participant, content string) error {
	f.comments[participant] = content
	return nil
}

func (f *fakeCommentStore) list(_ context.Context) ([]storedComment, error) {
	out := make([]storedComment, 0, len(f.comments))
	for p, c := range f.comments {
		out = append(out, storedComment{Participant: p, Content: c, ID: "DC_" + p})
	}
	return out, nil
}

func (f *fakeCommentStore) close() error { return nil }

func TestGitHubBusPublishCollectRoundTrip(t *testing.T) {
	f := newFakeCommentStore()
	b := newGitHubBusOn(f)
	ctx := context.Background()
	if err := b.Publish(ctx, Envelope{Participant: "Alice", Role: "backend", Atoms: []ettlemesh.Atom{atom("cache")}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.comments["alice"]; !ok {
		t.Fatalf("should be stored under the participant slug, got %v", f.comments)
	}

	envs, err := b.Collect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 || envs[0].Participant != "Alice" || envs[0].Role != "backend" {
		t.Fatalf("round-trip mismatch: %+v", envs)
	}
	if envs[0].EmittedAt == "" {
		t.Error("Publish should stamp EmittedAt when unset")
	}

	// Replace-current: a second publish overwrites rather than accumulating.
	if err := b.Publish(ctx, Envelope{Participant: "alice", Atoms: []ettlemesh.Atom{atom("queue")}}); err != nil {
		t.Fatal(err)
	}
	envs, err = b.Collect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 {
		t.Fatalf("same person should occupy one comment, got %d", len(envs))
	}
	if envs[0].Atoms[0].Subject != "queue" {
		t.Errorf("later publish should replace the earlier: %+v", envs[0].Atoms)
	}
}

func TestGitHubBusMarkerIdentityIsAuthoritative(t *testing.T) {
	f := newFakeCommentStore()
	b := newGitHubBusOn(f)
	// A comment marked as bob's that claims to be alice: the marker wins, and the
	// mismatch is warned. `ettle pull` publishes under a teammate's identity with the
	// puller's token, so the comment AUTHOR can never be the check here.
	spoof, _ := json.Marshal(Envelope{Participant: "alice", Atoms: []ettlemesh.Atom{atom("x")}})
	f.comments["bob"] = string(spoof)

	envs, err := b.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 || envs[0].Participant != "bob" {
		t.Fatalf("marker identity should override the in-content claim: %+v", envs)
	}
	w := b.Warnings()
	if len(w) != 1 || !strings.Contains(w[0], "alice") {
		t.Errorf("the override should be warned, got %v", w)
	}
}

func TestGitHubBusSkipsUnparseableAndKeepsGoing(t *testing.T) {
	f := newFakeCommentStore()
	b := newGitHubBusOn(f)
	good, _ := json.Marshal(Envelope{Participant: "alice", Atoms: []ettlemesh.Atom{atom("x")}})
	f.comments["alice"] = string(good)
	f.comments["bob"] = "not json at all"

	envs, err := b.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 || envs[0].Participant != "alice" {
		t.Fatalf("one bad comment must not lose the good one: %+v", envs)
	}
	if len(b.Warnings()) != 1 {
		t.Errorf("the skip should be warned, got %v", b.Warnings())
	}
}

func TestCommentBodyRoundTripAndHumanRepliesIgnored(t *testing.T) {
	body := renderCommentBody("alice", `{"v":1,"participant":"alice"}`)
	who, content, ok := parseCommentBody(body)
	if !ok || who != "alice" || content != `{"v":1,"participant":"alice"}` {
		t.Fatalf("round-trip mismatch: ok=%v who=%q content=%q", ok, who, content)
	}
	// A teammate replying in the thread is left strictly alone.
	for _, human := range []string{
		"Is this still accurate?",
		"```json\n{\"participant\":\"alice\"}\n```", // fenced json but no marker
		"<!-- ettle: -->\nempty marker",
	} {
		if _, _, ok := parseCommentBody(human); ok {
			t.Errorf("a human comment must not be read as an ettle comment: %q", human)
		}
	}
}

func TestParseGitHubSpec(t *testing.T) {
	cases := []struct{ in, owner, repo, room string }{
		{"acme/widgets", "acme", "widgets", "default"},
		{"acme/widgets/crew", "acme", "widgets", "crew"},
		{"/acme/widgets/", "acme", "widgets", "default"},
	}
	for _, c := range cases {
		o, r, room, err := ParseGitHubSpec(c.in)
		if err != nil || o != c.owner || r != c.repo || room != c.room {
			t.Errorf("%q → %q/%q/%q err=%v, want %q/%q/%q", c.in, o, r, room, err, c.owner, c.repo, c.room)
		}
	}
	for _, bad := range []string{"acme", "", "a/b/c/d"} {
		if _, _, _, err := ParseGitHubSpec(bad); err == nil {
			t.Errorf("%q should not parse", bad)
		}
	}
}

// stubGitHub serves one canned GraphQL response, so the visibility guard is
// tested for real (it lives in resolveDiscussion, behind a network call) without
// touching github.com.
func stubGitHub(t *testing.T, body string) *githubCommentStore {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return &githubCommentStore{
		http: srv.Client(), token: "tok", endpoint: srv.URL, ua: "ettle/test",
		owner: "acme", repo: "widgets",
	}
}

func TestResolveDiscussionRefusesPublicRepo(t *testing.T) {
	s := stubGitHub(t, `{"data":{"repository":{"id":"R_1","isPrivate":false,"hasDiscussionsEnabled":true,
	  "discussionCategories":{"nodes":[{"id":"C_1","name":"General","slug":"general"}]},
	  "discussions":{"nodes":[]}}}}`)
	err := s.resolveDiscussion(context.Background(), "crew")
	if err == nil {
		t.Fatal("a PUBLIC repo must be refused — its Discussions are world-readable")
	}
	if !strings.Contains(err.Error(), "PUBLIC") {
		t.Errorf("the refusal should say why, got: %v", err)
	}
	if s.discussionID != "" {
		t.Error("nothing should be resolved after a refusal")
	}
}

func TestResolveDiscussionReportsDisabledDiscussionsAndMissingRepo(t *testing.T) {
	off := stubGitHub(t, `{"data":{"repository":{"id":"R_1","isPrivate":true,"hasDiscussionsEnabled":false,
	  "discussionCategories":{"nodes":[]},"discussions":{"nodes":[]}}}}`)
	err := off.resolveDiscussion(context.Background(), "crew")
	if err == nil || !strings.Contains(err.Error(), "Discussions are not enabled") {
		t.Errorf("a private repo with Discussions off should say so, got: %v", err)
	}

	missing := stubGitHub(t, `{"data":{"repository":null}}`)
	err = missing.resolveDiscussion(context.Background(), "crew")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("an invisible repo should say so, got: %v", err)
	}
}

func TestResolveDiscussionFindsExistingRoom(t *testing.T) {
	s := stubGitHub(t, `{"data":{"repository":{"id":"R_1","isPrivate":true,"hasDiscussionsEnabled":true,
	  "discussionCategories":{"nodes":[{"id":"C_1","name":"General","slug":"general"}]},
	  "discussions":{"nodes":[{"id":"D_other","title":"Roadmap"},{"id":"D_crew","title":"ettle/crew"}]}}}}`)
	if err := s.resolveDiscussion(context.Background(), "crew"); err != nil {
		t.Fatal(err)
	}
	if s.discussionID != "D_crew" {
		t.Errorf("should reuse the room's discussion, not the team's own: %q", s.discussionID)
	}
}

func TestNewGitHubBusRejectsMissingPieces(t *testing.T) {
	if _, err := NewGitHubBus("", "acme", "widgets", "crew", "test"); err == nil {
		t.Error("an empty token should be refused before any network call")
	}
	if _, err := NewGitHubBus("tok", "", "widgets", "crew", "test"); err == nil {
		t.Error("a missing owner should be refused before any network call")
	}
}
