CONTROLLER_VERSION ?= 0.0.1
VERSION = $(CONTROLLER_VERSION)
GIT_COMMIT ?= $(shell git rev-list -1 HEAD)

NAME = kubenetmon
IMAGE_TAG_BASE ?= local/$(NAME)
IMG ?= $(IMAGE_TAG_BASE):latest

GOBIN = $(shell go env GOBIN || echo $(shell go env GOPATH)/bin)
SHELL := /usr/bin/env bash -o pipefail

.PHONY: all tidy lint vendor clean build test docker-build integration-test

all: build

tidy: ## Run `go mod tidy`.
	go mod tidy -e

lint: ## Run linters.
	@if ! command -v golangci-lint &> /dev/null && [ ! -x "$(GOBIN)/golangci-lint" ]; then \
		echo "golangci-lint not found. Install it with 'make golangci-lint'"; \
		exit 1; \
	fi
	golangci-lint run --allow-parallel-runners --fast --fix --verbose ./...

vendor: ## Vendor dependencies.
	go mod vendor -v

clean: ## Clean build artifacts.
	rm -rf bin cover.out

build: tidy vendor ## Build binaries (Linux only).
	GOOS=linux go build -o bin/ ./...

test: tidy vendor ## Run unit tests (Linux only).
	CGO_ENABLED=1 GOOS=linux go test -v -race -coverprofile cover.out -tags '!integration' ./...

docker-build: ## Build Docker image.
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		-t $(IMG) .

integration-test: ## Run integration tests.
	./test/test-kind.sh

golangci-lint: ## Install golangci-lint if not available.
	@[ -f $(GOBIN)/golangci-lint ] || { \
		echo "Installing golangci-lint..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	}
