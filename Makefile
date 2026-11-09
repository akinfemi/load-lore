BIN       := bin/loadl
PKG       := github.com/akinfemi/load-lore
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS   := -s -w -X $(PKG)/internal/version.Version=$(VERSION) -X $(PKG)/internal/version.Commit=$(COMMIT)
GOLANGCI  ?= golangci-lint

.PHONY: build test lint integration e2e golden image tidy clean

build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/loadl

test:
	go test -race ./...

lint:
	$(GOLANGCI) run ./...

# Integration tests run real Postgres and Redis containers (testcontainers).
# On Colima: DOCKER_HOST=unix://$$HOME/.colima/loadlore/docker.sock and
# TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock.
integration:
	go test -race -tags integration -timeout 15m ./...

# The loadlore/synthetic image.
image:
	docker build -f cmd/synthetic/Dockerfile -t loadlore/synthetic:$(VERSION) -t loadlore/synthetic:dev .

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
