GO      ?= $(shell which go)
OS      ?= $(shell $(GO) env GOOS)
ARCH    ?= $(shell $(GO) env GOARCH)

IMAGE_NAME := tazthemaniac/cert-manager-webhook-infoblox
IMAGE_TAG  ?= $(shell grep '^appVersion:' charts/cert-manager-webhook-infoblox/Chart.yaml 2>/dev/null | awk '{print $$2}' | tr -d '"' || echo "dev")
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null | sed 's/[\/_]/-/g' || echo "local")

# kind
KIND_CLUSTER ?=
NAMESPACE    ?= cert-manager
GROUP_NAME   ?= acme.example.com

BINARY   := webhook
OUT      := $(shell pwd)/_out
BIN_DIR  := $(shell pwd)/bin

# kubernetes / envtest versions
ENVTEST_K8S_VERSION ?= 1.31.x
SETUP_ENVTEST       := $(BIN_DIR)/setup-envtest

# ──────────────────────────────────────────────────────────────────────────────
.PHONY: help
help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

# ──────────────────────────────────────────────────────────────────────────────
# Build
# ──────────────────────────────────────────────────────────────────────────────
.PHONY: build
build: ## Build the webhook binary
	go mod tidy
	CGO_ENABLED=0 go build -o $(BINARY) -ldflags '-w -extldflags "-static"' .

# ──────────────────────────────────────────────────────────────────────────────
# Container
# ──────────────────────────────────────────────────────────────────────────────
.PHONY: docker-build
docker-build: ## Build the Docker image
	docker build -t "$(IMAGE_NAME):$(IMAGE_TAG)" .

.PHONY: docker-push
docker-push: ## Push the Docker image to the registry
	docker push "$(IMAGE_NAME):$(IMAGE_TAG)"

.PHONY: docker-build-push
docker-build-push: docker-build docker-push ## Build and push the Docker image


# ──────────────────────────────────────────────────────────────────────────────
# Security
# ──────────────────────────────────────────────────────────────────────────────
.PHONY: trivy-scan
TRIVY_REPORT := trivy-report.json
trivy-scan: ## Scan the Docker image with Trivy (table output by default; set TRIVY_OUTPUT=1 to save JSON to trivy-report.json)
ifdef TRIVY_OUTPUT
	trivy image --format json --output $(TRIVY_REPORT) "$(IMAGE_NAME):$(IMAGE_TAG)"
	@echo "Trivy report written to $(TRIVY_REPORT)"
else
	trivy image --format table "$(IMAGE_NAME):$(IMAGE_TAG)"
endif

# ──────────────────────────────────────────────────────────────────────────────
# Testing
# ──────────────────────────────────────────────────────────────────────────────
.PHONY: setup-envtest
setup-envtest: $(SETUP_ENVTEST) ## Install setup-envtest for running unit tests

$(SETUP_ENVTEST): | $(BIN_DIR)
	GOBIN=$(BIN_DIR) $(GO) install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

.PHONY: test
test: setup-envtest ## Run unit tests (excludes integration tests)
	@ASSETS="$$($(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" && \
	TEST_ASSET_ETCD="$$ASSETS/etcd" \
	TEST_ASSET_KUBE_APISERVER="$$ASSETS/kube-apiserver" \
	TEST_ASSET_KUBECTL="$$ASSETS/kubectl" \
	$(GO) test -v ./...

.PHONY: test-race
test-race: setup-envtest ## Run unit tests with race detector
	@ASSETS="$$($(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" && \
	TEST_ASSET_ETCD="$$ASSETS/etcd" \
	TEST_ASSET_KUBE_APISERVER="$$ASSETS/kube-apiserver" \
	TEST_ASSET_KUBECTL="$$ASSETS/kubectl" \
	$(GO) test -race -v ./...

.PHONY: test-coverage
test-coverage: setup-envtest ## Run unit tests with coverage report
	@ASSETS="$$($(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" && \
	TEST_ASSET_ETCD="$$ASSETS/etcd" \
	TEST_ASSET_KUBE_APISERVER="$$ASSETS/kube-apiserver" \
	TEST_ASSET_KUBECTL="$$ASSETS/kubectl" \
	$(GO) test -coverprofile=coverage.out -covermode=atomic ./... && \
	$(GO) tool cover -html=coverage.out -o coverage.html

.PHONY: test-integration
test-integration: setup-envtest ## Run the conformance test suite against a real Infoblox GRID
	@if [ -z "$$TEST_ZONE_NAME" ]; then \
		echo "ERROR: TEST_ZONE_NAME must be set (e.g. example.com.)"; exit 1; \
	fi
	@ASSETS="$$($(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" && \
	TEST_ASSET_ETCD="$$ASSETS/etcd" \
	TEST_ASSET_KUBE_APISERVER="$$ASSETS/kube-apiserver" \
	TEST_ASSET_KUBECTL="$$ASSETS/kubectl" \
	$(GO) test -v -tags integration -timeout 5m .

# ──────────────────────────────────────────────────────────────────────────────
# Linting / formatting
# ──────────────────────────────────────────────────────────────────────────────
.PHONY: lint
lint: ## Run golangci-lint (requires golangci-lint to be installed)
	golangci-lint run ./...

.PHONY: fmt
fmt: ## Format all Go source files
	$(GO) fmt ./...

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

# ──────────────────────────────────────────────────────────────────────────────
# Helm
# ──────────────────────────────────────────────────────────────────────────────
.PHONY: helm-lint
helm-lint: ## Lint the Helm chart
	helm lint charts/cert-manager-webhook-infoblox

.PHONY: helm-template
helm-template: | $(OUT) ## Render Helm templates (dry-run)
	helm template cert-manager-webhook-infoblox \
		--set image.repository=$(IMAGE_NAME) \
		--set image.tag=$(IMAGE_TAG) \
		charts/cert-manager-webhook-infoblox > $(OUT)/rendered-manifest.yaml
	@echo "Rendered manifest written to $(OUT)/rendered-manifest.yaml"

# ──────────────────────────────────────────────────────────────────────────────
# kind
# ──────────────────────────────────────────────────────────────────────────────
.PHONY: kind-setup
kind-setup: ## Build, load into kind and install the webhook (KIND_CLUSTER required; NAMESPACE, GROUP_NAME optional)
	KIND_CLUSTER=$(KIND_CLUSTER) NAMESPACE=$(NAMESPACE) GROUP_NAME=$(GROUP_NAME) bash kind/setup.sh

.PHONY: kind-teardown
kind-teardown: ## Uninstall the webhook Helm release from the kind cluster (NAMESPACE optional)
	NAMESPACE=$(NAMESPACE) bash kind/teardown.sh

# ──────────────────────────────────────────────────────────────────────────────
# Housekeeping
# ──────────────────────────────────────────────────────────────────────────────
.PHONY: tidy
tidy: ## Run go mod tidy
	$(GO) mod tidy

.PHONY: clean
clean: ## Remove build artefacts
	rm -rf $(BINARY) $(OUT) $(BIN_DIR) coverage.out coverage.html $(TRIVY_REPORT)

$(OUT) $(BIN_DIR):
	mkdir -p $@
