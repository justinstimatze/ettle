package ettlemesh

import (
	"regexp"
	"strings"
)

// scrub.go is the STRUCTURAL half of the privacy boundary — the deterministic
// backstop under the semantic prompt rule (see Distill / SECURITY.md). The
// prompt rule is model judgment ("never emit credentials"); this is the thing
// that holds when judgment doesn't. Anything with the recognizable STRUCTURE of
// a secret — a token with a known prefix, a connection string with inline
// credentials, a private-key header, a high-entropy blob — is redacted before
// the atom crosses, regardless of what the model decided to include.
//
// Design bias, matching the leak eval's liberal matcher: redact the SPAN, never
// drop the whole atom. A false positive costs one coordination clause its
// secret-looking substring (recoverable); a false negative leaks a credential
// (not). We over-redact before we under-redact. The redaction is loud at the
// call site (Distill), never silent — a dropped credential the operator never
// hears about reads as "the boundary held" when it didn't.

const redactionMark = "[redacted-secret]"

// Known token shapes: a fixed prefix followed by a run of token characters.
// These are ~zero-false-positive — no legitimate coordination clause contains
// `ghp_` followed by 16 base64 characters. Longer/more-specific prefixes are
// covered by the shared trailing class; `sk-` subsumes `sk-ant-` (redacting the
// whole span either way), which is fine.
var tokenPrefixRE = regexp.MustCompile(
	`(?:github_pat_|ghp_|gho_|ghu_|ghs_|ghr_|sk-ant-[a-z0-9-]*|sk-|xox[baprs]-|AKIA|ASIA)[A-Za-z0-9_\-]{12,}`,
)

// A connection string carrying inline credentials: scheme://[user]:password@host.
// Redact only the password span, preserving scheme/user/host so the atom can
// still say "the staging DB moved" without leaking the password.
//   - The user part is OPTIONAL ([^/\s:@]*): redis://:pass@host and
//     postgres://:secret@db (the standard credentials-only libpq/redis form) are
//     redacted, not just user:pass@host.
//   - The password class is [^/\s] (allows '@') and the trailing (@) is greedy to
//     the LAST '@' before the host, so a password that itself contains '@'
//     (p@ssw0rd) is redacted whole instead of leaking its tail.
//
// Port-only URLs without credentials (https://host:443/path — a ':' but no '@')
// do not match, since the trailing '@' is required.
var connCredsRE = regexp.MustCompile(`(://[^/\s:@]*:)[^/\s]+(@)`)

// A PEM private-key block. Clipped atoms won't carry the full key body, but the
// BEGIN marker alone is a guaranteed leak-in-progress; redact from the marker to
// the END marker if present, otherwise to end of string.
var pemKeyRE = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?(?:-----END [A-Z0-9 ]*PRIVATE KEY-----|$)`)

// A high-entropy span: a long run of base64/hex-ish characters. This is the SOFT
// catch-all for unprefixed secrets. It is gated on length (>=28) plus a mixed
// alphabet (must contain BOTH a letter and a digit), which keeps it off ordinary
// long words (all-letter) and pure-digit IDs/timestamps. NOTE: a string that
// mixes a-f and digits — including a canonical 40-char git SHA — DOES trip the
// gate and is redacted. That is deliberate over-redaction, not a miss: a hex-only
// carve-out to spare SHAs would also wave through hex-encoded secrets, and per
// this file's bias a commit ref losing its hash span (recoverable) beats leaking
// a hex key (not). UUIDs (36 chars, hyphen-segmented) fall under the length gate
// per-segment and are low-entropy, so they survive.
var entropySpanRE = regexp.MustCompile(`[A-Za-z0-9+/=_\-]{28,}`)

// scrubSecret redacts secret-structured spans from s, returning the cleaned
// string and whether anything changed. Pure (no I/O) so it is unit-testable
// without an API key; the caller owns the loud warning.
func scrubSecret(s string) (string, bool) {
	out := s

	out = tokenPrefixRE.ReplaceAllString(out, redactionMark)
	out = pemKeyRE.ReplaceAllString(out, redactionMark)
	out = connCredsRE.ReplaceAllString(out, "${1}"+redactionMark+"${2}")

	// High-entropy spans last, and only the spans that actually look random —
	// regexp can't express "mixed alphabet", so filter each candidate in Go.
	out = entropySpanRE.ReplaceAllStringFunc(out, func(m string) string {
		if looksHighEntropy(m) {
			return redactionMark
		}
		return m
	})

	return out, out != s
}

// looksHighEntropy gates the soft span matcher: a real secret blob mixes letters
// and digits. A 28-char run that is all letters (a long word, a path) or all
// digits (a timestamp, an ID) is not redacted; one that mixes both is.
func looksHighEntropy(s string) bool {
	var hasLetter, hasDigit bool
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			hasLetter = true
		}
	}
	return hasLetter && hasDigit
}

// scrubAtomFields applies scrubSecret to an atom's Subject and Content, returning
// the redacted pair and whether either changed.
func scrubAtomFields(subject, content string) (string, string, bool) {
	subject, s1 := scrubSecret(subject)
	content, s2 := scrubSecret(content)
	return subject, content, s1 || s2
}

// privateMark is what a user-marked private phrase is replaced with. It is
// deliberately distinct from redactionMark so the loud warning and any crossed
// atom make clear WHICH layer fired — the secret scanner (structural-by-shape)
// or this one (structural-by-user-declaration).
const privateMark = "[redacted-private]"

// scrubUserPhrases is the per-person privacy OVERRIDE: the deterministic half of
// honoring a note's `private:` declaration. The semantic layer (a suppress-list
// in the Distill prompt) asks the model not to emit these phrases; this is the
// thing that holds when judgment doesn't — every listed phrase is redacted by
// case-insensitive substring match, regardless of what the model chose to emit.
// Same defense-in-depth relationship as scrubSecret under the cause-vs-consequence
// prompt rule, but the patterns come from the user, not a fixed secret-shape list.
//
// Pure (no I/O) so it is unit-testable without an API key; the caller owns the
// loud warning. Empty/blank phrases are skipped so a stray trailing comma in the
// frontmatter cannot redact the whole atom.
func scrubUserPhrases(s string, phrases []string) (string, bool) {
	out := s
	for _, p := range phrases {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// (?i) + QuoteMeta: case-insensitive LITERAL match — the phrase is user
		// data, never a pattern, so metacharacters in it must not be live.
		re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(p))
		out = re.ReplaceAllString(out, privateMark)
	}
	return out, out != s
}

// scrubAtomUserPhrases applies scrubUserPhrases to an atom's Subject and Content.
func scrubAtomUserPhrases(subject, content string, phrases []string) (string, string, bool) {
	if len(phrases) == 0 {
		return subject, content, false
	}
	subject, s1 := scrubUserPhrases(subject, phrases)
	content, s2 := scrubUserPhrases(content, phrases)
	return subject, content, s1 || s2
}
