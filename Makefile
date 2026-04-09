.PHONY: build run clean test lint install coverage generate check release help

build: ## Build binary to ./bin/contextception
	go build -o bin/contextception ./cmd/contextception

run: build ## Build and run
	./bin/contextception

clean: ## Remove build artifacts
	rm -rf bin/ coverage.out coverage.html

test: ## Run all tests
	go test ./...

lint: ## Run golangci-lint
	golangci-lint run ./...

install: ## Install to $GOPATH/bin
	go install ./cmd/contextception

coverage: ## Run tests with coverage and open HTML report
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

generate: ## Regenerate protocol JSON schemas
	go run ./cmd/gen-schema

release: ## Show release info (use /release in Claude Code for full release)
	go run ./cmd/release info

check: ## Run vet, lint, and tests
	go vet ./...
	golangci-lint run ./...
	go test ./...

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'
