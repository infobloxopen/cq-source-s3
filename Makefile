.PHONY: help build test lint vet tidy clean test-coverage e2e docker-build docker-build-multiarch docker-push
.DEFAULT_GOAL := help

# Build metadata
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "development")
GIT_HASH    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
IMAGE_NAME  ?= cq-source-s3
LDFLAGS     := -s -w -X github.com/infobloxopen/cq-source-s3/plugin.Version=$(VERSION)

help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the plugin binary
	go build -trimpath -ldflags="$(LDFLAGS)" -o cq-source-s3 .

test: ## Run unit tests
	go test ./...

lint: ## Run go vet, go fmt check, and go fix check
	go vet ./...
	@test -z "$$(gofmt -l .)" || (echo "Files not formatted:"; gofmt -l .; exit 1)
	go fix ./...

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

docker-build: ## Build Docker image for the current platform
	docker build \
		--build-arg VERSION=$(VERSION) \
		-t $(IMAGE_NAME):$(VERSION)-$(GIT_HASH) \
		-t $(IMAGE_NAME):local \
		.

docker-build-multiarch: ## Build multi-arch Docker image (linux/amd64 + linux/arm64)
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg VERSION=$(VERSION) \
		-t $(IMAGE_NAME):$(VERSION)-$(GIT_HASH) \
		.

docker-push: ## Push Docker image to GCR and GHCR
	@test -n "$(GCP_PROJECT_ID)" || (echo "GCP_PROJECT_ID is required" && exit 1)
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg VERSION=$(VERSION) \
		-t gcr.io/$(GCP_PROJECT_ID)/$(IMAGE_NAME):$(VERSION)-$(GIT_HASH) \
		-t gcr.io/$(GCP_PROJECT_ID)/$(IMAGE_NAME):latest \
		-t ghcr.io/infobloxopen/$(IMAGE_NAME):$(VERSION)-$(GIT_HASH) \
		-t ghcr.io/infobloxopen/$(IMAGE_NAME):latest \
		--push \
		.
