.PHONY: build install test lint clean run fmt vet tidy bench-case-study demo-cache demo-cache-offline cover-cache web web-test desktop

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

test:
	go test ./...

test-race:
	go test -race ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	go vet ./...

fmt:
	gofmt -s -w .

vet:
	go vet ./...

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

cover-cache:
	go test -cover -coverprofile=coverage-cache.out ./internal/llm/ ./internal/repair/ ./internal/routing/ ./internal/agent/
	go tool cover -func=coverage-cache.out | tail -1

# ---------- Web SPA ----------
web:
	cd web && npm install --legacy-peer-deps && npm run build

web-test:
	cd web && npm install --legacy-peer-deps && npm test

# ---------- Wails desktop ----------
desktop: web
	cd desktop && wails build
