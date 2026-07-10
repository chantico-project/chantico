# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= 0.6.5

# Image REPOSITORY_URL to use all building/pushing image targets
IMG ?= ghcr.io/chantico-project/images/chantico:latest
CHANTICO_DATA_PATH ?= .chantico-persistent-volume
CHANTICO_PERSISTENT_VOLUME_NAME ?= chantico-persistent-volume
CHANTICO_PERSISTENT_VOLUME_CLAIM_NAME ?= chantico-persistent-volume-claim

LOCAL_DEVELOPMENT_STORAGE_CLASS_NAME ?= local-development
LOCAL_DEVELOPMENT_STORAGE ?= 3Gi

SNMP_MOCK_TAG ?= latest
SNMP_MOCK_IMAGE ?= ghcr.io/chantico-project/images/chantico-snmp-mock:$(SNMP_MOCK_TAG)

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
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crds/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate ## Run tests.
	go test ./internal/... -coverprofile cover.out

# Utilize Kind or modify the e2e tests to load the image locally, enabling compatibility with other vendors.
.PHONY: test-e2e  ## Run the e2e tests against a Kind k8s instance that is spun up.
test-e2e:
	go test ./test/e2e/ -v -ginkgo.v

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

##@ Cluster management

.PHONY: cluster-create-mount
cluster-create-mount: ## Create data path for volume mount (this is sometimes needed for daemon based setups, so the folder is owned by the user, rather than root)
	mkdir $(CHANTICO_DATA_PATH) || true

.PHONE: cluster-delete-mount
cluster-delete-mount: ## Remove data path for volume mount
	rm -rf $(CHANTICO_DATA_PATH) || true

.PHONY: cluster-up
cluster-up: kind cluster-create-mount ## Create Kind cluster
	$(KIND) create cluster --config ./dev/kind-config.yaml


.PHONY: cluster-down
cluster-down: kind ## Delete Kind cluster
	$(KIND) delete cluster || true

.PHONY: cluster-clean
cluster-clean: cluster-down cluster-delete-mount ## Delete Kind cluster and volume mount

.PHONY: cluster-configure
cluster-configure: sync-deployment-crds ## Configure cluster with namespace, helm installation and snmp mock
# 	idempotent function to create namespace
	$(KUBECTL) create namespace chantico --dry-run=client -o yaml | $(KUBECTL) apply -f -
# 	idempotent helm installation
	helm upgrade --install chantico ./chart/ \
		--namespace chantico \
		--set controller.include=false \
    	--set securityContext.runAsUser="$(shell id -u)" \
		--set securityContext.runAsGroup="$(shell id -g)" \
		--set persistentVolumeClaimName=$(CHANTICO_PERSISTENT_VOLUME_CLAIM_NAME) \
		--set pvc.storageClassName=$(LOCAL_DEVELOPMENT_STORAGE_CLASS_NAME) \
		--set pvc.volumeName=$(CHANTICO_PERSISTENT_VOLUME_NAME) \
		--set pv.include=true \
		--set pv.name=$(CHANTICO_PERSISTENT_VOLUME_NAME) \
		--set pv.storage=$(LOCAL_DEVELOPMENT_STORAGE) \
		--set pv.storageClassName=$(LOCAL_DEVELOPMENT_STORAGE_CLASS_NAME) \
		--set pv.hostPath.path="/data/chantico-persistent-volume" \
		--set snmp.service.type="NodePort" \
		--set filebrowser.service.type="NodePort" \
		--set prometheus.service.type="NodePort" \
		--set victoriaMetrics.service.type="NodePort"
	
	$(CONTAINER_TOOL) pull $(SNMP_MOCK_IMAGE)
	$(CONTAINER_TOOL) tag $(SNMP_MOCK_IMAGE) chantico-snmp-mock:latest
	$(KIND) load docker-image chantico-snmp-mock:latest --name kind
	$(KUBECTL) apply -f config/samples/chantico_v1alpha1_physicalmeasurement_mock.yaml
	$(KUBECTL) apply -f dev/k8s/snmp-mock-deployment.yaml
	$(KUBECTL) apply -f dev/k8s/snmp-mock-service.yaml

.PHONY: cluster-mibs
cluster-mibs: ## Copy MIBs to volume. Not tested: maybe we need to wait for the mibs directory to be created?
	cp dev/mibs/* $(CHANTICO_DATA_PATH)/snmp/mibs/

##@ Build

.PHONY: build
build: manifests generate ## Build manager binary.
	go build -o bin/manager cmd/operator/main.go

.PHONY: run
run: manifests generate ## Run a controller from your host.
	go run ./cmd/operator/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

HELM_CHART_DIR ?= chart
GHCR_HELM_REPO ?= oci://ghcr.io/chantico-project/charts

.PHONY: helm-package
helm-package: sync-deployment-crds ## Package Helm chart.
	helm package $(HELM_CHART_DIR)

.PHONY: helm-push
helm-push: helm-package ## Package and push Helm chart to GHCR.
	helm push chantico-$(VERSION).tgz $(GHCR_HELM_REPO)




# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name chantico-builder
	$(CONTAINER_TOOL) buildx use chantico-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm chantico-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/install.yaml

##@ Docs

define DOCS_CHANGELOG_HEADER
---
title: "Changelog"
weight: 60
main:
  parent: technical
  weight: 50
---

endef
export DOCS_CHANGELOG_HEADER

GITHUB_REPOSITORY ?= chantico-project/chantico
REPOSITORY_URL := https://github.com/$(GITHUB_REPOSITORY)
DOCS_DIRECTORY ?= docs
DOCS_CHANGELOG_OUTPUT_PATH := $(DOCS_DIRECTORY)/content/technical/changelog.md
DOCS_PORT := 1313
DOCS_VERBOSE ?= false

ifeq ($(DOCS_VERBOSE),true)
    MUFFET_VERBOSE_FLAG := --verbose
else
    MUFFET_VERBOSE_FLAG :=
endif


.PHONY: docs-build
docs-build: doc2go hugo ## Build the documentation
	@echo "Generating api reference with doc2go..."
	@$(DOC2GO) -embed -highlight classes:monokai \
		-basename _index.html \
		-out $(DOCS_DIRECTORY)/content/technical/api \
		-frontmatter $(DOCS_DIRECTORY)/frontmatter.tmpl \
		-rel-link-style directory \
		-internal ./...

	@echo "Generating $(DOCS_CHANGELOG_OUTPUT_PATH)..."
	@echo "$$DOCS_CHANGELOG_HEADER" > $(DOCS_CHANGELOG_OUTPUT_PATH)
	@sed -E \
		-e "s|^## ([0-9]+\.[0-9]+\.[0-9]+)|## [\1]($(REPOSITORY_URL)/releases/tag/v\1)|" \
	    -e "s|\(([0-9a-f]{7,})\)|([\1]($(REPOSITORY_URL)/commit/\1))|" \
	    -e "s|\(#([1-9][0-9]+)\)|([#\1]($(REPOSITORY_URL)/issues/\1))|" \
		CHANGELOG.md >> $(DOCS_CHANGELOG_OUTPUT_PATH)

	@echo "Building docs with Hugo..."
	@$(HUGO) build --source $(DOCS_DIRECTORY)

.PHONY: docs-serve 
docs-serve: docs-build docs-serve-only ## Build and run the documentation

.PHONY: docs-serve-only
docs-serve-only: ## Run the documentation
	$(HUGO) server serve --source $(DOCS_DIRECTORY) --port $(DOCS_PORT)

.PHONY: docs-test 
docs-test: muffet ## Runs tests against documentation (requires documentation to be hosted at localhost)
	@echo "Running tests..."
	@$(MUFFET) $(MUFFET_VERBOSE_FLAG) --include="http://localhost:$(DOCS_PORT)/chantico" http://localhost:$(DOCS_PORT)/chantico/index.html
	@echo "All tests successful"

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize sync-deployment-crds ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crds | $(KUBECTL) apply -f -
	
.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crds | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: sync-deployment-crds
sync-deployment-crds:
	mkdir -p chart/crds
	cp config/crds/bases/*.yaml chart/crds/
	sed -i'' -e '/^\s*format: int64$$/d' chart/crds/*.yaml

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package REPOSITORY_URL which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
KIND ?= $(LOCALBIN)/kind
HUGO ?= $(LOCALBIN)/hugo
MUFFET ?= $(LOCALBIN)/muffet
DOC2GO ?= $(LOCALBIN)/doc2go

## Tool Versions
KUSTOMIZE_VERSION ?= v5.4.3
CONTROLLER_TOOLS_VERSION ?= v0.19.0
ENVTEST_VERSION ?= release-0.19
GOLANGCI_LINT_VERSION ?= v2.12.2
KIND_VERSION ?= v0.30.0
HUGO_VERSION ?= v0.163.3
MUFFET_VERSION ?= v2.11.2
DOC2GO_VERSION ?= v0.11.0

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: kind
kind: $(KIND) ## Download kind locally if necessary.
$(KIND): $(LOCALBIN)
	$(call go-install-tool,$(KIND),sigs.k8s.io/kind,$(KIND_VERSION))

.PHONY: hugo
hugo: $(HUGO) ## Download hugo locally if necessary.
$(HUGO): $(LOCALBIN)
	$(call go-install-tool,$(HUGO),github.com/gohugoio/hugo,$(HUGO_VERSION))

.PHONY: muffet
muffet: $(MUFFET) ## Download muffet locally if necessary.
$(MUFFET): $(LOCALBIN)
	$(call go-install-tool,$(MUFFET),github.com/raviqqe/muffet/v2,$(MUFFET_VERSION))

.PHONY: doc2go
doc2go: $(DOC2GO) ## Download doc2go locally if necessary.
$(DOC2GO): $(LOCALBIN)
	$(call go-install-tool,$(DOC2GO),go.abhg.dev/doc2go,$(DOC2GO_VERSION))


# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package REPOSITORY_URL which can be installed
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
