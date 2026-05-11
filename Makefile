SHELL := /bin/bash
GO ?= go
HELM ?= helm

BIN := bin/nightshift-slack-bot
PKG := ./cmd/nightshift-slack-bot

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/nightshiftco/nightshift-slack-bot/internal/version.Version=$(VERSION)

.PHONY: build vet test tidy lint chart-lint chart-template clean

build:
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) $(PKG)

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

tidy:
	$(GO) mod tidy

chart-lint:
	$(HELM) lint deploy/charts/nightshift-slack-bot

chart-template:
	$(HELM) template bug-bot deploy/charts/nightshift-slack-bot \
		--set botName=bug-bot \
		--set userId=00000000-0000-0000-0000-000000000000

clean:
	rm -rf bin dist
