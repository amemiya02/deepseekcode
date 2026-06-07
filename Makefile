.PHONY: build build-web install install-web test lint clean run fmt vet tidy bench-case-study demo-cache demo-cache-offline demo-headtohead cover-cache web web-test desktop ci

BIN_NAME := dsc
BIN_DIR := bin
PKG := github.com/amemiya02/deepseekcode
VERSION := $(shell git describe --tags --always 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -ldflags "-s -w \
	-X $(PKG)/internal/version.Version=$(VERSION) \
	-X $(PKG)/internal/version.Commit=$(COMMIT) \
	-X $(PKG)/internal/version.BuildDate=$(DATE)"

build:
	@mkdir -p $(BIN_DIR)
	go build $(LDFLAGS) -o $(BIN_DIR)/$(BIN_NAME) ./cmd/dsc

install:
	go install $(LDFLAGS) ./cmd/dsc

run: build
	./$(BIN_DIR)/$(BIN_NAME)

GO_PACKAGES := $(shell go list ./... | grep -v /desktop)

test:
	go test $(GO_PACKAGES)

test-race:
	go test -race $(GO_PACKAGES)

cover:
	go test -coverprofile=coverage.out $(GO_PACKAGES)
	go tool cover -html=coverage.out -o coverage.html

lint:
	go vet $(GO_PACKAGES)

fmt:
	gofmt -s -w .

vet:
	go vet $(GO_PACKAGES)

tidy:
	go mod tidy

clean:
	rm -rf $(BIN_DIR) dist coverage.out coverage.html

bench-case-study: build
	@echo "Running case-study benchmark..."
	@go run ./bench/cmd/benchrunner/ --agent deepseekcode-current --task case-study

demo-cache:
	@echo "Running LIVE cache A/B demo on deepseek-v4-flash (needs DEEPSEEK_API_KEY)..."
	@go run ./bench/cmd/cachedemo -live -out bench/cache-demo/results.json

demo-cache-offline:
	@echo "Running offline cache A/B demo (committed fixture, no API key)..."
	@go run ./bench/cmd/cachedemo -fixture bench/cache-demo/results.sample.json -out bench/cache-demo/results.json

demo-headtohead: ## run cache cost head-to-head (offline golden by default)
	go run ./bench/cmd/cachedemo headtohead $(ARGS)

cover-cache:
	go test -cover -coverprofile=coverage-cache.out ./internal/llm/ ./internal/repair/ ./internal/routing/ ./internal/agent/
	go tool cover -func=coverage-cache.out | tail -1

# ---------- Web SPA ----------
web:
	cd web && npm install --legacy-peer-deps && npm run build
	mkdir -p webapp
	rm -rf webapp/dist
	cp -r web/dist webapp/dist

# Build the dsc binary WITH the compiled SPA embedded (-tags withwebapp), so
# `dsc serve --http` serves the real web UI at / instead of the "SPA not
# embedded" stub. Depends on `web` to populate webapp/dist first.
build-web: web
	@mkdir -p $(BIN_DIR)
	go build -tags withwebapp $(LDFLAGS) -o $(BIN_DIR)/$(BIN_NAME) ./cmd/dsc
	@echo "Built $(BIN_DIR)/$(BIN_NAME) with embedded SPA. Run: ./$(BIN_DIR)/$(BIN_NAME) serve --http 127.0.0.1:7432"

# Install the embedded-SPA dsc onto your PATH (go env GOPATH/bin).
install-web: web
	go install -tags withwebapp $(LDFLAGS) ./cmd/dsc

web-test:
	cd web && npm install --legacy-peer-deps && npm test

# ---------- Wails desktop (v3) ----------
# Builds the embedded SPA first (web), then packages the macOS .app using only
# Go + stock macOS tools (sips, iconutil, plutil, codesign) — no wails3 CLI or
# go-task required. Output: bin/DeepSeekCode.app. The CLI-based path stays
# available for anyone who has it installed:
#   cd desktop && wails3 build -tags withwebapp   (or: wails3 package ...)
desktop: web
	bash desktop/package-darwin.sh

# ---------- CI gate ----------
ci: web-test test
	@echo "CI: SPA tests + Go tests passed."
