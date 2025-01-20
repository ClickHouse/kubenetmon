CONTROLLER_VERSION ?= 0.0.1
VERSION = $(CONTROLLER_VERSION)
GIT_COMMIT ?= $(shell git rev-list -1 HEAD)

NAME=flowd
IMAGE_TAG_BASE ?= local/$(NAME)
IMG ?= $(IMAGE_TAG_BASE):latest

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	GOOS=linux go vet ./...

.PHONY: lint
lint: golangci-lint ## Run golangci linter against code.
	GOOS=linux $(GOLANGCI_LINT) run --allow-parallel-runners -v

.PHONY: test
test: fmt vet ## Run tests (skips integration tests)
	go test -timeout 900s -v -race $$(go list -buildvcs=false ./... | grep -v 'integrationtest' | grep -v 'flowd/integration') -coverprofile cover.out -covermode=atomic

.PHONY: test-cover
test-cover:
	go tool cover -func=cover.out

.PHONY: test-cover-html
test-cover-html:
	go tool cover -html cover.out

.PHONY: mod
mod: ## Run go mod against code.
	go mod download

.PHONY: tidy
tidy: ## Run go tidy against code.
	go mod tidy -e

.PHONY: build
build: fmt vet lint mod tidy ## Build the binaries.
	go build -o bin/ ./...

.PHONY: run
run: fmt vet lint mod tidy ## Run a controller from your host.
	go run ./cmd/main.go

.PHONY: docker-build
docker-build: fmt vet lint ## Build docker image.
	go mod vendor -v
	docker build \
		--build-arg VERSION=${VERSION} \
		--build-arg GIT_COMMIT=${GIT_COMMIT} \
		-t ${IMG} .

.PHONY: clean
clean: 
	rm -rf bin

.PHONY: helm-render
helm-render: ## Render 
	helm template -f deploy/helm/agent/values.yaml -f deploy/helm/agent/ci/test-values.yaml deploy/helm/agent

GOLANGCI_LINT = $(shell go env GOPATH)/bin/golangci-lint
# Test if golangci-lint is available in the GOPATH, if not, set to local and download if needed
ifneq ($(shell test -f $(GOLANGCI_LINT) && echo -n yes),yes)
GOLANGCI_LINT = $(shell pwd)/bin/golangci-lint
endif
golangci-lint: ## Download the golangci-lint linter locally if necessary.
	$(call go-get-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint@latest)

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go install $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef
