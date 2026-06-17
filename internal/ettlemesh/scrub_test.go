package ettlemesh

import (
	"strings"
	"testing"
)

func TestScrubSecretTokenPrefixes(t *testing.T) {
	cases := []struct {
		name, in, marker string
	}{
		{"github classic", "rotate ghp_AbCd1234EfGh5678IjKl into the CI secret", "ghp_AbCd1234EfGh5678IjKl"},
		{"github pat", "use github_pat_11ABCDEFG0aBcDeFgHiJkLmNoP for the action", "github_pat_11ABCDEFG0aBcDeFgHiJkLmNoP"},
		{"openai", "key sk-ant-api03-AbCd1234EfGh5678 for the call", "sk-ant-api03-AbCd1234EfGh5678"},
		{"slack", "webhook xoxb-1234567890-AbCdEfGhIjKl posted", "xoxb-1234567890-AbCdEfGhIjKl"},
		{"aws", "AKIAIOSFODNN7EXAMPLE1 is the access key", "AKIAIOSFODNN7EXAMPLE1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, changed := scrubSecret(c.in)
			if !changed {
				t.Fatalf("expected redaction, got none for %q", c.in)
			}
			if strings.Contains(out, c.marker) {
				t.Errorf("secret survived: %q still in %q", c.marker, out)
			}
			if !strings.Contains(out, redactionMark) {
				t.Errorf("expected %q in output, got %q", redactionMark, out)
			}
		})
	}
}

func TestScrubSecretConnString(t *testing.T) {
	out, changed := scrubSecret("staging DB moved to pg://staging:Hunter2Pg@db.internal:5432/app")
	if !changed {
		t.Fatal("expected redaction of inline credentials")
	}
	if strings.Contains(out, "Hunter2Pg") {
		t.Errorf("password survived: %q", out)
	}
	// Scheme/user/host preserved so the coordination clause still reads.
	for _, keep := range []string{"pg://staging:", "@db.internal"} {
		if !strings.Contains(out, keep) {
			t.Errorf("expected %q preserved, got %q", keep, out)
		}
	}
}

func TestScrubSecretConnStringCredsOnly(t *testing.T) {
	// The standard credentials-only form (empty username before the colon) must
	// still redact the password — redis://:pass@host and postgres://:secret@db.
	cases := []struct{ in, leak string }{
		{"cache moved to redis://:hunter2pw@cache-prod:6379", "hunter2pw"},
		{"DSN is postgres://:s3cretpw@db.internal:5432/app", "s3cretpw"},
	}
	for _, c := range cases {
		out, changed := scrubSecret(c.in)
		if !changed {
			t.Errorf("expected redaction of credentials-only URL %q", c.in)
		}
		if strings.Contains(out, c.leak) {
			t.Errorf("password survived: %q still in %q", c.leak, out)
		}
	}
}

func TestScrubSecretConnStringAtInPassword(t *testing.T) {
	// A password that itself contains '@' must be redacted whole, not just its
	// head — the trailing '@' match is greedy to the LAST '@' before the host.
	out, changed := scrubSecret("pg://staging:p@ssw0rd@db.internal:5432/app")
	if !changed {
		t.Fatal("expected redaction of @-containing password")
	}
	if strings.Contains(out, "ssw0rd") {
		t.Errorf("password tail survived: %q", out)
	}
	if !strings.Contains(out, "@db.internal") {
		t.Errorf("host should be preserved: %q", out)
	}
}

func TestScrubSecretPortURLNotRedacted(t *testing.T) {
	// A URL with a port but no inline credentials (a ':' but no '@') must NOT be
	// caught by the connection-string rule.
	if out, changed := scrubSecret("the API is at https://api.example.com:8443/v2/health"); changed {
		t.Errorf("port-only URL falsely redacted: %q", out)
	}
}

func TestScrubSecretRedactsGitSHA(t *testing.T) {
	// Locks the DOCUMENTED over-redaction: a canonical 40-char git SHA mixes a-f
	// and digits, so it trips the high-entropy gate and is redacted. This is the
	// deliberate bias (a hex-only carve-out would also pass hex-encoded secrets).
	out, changed := scrubSecret("reverting commit 5f2e8a1c9b3d4e6f7a8b2c1d9e0f3a4b5c6d7e8f on main")
	if !changed {
		t.Fatal("expected a mixed-hex SHA to be redacted (documented over-redaction)")
	}
	if strings.Contains(out, "5f2e8a1c9b3d4e6f7a8b2c1d9e0f3a4b5c6d7e8f") {
		t.Errorf("SHA survived: %q", out)
	}
}

func TestScrubSecretPEM(t *testing.T) {
	in := "deploy key -----BEGIN OPENSSH PRIVATE KEY-----b3BlbnNzaC1rZXktdjEAAAAA-----END OPENSSH PRIVATE KEY----- attached"
	out, changed := scrubSecret(in)
	if !changed {
		t.Fatal("expected redaction of PEM block")
	}
	if strings.Contains(out, "b3BlbnNzaC1rZXktdjEAAAAA") {
		t.Errorf("key body survived: %q", out)
	}
}

func TestScrubSecretHighEntropySpan(t *testing.T) {
	out, changed := scrubSecret("the leaked blob is aB3xK9mQ2pL7vR4nT8wY1zC6dF0gH5jE before deploy")
	if !changed {
		t.Fatal("expected redaction of high-entropy span")
	}
	if strings.Contains(out, "aB3xK9mQ2pL7vR4nT8wY1zC6dF0gH5jE") {
		t.Errorf("entropy span survived: %q", out)
	}
}

func TestScrubSecretCleanPassthrough(t *testing.T) {
	// Ordinary coordination clauses, including a long all-letter word and a
	// hyphenated identifier, must survive untouched.
	clean := []string{
		"migrating the billing service to the new schema by Friday",
		"refactoring authentication and authorization across the gateway",
		"the feature-flag rollout-strategy depends on the staging deploy",
		"commit deadbeefcafe1234 needs a cherry-pick onto release",
	}
	for _, s := range clean {
		out, changed := scrubSecret(s)
		if changed {
			t.Errorf("false positive: %q was redacted to %q", s, out)
		}
	}
}

func TestScrubAtomFields(t *testing.T) {
	subj, content, changed := scrubAtomFields("rotate token", "set CI secret to ghp_AbCd1234EfGh5678IjKl now")
	if !changed {
		t.Fatal("expected content redaction")
	}
	if subj != "rotate token" {
		t.Errorf("clean subject mutated: %q", subj)
	}
	if strings.Contains(content, "ghp_AbCd1234EfGh5678IjKl") {
		t.Errorf("token survived in content: %q", content)
	}
}

func TestScrubUserPhrases(t *testing.T) {
	phrases := []string{"compensation", "my health situation"}

	// Case-insensitive substring match: each marked phrase is redacted wherever
	// it appears, regardless of case.
	out, changed := scrubUserPhrases("my Compensation review is Thursday and my health situation is unchanged", phrases)
	if !changed {
		t.Fatal("expected redaction of marked phrases")
	}
	for _, leaked := range []string{"Compensation", "compensation", "health situation"} {
		if strings.Contains(strings.ToLower(out), strings.ToLower(leaked)) {
			t.Errorf("marked phrase survived: %q still in %q", leaked, out)
		}
	}
	if !strings.Contains(out, privateMark) {
		t.Errorf("expected %q in output, got %q", privateMark, out)
	}

	// Unmarked text survives untouched; an empty/blank phrase list is a no-op.
	if got, changed := scrubUserPhrases("shipping the billing refactor by Friday", phrases); changed {
		t.Errorf("false positive on unmarked text: %q", got)
	}
	if _, changed := scrubUserPhrases("anything at all", []string{"", "   "}); changed {
		t.Error("blank phrases must not redact (a stray frontmatter comma is harmless)")
	}
	if _, changed := scrubUserPhrases("anything at all", nil); changed {
		t.Error("nil phrase list must be a no-op")
	}
}

func TestScrubAtomUserPhrases(t *testing.T) {
	subj, content, changed := scrubAtomUserPhrases(
		"comp review timing", "stressed about comp review and hoping for the 185k bump", []string{"185k", "comp review"})
	if !changed {
		t.Fatal("expected redaction across subject and content")
	}
	if strings.Contains(subj, "comp review") || strings.Contains(content, "185k") || strings.Contains(content, "comp review") {
		t.Errorf("marked phrase survived: subj=%q content=%q", subj, content)
	}
	// No phrases → untouched, unchanged.
	if s, c, changed := scrubAtomUserPhrases("a", "b", nil); changed || s != "a" || c != "b" {
		t.Errorf("empty phrase list mutated fields: %q %q %v", s, c, changed)
	}
}
