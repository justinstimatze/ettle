//go:build !nats

package main

import (
	"fmt"

	"github.com/justinstimatze/ettle/internal/transport"
)

// busFor selects the atom transport. The default build knows the in-process
// adapter (zero infra) and the file:// directory bus (shared synced folder, also
// zero infra). Asking for "nats" here points you at the tagged build.
func busFor(name string, _ bool) (transport.Transport, error) {
	if b, ok, err := dirBusFor(name); ok {
		return b, err
	}
	switch name {
	case "", "inproc":
		return transport.NewInProcess(), nil
	case "nats":
		return nil, fmt.Errorf("nats transport needs the tagged build: go run -tags nats ./cmd/ettle ...")
	default:
		return nil, fmt.Errorf("unknown transport %q (inproc | file://<path> | nats)", name)
	}
}
