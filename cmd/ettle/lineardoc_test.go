package main

import (
	"strings"
	"testing"
)

// doLinearDocUpsert validates its arguments before touching the network, so these
// run key-free. The pattern this guards: pennon (a separate project) drives this
// command as a subprocess, so a malformed call from there has to fail loudly and
// early rather than reach Linear and fail on some unrelated 400.
func TestLinearDocUpsertValidatesBeforeAnyNetworkCall(t *testing.T) {
	cases := []struct {
		name, room, title, content string
		wantErrSubstr              string
	}{
		{"empty room", "", "alice", "content", "--room"},
		{"empty title", "crew", "", "content", "--title"},
		{"reserved ettle/ prefix", "crew", "ettle/alice", "content", "ettle/"},
		{"empty content", "crew", "alice", "", "--content"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := doLinearDocUpsert(tc.room, tc.title, tc.content, "")
			if err == nil {
				t.Fatalf("want an error for %+v, got nil", tc)
			}
			if !strings.Contains(err.Error(), tc.wantErrSubstr) {
				t.Errorf("error %q doesn't mention %q — the caller can't tell which flag was wrong", err, tc.wantErrSubstr)
			}
		})
	}
}
