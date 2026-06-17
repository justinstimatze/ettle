package loopback

import "testing"

func TestIsHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"127.0.0.2", true}, // whole 127/8 is loopback
		{"", false},
		{"8.8.8.8", false},                 // parsed IP, non-loopback → false (no DNS)
		{"nonexistent.invalid", false},     // RFC 6761 reserved TLD: NXDOMAIN, fail-closed, no network
	}
	for _, c := range cases {
		if got := IsHost(c.host); got != c.want {
			t.Errorf("IsHost(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestIsURL(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"nats://localhost:4222", true},
		{"http://127.0.0.1:8088/mcp", true},
		{"https://gemot.invalid/mcp", false}, // unresolvable → fail-closed, no network
		{"http://8.8.8.8:80", false},
		{"://bogus", false},
	}
	for _, c := range cases {
		if got := IsURL(c.url); got != c.want {
			t.Errorf("IsURL(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}
