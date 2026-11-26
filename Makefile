VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)

.PHONY: build example test vet fmt check

build:
	go build -trimpath -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)" -o dist/tracelet ./cmd/tracelet

example:
	go build -o dist/tracelet-go-api ./examples/go-api

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w cmd internal examples

check:
	@test -z "$$(gofmt -l cmd internal examples)" || (gofmt -l cmd internal examples; exit 1)
	go vet ./...
	go test ./...
