# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

#
# Datadog custom variables
#
BUILDINFOPKG=github.com/DataDog/datadog-operator/pkg/version
GIT_TAG?=$(shell git tag -l --contains HEAD | tail -1)
TAG_HASH=$(shell git tag | tail -1)_$(shell git rev-parse --short HEAD)
IMG_VERSION?=$(if $(VERSION),$(VERSION),latest)
VERSION?=$(if $(GIT_TAG),$(GIT_TAG),$(TAG_HASH))
GIT_COMMIT?=$(shell git rev-parse HEAD)
DATE=$(shell date +%Y-%m-%d/%H:%M:%S )
LDFLAGS=-w -s -X ${BUILDINFOPKG}.Commit=${GIT_COMMIT} -X ${BUILDINFOPKG}.Version=${VERSION} -X ${BUILDINFOPKG}.BuildTime=${DATE}
CHANNELS=alpha
DEFAULT_CHANNEL=alpha
GOARCH?=amd64

# Default bundle image tag
BUNDLE_IMG ?= controller-bundle:$(VERSION)

# Options for 'bundle-build'
# CHANNELS define the bundle channels used in the bundle.
# Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
# To re-generate a bundle for other specific channels without changing the standard setup, you can:
# - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=candidate,fast,stable)
# - use environment variables to overwrite this value (e.g export CHANNELS="candidate,fast,stable")
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

# DEFAULT_CHANNEL defines the default channel used in the bundle.
# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
# To re-generate a bundle for any other default channel without changing the default setup, you can:
# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# Image URL to use all building/pushing image targets
IMG ?= gcr.io/datadoghq/operator:$(IMG_VERSION)

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true,preserveUnknownFields=false"
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.20

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

##@ Development

all: build test ## Build test

build: manager kubectl-datadog ## Builds manager + kubectl plugin

fmt: ## Run go fmt against code
	go fmt ./...

vet: ## Run go vet against code
	go vet ./...

##@ Deploy

manager: generate lint ## Build manager binary
	go build -ldflags '${LDFLAGS}' -o bin/manager main.go

run: generate lint manifests ## Run against the configured Kubernetes cluster in ~/.kube/config
	go run ./main.go

install: manifests kustomize ## Install CRDs into a cluster
	$(KUSTOMIZE) build config/crd | kubectl apply --force-conflicts --server-side -f -

uninstall: manifests kustomize ## Uninstall CRDs from a cluster
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

deploy: manifests kustomize ## Deploy controller in the configured Kubernetes cluster in ~/.kube/config
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply --force-conflicts --server-side -f -

undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl delete -f -

manifests: generate-manifests patch-crds ## Generate manifestcd s e.g. CRD, RBAC etc.

generate-manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS),crdVersions=v1 rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases/v1
	$(CONTROLLER_GEN) $(CRD_OPTIONS),crdVersions=v1beta1 rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases/v1beta1

generate: controller-gen generate-openapi generate-docs ## Generate code
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

generate-docs: manifests
	go run ./hack/generate-docs.go

docker-build: generate docker-build-ci ## Build the docker image

docker-build-ci:
	docker build . -t ${IMG} --build-arg LDFLAGS="${LDFLAGS}" --build-arg GOARCH="${GOARCH}"

docker-push: ## Push the docker image
	docker push ${IMG}

##@ Test

test: build manifests generate fmt vet verify-license gotest integration-tests ## Run unit tests and E2E tests

gotest:
	go test ./... -coverprofile cover.out

integration-tests: envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test --tags=integration github.com/DataDog/datadog-operator/controllers -coverprofile cover.out

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.6.1)

KUSTOMIZE = $(shell pwd)/bin/kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v3@v3.8.7)

ENVTEST = $(shell pwd)/bin/setup-envtest
envtest: ## Download envtest-setup locally if necessary.
	$(call go-get-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@latest)

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

.PHONY: bundle
bundle: bin/operator-sdk kustomize manifests ## Generate bundle manifests and metadata, then validate generated files.
	./bin/operator-sdk generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | ./bin/operator-sdk generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	./hack/patch-bundle.sh
	./bin/operator-sdk bundle validate ./bundle
	./hack/redhat-bundle.sh

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-redhat-build
bundle-redhat-build:
	docker build -f bundle.redhat.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-push
bundle-push:
	docker push $(BUNDLE_IMG)

.PHONY: opm
OPM = ./bin/opm
opm: ## Download opm locally if necessary.
ifeq (,$(wildcard $(OPM)))
ifeq (,$(shell which opm 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.15.1/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}
else
OPM = $(shell which opm)
endif
endif

# A comma-separated list of bundle images (e.g. make catalog-build BUNDLE_IMGS=example.com/operator-bundle:v0.1.0,example.com/operator-bundle:v0.2.0).
# These images MUST exist in a registry and be pull-able.
BUNDLE_IMGS ?= $(BUNDLE_IMG)

# The image tag given to the resulting catalog image (e.g. make catalog-build CATALOG_IMG=example.com/operator-catalog:v0.2.0).
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:v$(VERSION)

# Set CATALOG_BASE_IMG to an existing catalog image tag to add $BUNDLE_IMGS to that image.
ifneq ($(origin CATALOG_BASE_IMG), undefined)
FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG)
endif

# Build a catalog image by adding bundle images to an empty catalog using the operator package manager tool, 'opm'.
# This recipe invokes 'opm' in 'semver-skippatch' bundle add mode. For more information on add modes, see:
# https://github.com/operator-framework/community-operators/blob/7f1438c/docs/packaging-operator.md#updating-your-existing-operator
.PHONY: catalog-build
catalog-build: opm ## Build a catalog image.
	$(OPM) index add --container-tool docker --mode semver-skippatch --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

# Push the catalog image.
.PHONY: catalog-push
catalog-push: ## Push a catalog image.
	$(MAKE) docker-push IMG=$(CATALOG_IMG)

##@ Datadog Custom part
.PHONY: install-tools
install-tools: bin/golangci-lint bin/operator-sdk bin/yq

.PHONY: generate-openapi
generate-openapi: bin/openapi-gen
	./bin/openapi-gen --logtostderr=true -o "" -i ./apis/datadoghq/v1alpha1 -O zz_generated.openapi -p ./apis/datadoghq/v1alpha1 -h ./hack/boilerplate.go.txt -r "-"
	./bin/openapi-gen --logtostderr=true -o "" -i ./apis/datadoghq/v2alpha1 -O zz_generated.openapi -p ./apis/datadoghq/v2alpha1 -h ./hack/boilerplate.go.txt -r "-"

.PHONY: patch-crds
patch-crds: bin/yq ## Patch-crds
	./hack/patch-crds.sh

.PHONY: lint
lint: vendor bin/golangci-lint fmt vet ## Lint
	./bin/golangci-lint run ./...

.PHONY: license
license: bin/wwhrd vendor
	./hack/license.sh

.PHONY: verify-license
verify-license: bin/wwhrd vendor ## Verify licenses
	./hack/verify-license.sh

.PHONY: tidy
tidy: ## Run go tidy
	go mod tidy -v

.PHONY: vendor
vendor: ## Run go vendor
	go mod vendor

kubectl-datadog: lint
	go build -ldflags '${LDFLAGS}' -o bin/kubectl-datadog ./cmd/kubectl-datadog/main.go

bin/openapi-gen:
	go build -o ./bin/openapi-gen k8s.io/kube-openapi/cmd/openapi-gen

bin/yq:
	./hack/install-yq.sh 3.3.0

bin/golangci-lint:
	hack/golangci-lint.sh v1.38.0

bin/operator-sdk:
	./hack/install-operator-sdk.sh v1.13.1

bin/wwhrd:
	./hack/install-wwhrd.sh 0.2.4

.DEFAULT_GOAL := help
.PHONY: help
help: ## Show this help screen.
	@echo 'Usage: make <OPTIONS> ... <TARGETS>'
	@echo ''
	@echo 'Available targets are:'
	@echo ''
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
