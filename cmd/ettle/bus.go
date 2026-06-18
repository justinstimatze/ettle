package main

import (
	"strings"

	"github.com/justinstimatze/ettle/internal/transport"
)

// dirBusFor handles the build-tag-INDEPENDENT transport schemes so the file://
// path is defined exactly once and can't drift between the nats and non-nats
// busFor copies (the dual-path bug class). Returns (bus, handled, err): when
// handled is false the caller falls through to its own inproc/nats switch.
func dirBusFor(name string) (transport.Transport, bool, error) {
	if path, ok := strings.CutPrefix(name, "file://"); ok {
		b, err := transport.NewDirBus(path)
		return b, true, err
	}
	return nil, false, nil
}
