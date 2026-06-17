//go:build !nats

package main

import (
	"fmt"

	"github.com/justinstimatze/ettle/internal/transport"
)

// busFor selects the atom transport. The default build only knows the in-process
// adapter (zero infra). Asking for "nats" here points you at the tagged build.
func busFor(name string, _ bool) (transport.Transport, error) {
	switch name {
	case "", "inproc":
		return transport.NewInProcess(), nil
	case "nats":
		return nil, fmt.Errorf("nats transport needs the tagged build: go run -tags nats ./cmd/ettle ...")
	default:
		return nil, fmt.Errorf("unknown transport %q (inproc | nats)", name)
	}
}
