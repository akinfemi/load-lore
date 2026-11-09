BIN       := bin/loadl
PKG       := github.com/akinfemi/loadlore
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS   := -s -w -X $(PKG)/internal/version.Version=$(VERSION) -X $(PKG)/internal/version.Commit=$(COMMIT)
GOLANGCI  ?= golangci-lint

.PHONY: build test lint e2e golden tidy clean

build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/loadl

test:
	go test -race ./...

lint:
	$(GOLANGCI) run ./...

# End-to-end tests need Docker; they are behind the e2e build tag.
e2e:
	go test -race -tags e2e -timeout 30m ./...

# Regenerate golden files after an intended output change.
golden:
	go test ./... -run Golden -update

tidy:
	go mod tidy

clean:
	rm -rf bin
