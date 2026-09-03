package transport

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The workspace guard lives inside resolveProject, which is a method on the concrete
// linearDocStore and runs inside NewLinearBus BEFORE the docStore interface is
// injected — so fakeDocStore cannot reach it and the only other path through it is
// the network-gated TestLinearLive. A stub server is therefore the only way to cover
// this key-free, and covering it matters more than usual: every failure mode here is
// silent. A guard that never fires looks exactly like a guard with nothing to catch.

// stubLinear serves canned GraphQL responses chosen by what the query asks for, and
// records every operation so a test can assert that a create did NOT happen.
func stubLinear(t *testing.T, expect Workspace, viewer string, viewerStatus int) (*linearDocStore, *[]string) {
	t.Helper()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(body, &req)
		switch {
		case strings.Contains(req.Query, "viewer"):
			seen = append(seen, "viewer")
			if viewerStatus != 0 && viewerStatus != http.StatusOK {
				w.WriteHeader(viewerStatus)
				_, _ = io.WriteString(w, `{"errors":[{"message":"nope"}]}`)
				return
			}
			_, _ = io.WriteString(w, viewer)
		case strings.Contains(req.Query, "projectCreate"):
			seen = append(seen, "projectCreate")
			_, _ = io.WriteString(w, `{"data":{"projectCreate":{"success":true,"project":{"id":"P_new"}}}}`)
		default: // the projects listing — always empty, so every case reaches the create branch
			seen = append(seen, "projects")
			_, _ = io.WriteString(w, `{"data":{"projects":{"nodes":[]}}}`)
		}
	}))
	t.Cleanup(srv.Close)
	return &linearDocStore{
		http: srv.Client(), apiKey: "lin_test", endpoint: srv.URL, ua: "ettle/test",
		expectOrg: expect,
	}, &seen
}

const orgAcme = `{"data":{"viewer":{"organization":{"id":"org_acme","name":"Acme"}}}}`

func TestResolveProjectRefusesCreateInTheWrongWorkspace(t *testing.T) {
	s, seen := stubLinear(t, Workspace{ID: "org_side", Name: "Side Project"}, orgAcme, 0)

	_, err := s.resolveProject(context.Background(), "crew", "team_1")
	if err == nil {
		t.Fatal("a key from another workspace must be refused, not allowed to create a second ettle-crew")
	}
	// The refusal is only actionable if it says which workspace is which — an id
	// tells a person nothing about where to look.
	for _, want := range []string{"Side Project", "Acme"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal should name both workspaces, missing %q: %v", want, err)
		}
	}
	for _, op := range *seen {
		if op == "projectCreate" {
			t.Fatal("refusing must mean creating NOTHING; a project was created anyway")
		}
	}
}

func TestResolveProjectCreatesInTheExpectedWorkspace(t *testing.T) {
	s, seen := stubLinear(t, Workspace{ID: "org_acme", Name: "Acme"}, orgAcme, 0)

	id, err := s.resolveProject(context.Background(), "crew", "team_1")
	if err != nil {
		t.Fatalf("the matching workspace must still create: %v", err)
	}
	if id != "P_new" {
		t.Errorf("got project id %q, want P_new", id)
	}
	if !contains(*seen, "projectCreate") {
		t.Error("a matching workspace should reach the create")
	}
}

func TestResolveProjectWithNoExpectationCreatesAsBefore(t *testing.T) {
	// The additive case, and the one that keeps this change from breaking every
	// existing install: no recorded workspace means no expectation, so nothing is
	// checked and nothing changes.
	s, seen := stubLinear(t, Workspace{}, orgAcme, 0)

	if _, err := s.resolveProject(context.Background(), "crew", "team_1"); err != nil {
		t.Fatalf("an empty expectation must behave exactly as before: %v", err)
	}
	if contains(*seen, "viewer") {
		t.Error("with no expectation there is nothing to check, so the extra round trip should not be spent")
	}
	if !contains(*seen, "projectCreate") {
		t.Error("an empty expectation should create")
	}
}

func TestResolveProjectRefusesWhenTheWorkspaceCannotBeChecked(t *testing.T) {
	// "Could not check, carry on" is the natural thing to write and would restore the
	// exact silent duplicate the guard exists to prevent.
	s, seen := stubLinear(t, Workspace{ID: "org_side", Name: "Side Project"}, "", http.StatusInternalServerError)

	if _, err := s.resolveProject(context.Background(), "crew", "team_1"); err == nil {
		t.Fatal("an unverifiable workspace must not get a create")
	}
	if contains(*seen, "projectCreate") {
		t.Fatal("a failed check must not fall through to creating")
	}
}

func TestViewerOrgReadsTheWorkspace(t *testing.T) {
	s, _ := stubLinear(t, Workspace{}, orgAcme, 0)

	got, err := s.viewerOrg(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "org_acme" || got.Name != "Acme" {
		t.Errorf("got %+v, want {org_acme Acme}", got)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
