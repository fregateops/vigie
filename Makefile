BINARY     := vigie
CMD        := ./cmd/vigie
BUILD_DIR  := dist
BIN        := $(BUILD_DIR)/$(BINARY)

.PHONY: build run test lint tidy clean setup-pre-commit pre-commit \
        release-dry-run release-build help

build: ## Build the vigie binary into dist/
	go build -o $(BIN) $(CMD)

run: ## Run vigie from source (pass args via ARGS=...)
	go run $(CMD) $(ARGS)

test: pre-commit ## Run pre-commit hooks then the Go test suite
	go test ./...

lint: ## Run golangci-lint
	golangci-lint run ./...

tidy: ## Tidy go.mod / go.sum
	go mod tidy

clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR)

setup-pre-commit: ## Install the git pre-commit hook
	pre-commit install

pre-commit: ## Run all pre-commit hooks against every file
	pre-commit run --all-files

release-dry-run: ## Preview a release locally (requires goreleaser in PATH)
	goreleaser release --snapshot --clean

release-build: ## Build for host OS/arch only via goreleaser snapshot
	goreleaser build --snapshot --clean --single-target

help: ## List all targets with descriptions
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "  \033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
