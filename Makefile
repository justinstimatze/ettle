VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
GOFMT_DIRS := cmd internal

.PHONY: build build-nats vet test install fmt fmt-check ci hooks

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

fmt:                         ## format in place
	gofmt -w $(GOFMT_DIRS)

fmt-check:                   ## fail if anything needs gofmt (exactly what CI enforces)
	@out="$$(gofmt -l $(GOFMT_DIRS))"; \
	if [ -n "$$out" ]; then echo "gofmt needed (run: make fmt):"; echo "$$out"; exit 1; fi

ci: fmt-check                ## the FULL gate CI runs — run before pushing (CI runs this same target)
	go build ./...
	go vet ./...
	go test ./...
	go build -tags nats ./...
	go vet -tags nats ./...
	go test -tags nats ./...

hooks:                       ## install the pre-push hook so `git push` runs `make ci` first
	git config core.hooksPath .githooks
	@echo "pre-push hook active (core.hooksPath=.githooks) — git push now runs 'make ci' first"

install:                     ## install the ettle binary, self-describing its version
	go install $(LDFLAGS) ./cmd/ettle
