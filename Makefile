SHELL := /bin/bash
export GOCACHE ?= /tmp/reforge-go-build
export GOMODCACHE ?= /tmp/reforge-go-mod
export GOPATH ?= /tmp/reforge-go
export GOTOOLCHAIN := auto
GO := go

.PHONY: dev generate build check test test-integration test-e2e qualify migrate restore-drill install-check

dev: build
	bash scripts/dev.sh

generate:
	bash scripts/generate.sh

build:
	npm --prefix web run build
	$(GO) build -trimpath -o bin/reforge ./cmd/server
	$(GO) build -trimpath -o bin/reforge-migrate ./cmd/migrate
	@if test -d cmd/runner; then $(GO) build -trimpath -o bin/reforge-runner ./cmd/runner; fi

check:
	@test -z "$$(gofmt -l $$(rg --files cmd internal test -g '*.go'))" || (echo 'Go formatting drift'; exit 1)
	$(GO) vet ./...
	npm --prefix web run build
	bash scripts/generate.sh --check

test:
	$(GO) test ./...

test-integration:
	bash scripts/test-integration.sh

test-e2e:
	npm --prefix web run test:e2e

qualify:
	bash scripts/qualify.sh

migrate:
	$(GO) run ./cmd/migrate

restore-drill:
	bash scripts/restore-drill.sh

install-check:
	bash scripts/install-check.sh
