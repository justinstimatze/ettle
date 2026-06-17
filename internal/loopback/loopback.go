// Package loopback decides whether a URL points at the local machine — the
// gate for the dev-only `--insecure-local` (plaintext/tokenless) paths.
//
// It does NOT trust the hostname string alone: a name like "evil.local" (or an
// /etc/hosts entry) can be made to *look* local while pointing elsewhere. So
// anything other than the literal loopback names is RESOLVED, and every
// returned address must be a loopback IP. This catches a non-loopback hostname
// dressed up as local.
//
// Honest limit: a process that binds loopback and then port-forwards it off-box
// (an SSH tunnel, `socat`, a sidecar proxy) still resolves to 127.0.0.1 here —
// IP resolution cannot see the forward. That requires deliberate operator
// action, not an accidental misconfiguration; the insecure-local path is dev-only
// and documented as such. This check closes the accidental cases, not a
// determined operator routing loopback elsewhere.
package loopback

import (
	"net"
	neturl "net/url"
)

// IsURL reports whether rawURL's host is the local machine: a literal loopback
// name, or a hostname that resolves to loopback IPs only. A parse failure, an
// unresolvable host, or any non-loopback address resolved for it returns false
// (fail closed — an unverifiable host is treated as remote).
func IsURL(rawURL string) bool {
	u, err := neturl.Parse(rawURL)
	if err != nil {
		return false
	}
	return IsHost(u.Hostname())
}

// IsHost reports whether host is loopback. Literal loopback names short-circuit
// (no DNS); any other name is resolved and every address must be loopback.
func IsHost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	case "":
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	addrs, err := net.LookupHost(host)
	if err != nil || len(addrs) == 0 {
		return false
	}
	for _, a := range addrs {
		ip := net.ParseIP(a)
		if ip == nil || !ip.IsLoopback() {
			return false
		}
	}
	return true
}
