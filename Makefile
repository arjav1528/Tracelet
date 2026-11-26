VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)

.PHONY: build test vet fmt check

build:
	go build -trimpath -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)" -o dist/tracelet ./cmd/tracelet

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w cmd internal

check:
	@test -z "$$(gofmt -l cmd internal)" || (gofmt -l cmd internal; exit 1)
	go vet ./...
	go test ./...
