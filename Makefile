VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)

.PHONY: build web web-check example test vet fmt check

build: web
	go build -trimpath -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)" -o dist/tracelet ./cmd/tracelet

web:
	npm --prefix web run build

web-check:
	npm --prefix web run typecheck
	npm --prefix web run lint
	npm --prefix web test

example:
	go build -o dist/tracelet-go-api ./examples/go-api

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w cmd internal examples web/*.go

check:
	@test -z "$$(gofmt -l cmd internal examples web/*.go)" || (gofmt -l cmd internal examples web/*.go; exit 1)
	go vet ./...
	go test ./...
