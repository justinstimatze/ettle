package transport

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Resolving a team by the key or name a person can actually SEE is the whole point:
// LINEAR_TEAM_ID is a uuid, Linear shows that uuid on no screen, and the documented
// alternative was a curl with the key interpolated into a shell.
//
// These drive resolveTeam on a stubbed store, which is the same code the live call
// runs — a test that reimplemented the matching would pass while the real function
// was broken.
func teamStore(t *testing.T, body string) *linearDocStore {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return &linearDocStore{http: srv.Client(), apiKey: "k", endpoint: srv.URL, ua: "ettle/test"}
}

const twoTeams = `{"data":{"teams":{"nodes":[
  {"id":"11111111-1111-1111-1111-111111111111","key":"CUR","name":"Current AI"},
  {"id":"22222222-2222-2222-2222-222222222222","key":"ENG","name":"Engineering"}]}}}`

func TestResolveTeamByKeyAndName(t *testing.T) {
	s := teamStore(t, twoTeams)
	// Case-insensitive on both, because a person reads "CUR" off an issue and types
	// whatever they remember.
	for _, want := range []string{"CUR", "cur", "Current AI", "current ai"} {
		got, err := s.resolveTeam(context.Background(), want)
		if err != nil || got.ID != "11111111-1111-1111-1111-111111111111" {
			t.Errorf("%q -> %+v, %v; want the CUR team", want, got, err)
		}
	}
}

func TestResolveTeamPassesAUUIDThroughWithoutALookup(t *testing.T) {
	// An existing LINEAR_TEAM_ID must keep working untouched, or this change breaks
	// every install that already found its uuid the hard way. The stub returns no
	// teams at all, so reaching the network would fail the match.
	const id = "99999999-9999-9999-9999-999999999999"
	s := teamStore(t, `{"data":{"teams":{"nodes":[]}}}`)
	got, err := s.resolveTeam(context.Background(), id)
	if err != nil || got.ID != id {
		t.Errorf("a uuid should pass straight through, got %+v %v", got, err)
	}
}

func TestResolveTeamRefusesAMissAndSaysWhatExists(t *testing.T) {
	s := teamStore(t, twoTeams)
	_, err := s.resolveTeam(context.Background(), "NOPE")
	if err == nil {
		t.Fatal("an unknown team must be an error, not a silent fall back to the first one")
	}
	// The error has to be actionable — naming the teams that DO exist is the
	// difference between one retry and a hunt.
	for _, want := range []string{"CUR", "ENG"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal should list the real teams, missing %q: %v", want, err)
		}
	}
}

func TestResolveTeamRefusesAnAmbiguousMatch(t *testing.T) {
	// Guessing here creates the room's project under the wrong team, which nobody
	// notices until a teammate cannot find it.
	s := teamStore(t, `{"data":{"teams":{"nodes":[
	  {"id":"a","key":"OPS","name":"Platform"},
	  {"id":"b","key":"PLATFORM","name":"Ops"}]}}}`)
	if _, err := s.resolveTeam(context.Background(), "platform"); err == nil {
		t.Error("two teams matching must be an error rather than a coin flip")
	}
}

func TestLooksLikeUUIDSeparatesIdsFromKeys(t *testing.T) {
	for _, s := range []string{"CUR", "Current AI", "", "1111111111111111111111111111111111111"} {
		if looksLikeUUID(s) {
			t.Errorf("%q is not a uuid", s)
		}
	}
	if !looksLikeUUID("11111111-1111-1111-1111-111111111111") {
		t.Error("a real uuid should be recognized, or every existing LINEAR_TEAM_ID pays for a needless lookup")
	}
}
