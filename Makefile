VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build build-nats vet test install fmt

build:                       ## default build (in-process transport, no extra deps)
	go build $(LDFLAGS) ./...

build-nats:                  ## build with the NATS distributed bus
	go build -tags nats $(LDFLAGS) ./...

vet:
	go vet ./...
	go vet -tags nats ./...

test:
	go test ./...
	go test -tags nats ./...

fmt:
	gofmt -w cmd internal

install:                     ## install the ettle binary, self-describing its version
	go install $(LDFLAGS) ./cmd/ettle
