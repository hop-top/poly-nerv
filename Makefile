# Use 'make help' to see available commands.

BIN_DIR    = bin
VERSION   ?= $(shell cat VERSION.txt 2>/dev/null | sed 's/\.$$//' || echo "dev")
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE      ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

GOFLAGS   ?= -buildvcs=false -mod=mod
LDFLAGS   ?= -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

# `go install` writes to $GOBIN if set, else $GOPATH/bin. Compute the
# effective install dir so `make install` prints the right path.
GOBIN      = $(shell go env GOBIN)
GOPATH     = $(shell go env GOPATH)
INSTALL_DIR = $(if $(GOBIN),$(GOBIN),$(GOPATH)/bin)

.PHONY: help all build build-nerv build-nervd codegen \
	proto proto-lint proto-breaking \
	test test-short test-race bench coverage \
	lint lint-fix fmt vet tidy tidy-check links \
	install install-nerv install-nervd \
	setup clean check ci run \
	daemon-start daemon-stop daemon-status \
	12fcc-record 12fcc-grade 12fcc-badge

help: ## Show this help message
	@awk 'BEGIN {FS = ":.*?## "; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} \
		/^[a-zA-Z0-9_-]+:.*?## / { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 }' \
		$(MAKEFILE_LIST)
	@echo

all: lint test build ## Lint, test, then build

## ----- Build -----

build: build-nerv build-nervd ## Build nerv and nervd binaries into bin/

build-nerv: ## Build the full nerv CLI binary
	@mkdir -p $(BIN_DIR)
	@go build $(GOFLAGS) $(LDFLAGS) -o $(BIN_DIR)/nerv ./cmd/nerv
	@echo "built $(BIN_DIR)/nerv"

build-nervd: ## Build the nervd hot-path dispatcher (static, no CGO)
	@mkdir -p $(BIN_DIR)
	@CGO_ENABLED=0 go build $(GOFLAGS) $(LDFLAGS) -o $(BIN_DIR)/nervd ./cmd/nervd
	@echo "built $(BIN_DIR)/nervd"

codegen: ## Run cmd/nerv-codegen to regenerate event constants from spec/events.yaml
	@go run ./cmd/nerv-codegen

## ----- Proto -----

proto: ## Regenerate proto code into every sdk/<lang>/gen tree
	@buf generate

proto-lint: ## Lint protos
	@buf lint

proto-breaking: ## Check breaking changes vs main
	@buf breaking --against '.git#branch=main'

## ----- Install -----

install: install-nerv install-nervd ## Install nerv and nervd to $(INSTALL_DIR)

install-nerv: ## Install nerv to $(INSTALL_DIR)
	@GOFLAGS="$(GOFLAGS)" go install $(LDFLAGS) ./cmd/nerv
	@echo "installed $(INSTALL_DIR)/nerv"

install-nervd: ## Install nervd to $(INSTALL_DIR)
	@CGO_ENABLED=0 GOFLAGS="$(GOFLAGS)" go install $(LDFLAGS) ./cmd/nervd
	@echo "installed $(INSTALL_DIR)/nervd"

## ----- Test -----

test: ## Run all tests with race detector
	@go test -race $(GOFLAGS) ./...

test-short: ## Run all tests in short mode (no race detector)
	@go test -short $(GOFLAGS) ./...

test-race: test ## Alias for test (kept for parity with hop-top siblings)

bench: ## Run benchmarks with memory stats
	@go test -bench=. -benchmem $(GOFLAGS) ./internal/...

coverage: ## Run tests with coverage and print the function-level report
	@go test -cover -coverprofile=coverage.out $(GOFLAGS) ./...
	@go tool cover -func=coverage.out | tail -20
	@go tool cover -html=coverage.out -o coverage.html
	@echo "coverage report: coverage.html"

## ----- Lint / Format -----

lint: ## Run golangci-lint over the whole module
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "golangci-lint not installed. Run: make setup"; \
		exit 1; \
	fi
	@GOFLAGS="-buildvcs=false" golangci-lint run ./...

lint-fix: ## Run golangci-lint with --fix
	@GOFLAGS="-buildvcs=false" golangci-lint run --fix ./...

fmt: ## Run gofmt -s on all Go sources
	@gofmt -s -w .

vet: ## Run go vet
	@go vet $(GOFLAGS) ./...

tidy: ## Run go mod tidy
	@go mod tidy

tidy-check: ## Fail if go.mod or go.sum would change under go mod tidy
	@go mod tidy
	@git diff --exit-code go.mod go.sum

## ----- Setup -----

setup: ## Install dev tooling (golangci-lint) and sync dependencies
	@echo "==> installing golangci-lint"
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0; \
	else \
		echo "golangci-lint already installed: $$(golangci-lint --version 2>&1 | head -1)"; \
	fi
	@echo "==> go mod tidy"
	@go mod tidy
	@echo "setup complete"

## ----- Convenience -----

run: ## Run nerv via 'go run' (pass args via ARGS=...)
	@go run ./cmd/nerv $(ARGS)

check: lint vet test links ## Lint + vet + test + link check (matches CI gate)

links: ## Check relative links across the repo with lychee (offline)
	@if command -v lychee >/dev/null 2>&1; then \
	  lychee --offline --no-progress .; \
	else \
	  echo "lychee not installed; skipping link check"; \
	fi

ci: tidy-check check ## What CI runs locally (tidy-check + lint + vet + test)

clean: ## Remove build artifacts and coverage files
	@rm -rf $(BIN_DIR) dist/
	@rm -f coverage.out coverage.html

## ----- Daemon shortcuts (mirrors old justfile) -----

daemon-start: build-nerv ## Start the nerv daemon via the built CLI
	@./$(BIN_DIR)/nerv daemon start

daemon-stop: ## Stop the nerv daemon (requires --confirm=yes in non-TTY)
	@./$(BIN_DIR)/nerv daemon stop --confirm=yes

daemon-status: ## Print daemon status via the built CLI
	@./$(BIN_DIR)/nerv daemon status

.DEFAULT_GOAL := help

## ----- Conformance grading (12fcc service tier) -----

12fcc-record: ## Re-record conformance cassettes from real binary runs
	@./scripts/12fcc-record.sh

12fcc-grade: ## Grade recorded cassettes (KIT_BIN=<kit with conformance>; STRICT=1 to fail on fail verdicts)
	@./scripts/12fcc-grade.sh

12fcc-badge: ## Regenerate .12fc.json from verify leaves + graded verdicts (KIT_BIN=<kit with conformance>)
	@./scripts/12fcc-badge.sh
