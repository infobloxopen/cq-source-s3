.PHONY: help build test lint vet tidy clean test-coverage e2e
.DEFAULT_GOAL := help

help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the plugin binary
	go build -o cq-source-s3 .

test: ## Run unit tests
	go test ./...

lint: ## Run golangci-lint
	golangci-lint run

vet: ## Run go vet
	go vet ./...

tidy: ## Tidy go modules
	go mod tidy

clean: ## Remove build artifacts and coverage files
	rm -f cq-source-s3
	rm -f coverage.out coverage.html

test-coverage: ## Run tests with coverage report
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

e2e: ## Run E2E tests against LocalStack (docker-compose)
	docker-compose -f test/docker-compose.yml up -d
	go test -v -count=1 ./test/...
	docker-compose -f test/docker-compose.yml down
