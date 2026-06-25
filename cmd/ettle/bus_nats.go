//go:build nats

package main

import (
	"fmt"

	"github.com/justinstimatze/ettle/internal/transport"
)

// busFor (tagged build) adds the distributed NATS bus. NATS_URL / NATS_CREDS
// configure it; TLS + auth are enforced unless --insecure-local opts into the
// localhost-plaintext path (for local docker).
func busFor(name string, insecureLocal bool) (transport.Transport, error) {
	if b, ok, err := dirBusFor(name); ok {
		return b, err
	}
	switch name {
	case "", "inproc":
		return transport.NewInProcess(), nil
	case "nats":
		return transport.DialNATS(transport.NATSConfig{InsecureTCP: insecureLocal})
	default:
		return nil, fmt.Errorf("unknown transport %q (inproc | file://<path> | leat://<repoDir> | nats)", name)
	}
}
