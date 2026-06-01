.PHONY: build install test lint clean run fmt vet tidy bench-case-study

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
