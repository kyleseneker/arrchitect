# Image URL to use all building/pushing image targets
IMG ?= controller:latest

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	"$(CONTROLLER_GEN)" rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

CHART_CRDS = chart/arrchitect/files/crds

.PHONY: sync-crds
sync-crds: manifests ## Copy generated CRDs into the Helm chart, kept on uninstall.
	@mkdir -p $(CHART_CRDS)
	@for f in config/crd/bases/*.yaml; do \
		awk '/^    controller-gen.kubebuilder.io\/version:/ {print "    helm.sh/resource-policy: keep"} {print}' $$f > $(CHART_CRDS)/$$(basename $$f); \
	done
	@echo "CRDs synced into $(CHART_CRDS)"

.PHONY: verify-crds
verify-crds: sync-crds ## Fail if chart CRDs drift from generated CRDs.
	@if [ -n "$$(git status --porcelain $(CHART_CRDS))" ]; then \
		echo "chart CRDs are out of date; run 'make sync-crds' and commit"; \
		git --no-pager diff --stat $(CHART_CRDS); \
		exit 1; \
	fi
	@echo "chart CRDs are up to date"

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	"$(CONTROLLER_GEN)" object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet ## Run tests.
	go test ./... -coverprofile cover.out

.PHONY: test-envtest
test-envtest: manifests generate setup-envtest ## Apply every sample to a real kube-apiserver.
	KUBEBUILDER_ASSETS="$$('$(ENVTEST)' use $(ENVTEST_K8S_VERSION) --bin-dir '$(LOCALBIN)' -p path)" \
		go test ./test/envtest/... -count=1

.PHONY: test-chart
test-chart: helm-unittest ## Run the chart's helm-unittest suites.
	HELM_PLUGINS="$(HELM_PLUGINS)" helm unittest chart/arrchitect

.PHONY: verify-crd-compat
verify-crd-compat: manifests crdify ## Report CRD changes since the last release that existing resources could break on. PREVIOUS_REF= picks another release.
	@prev="$${PREVIOUS_REF:-$$(git describe --tags --abbrev=0 HEAD^ 2>/dev/null)}"; \
	if [ -z "$$prev" ]; then echo "No earlier release to compare with; skipping."; exit 0; fi; \
	echo "Comparing CRDs with $$prev"; status=0; \
	for crd in config/crd/bases/*.yaml; do \
		git cat-file -e "$$prev:$$crd" 2>/dev/null || { echo "$$crd: new since $$prev"; continue; }; \
		out="$$("$(CRDIFY)" "git://$$prev?path=$$crd" "file://$$crd" 2>&1)" || { status=1; printf '\n%s\n%s\n' "$$crd" "$$out"; }; \
	done; \
	exit $$status

.PHONY: test-e2e
test-e2e: kind ## Install the chart into a throwaway kind cluster, with its own kubeconfig, and reconcile a real Sonarr (needs docker).
	$(MAKE) docker-build IMG=arrchitect:e2e
	KIND="$(KIND)" PREVIOUS_RELEASE="$$(git describe --tags --abbrev=0 HEAD^ 2>/dev/null)" \
		go test -tags=e2e ./test/e2e/ -count=1 -v -timeout 30m

.PHONY: test-compat
test-compat: manifests generate setup-envtest ## Reconcile every integration against pinned application containers (needs docker, or CONTAINER_TOOL=podman). Narrow with COMPAT_RUN=TestSonarr.
	KUBEBUILDER_ASSETS="$$('$(ENVTEST)' use $(ENVTEST_K8S_VERSION) --bin-dir '$(LOCALBIN)' -p path)" \
		CONTAINER_TOOL=$(CONTAINER_TOOL) go test -tags=compat ./test/compat/... -count=1 -v -timeout 45m -run '$(COMPAT_RUN)'

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	"$(GOLANGCI_LINT)" run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	"$(GOLANGCI_LINT)" run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	"$(GOLANGCI_LINT)" config verify

##@ Build

BINARIES = operator cli
BINARY ?= operator

.PHONY: build
build: manifests generate fmt vet ## Build every binary.
	$(foreach bin,$(BINARIES),go build -o bin/arrchitect-$(bin) ./cmd/$(bin)/;)

.PHONY: run
run: manifests generate fmt vet ## Run the operator from your host against the cluster in KUBECONFIG, which must be set.
	@test -n "$$KUBECONFIG" || { echo "Set KUBECONFIG to the cluster to run against; run never falls back to your default kubeconfig."; exit 1; }
	go run ./cmd/operator/ --kubeconfig "$$KUBECONFIG"

ARCH ?= $(shell go env GOARCH)

.PHONY: docker-build
docker-build: ## Build the operator's image, or the CLI's with BINARY=cli, for ARCH.
	mkdir -p bin/image/linux/$(ARCH)
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) go build -ldflags "-s -w" -o bin/image/linux/$(ARCH)/arrchitect-$(BINARY) ./cmd/$(BINARY)/
	$(CONTAINER_TOOL) build --platform linux/$(ARCH) --build-arg BINARY=arrchitect-$(BINARY) -f Dockerfile -t ${IMG} bin/image

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p "$(LOCALBIN)"

## Tool Binaries
KIND ?= $(LOCALBIN)/kind
CRDIFY ?= $(LOCALBIN)/crdify
HELM_PLUGINS ?= $(LOCALBIN)/helm-plugins
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint

## Tool Versions
# renovate: datasource=go depName=sigs.k8s.io/kind
KIND_VERSION ?= v0.33.0
# renovate: datasource=go depName=sigs.k8s.io/crdify
CRDIFY_VERSION ?= v0.6.0
# renovate: datasource=github-releases depName=helm-unittest/helm-unittest
HELM_UNITTEST_VERSION ?= v1.2.1
# renovate: datasource=go depName=sigs.k8s.io/controller-tools
CONTROLLER_TOOLS_VERSION ?= v0.23.0

#ENVTEST_VERSION is the version of controller-runtime release branch to fetch the envtest setup script (i.e. release-0.20)
ENVTEST_VERSION ?= $(shell v='$(call gomodver,sigs.k8s.io/controller-runtime)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_VERSION manually (controller-runtime replace has no tag)" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?([0-9]+)\.([0-9]+).*/release-\1.\2/')

#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell v='$(call gomodver,k8s.io/api)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_K8S_VERSION manually (k8s.io/api replace has no tag)" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?[0-9]+\.([0-9]+).*/1.\1/')

# renovate: datasource=github-releases depName=golangci/golangci-lint
GOLANGCI_LINT_VERSION ?= v2.15.0
.PHONY: helm-unittest
helm-unittest: ## Install the helm-unittest plugin into bin if necessary.
	@HELM_PLUGINS="$(HELM_PLUGINS)" helm plugin list 2>/dev/null | grep -Eq "^unittest[[:space:]]+$(HELM_UNITTEST_VERSION:v%=%)[[:space:]]" || { \
		HELM_PLUGINS="$(HELM_PLUGINS)" helm plugin uninstall unittest >/dev/null 2>&1 || true; \
		HELM_PLUGINS="$(HELM_PLUGINS)" helm plugin install https://github.com/helm-unittest/helm-unittest.git \
			--version $(HELM_UNITTEST_VERSION) $$(helm version --short | grep -q '^v4' && echo --verify=false); }

.PHONY: crdify
crdify: $(CRDIFY) ## Download crdify locally if necessary.
$(CRDIFY): $(LOCALBIN)
	$(call go-install-tool,$(CRDIFY),sigs.k8s.io/crdify,$(CRDIFY_VERSION))

.PHONY: kind
kind: $(KIND) ## Download kind locally if necessary.
$(KIND): $(LOCALBIN)
	$(call go-install-tool,$(KIND),sigs.k8s.io/kind,$(KIND_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@"$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))
	@test -f .custom-gcl.yml && { \
		echo "Building custom golangci-lint with plugins..." && \
		$(GOLANGCI_LINT) custom --destination $(LOCALBIN) --name golangci-lint-custom && \
		mv -f $(LOCALBIN)/golangci-lint-custom $(GOLANGCI_LINT); \
	} || true

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] && [ "$$(readlink -- "$(1)" 2>/dev/null)" = "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f "$(1)" ;\
GOBIN="$(LOCALBIN)" go install $${package} ;\
mv "$(LOCALBIN)/$$(basename "$(1)")" "$(1)-$(3)" ;\
} ;\
ln -sf "$$(realpath "$(1)-$(3)")" "$(1)"
endef

define gomodver
$(shell go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' $(1) 2>/dev/null)
endef
