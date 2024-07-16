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
CHANNELS=stable
DEFAULT_CHANNEL=stable
GOARCH?=
PLATFORM=$(shell uname -s | tr '[:upper:]' '[:lower:]')-$(shell uname -m)
ROOT=$(dir $(abspath $(firstword $(MAKEFILE_LIST))))
KUSTOMIZE_CONFIG?=config/default

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
IMG_CHECK ?= gcr.io/datadoghq/operator-check:latest

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.24

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

##@ Development

.PHONY: all
all: build test ## Build test

.PHONY: build
build: manager kubectl-datadog ## Builds manager + kubectl plugin

.PHONY: fmt
fmt: ## Run go fmt against code
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code
	go vet ./...

.PHONY: echo-img
echo-img: ## Use `make -s echo-img` to get image string for other shell commands
	$(info $(IMG))

##@ Tools
CONTROLLER_GEN = bin/$(PLATFORM)/controller-gen
$(CONTROLLER_GEN): Makefile  ## Download controller-gen locally if necessary.
	$(call go-get-tool,$@,sigs.k8s.io/controller-tools/cmd/controller-gen@v0.14.0)

KUSTOMIZE = bin/$(PLATFORM)/kustomize
$(KUSTOMIZE): Makefile  ## Download kustomize locally if necessary.
	$(call go-get-tool,$@,sigs.k8s.io/kustomize/kustomize/v4@v4.5.7)

ENVTEST = bin/$(PLATFORM)/setup-envtest
$(ENVTEST): Makefile ## Download envtest-setup locally if necessary.
	$(call go-get-tool,$@,sigs.k8s.io/controller-runtime/tools/setup-envtest@v0.0.0-20240320141353-395cfc7486e6)

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin/$(PLATFORM) go install $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

##@ Deploy

.PHONY: manager
manager: generate lint managergobuild ## Build manager binary
	go build -ldflags '${LDFLAGS}' -o bin/$(PLATFORM)/manager main.go
managergobuild: ## Builds only manager go binary
	go build -ldflags '${LDFLAGS}' -o bin/$(PLATFORM)/manager main.go

##@ Deploy

manager: generate lint managergobuild ## Build manager binary

.PHONY: run
run: generate lint manifests ## Run against the configured Kubernetes cluster in ~/.kube/config
	go run ./main.go

.PHONY: install
install: manifests $(KUSTOMIZE) ## Install CRDs into a cluster
	$(KUSTOMIZE) build config/crd | kubectl apply --force-conflicts --server-side -f -

.PHONY: uninstall
uninstall: manifests $(KUSTOMIZE) ## Uninstall CRDs from a cluster
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

.PHONY: deploy
deploy: manifests $(KUSTOMIZE) ## Deploy controller in the configured Kubernetes cluster in ~/.kube/config
	cd config/manager && $(ROOT)/$(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build $(KUSTOMIZE_CONFIG) | kubectl apply --force-conflicts --server-side -f -

.PHONY: undeploy
undeploy: $(KUSTOMIZE) ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build $(KUSTOMIZE_CONFIG) | kubectl delete -f -

.PHONY: manifests
manifests: generate-manifests patch-crds ## Generate manifestcd s e.g. CRD, RBAC etc.

.PHONY: generate-manifests
generate-manifests: $(CONTROLLER_GEN)
	$(CONTROLLER_GEN) crd:crdVersions=v1 rbac:roleName=manager-role paths="./apis/..." paths="./controllers/..." output:crd:artifacts:config=config/crd/bases/v1

.PHONY: generate
generate: $(CONTROLLER_GEN) generate-openapi generate-docs ## Generate code
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./apis/..."

.PHONY: generate-docs
generate-docs: manifests
	go run ./hack/generate-docs.go

# Build the docker images, for local use
.PHONY: docker-build
docker-build: generate docker-build-ci docker-build-check-ci

# For local use
.PHONY: docker-build-ci
docker-build-ci:
	docker build . -t ${IMG} --build-arg LDFLAGS="${LDFLAGS}" --build-arg GOARCH="${GOARCH}"

# For local use
.PHONY: docker-build-check-ci
docker-build-check-ci:
	docker build . -t ${IMG_CHECK} -f check-operator.Dockerfile --build-arg LDFLAGS="${LDFLAGS}" --build-arg GOARCH="${GOARCH}"


# For Gitlab use
.PHONY: docker-build-push-ci
docker-build-push-ci:
	docker buildx build . -t ${IMG} --build-arg LDFLAGS="${LDFLAGS}" --build-arg GOARCH="${GOARCH}" --platform=linux/${GOARCH} --provenance=false --push

# For Gitlab use
.PHONY: docker-build-push-check-ci
docker-build-push-check-ci:
	docker buildx build . -t ${IMG_CHECK} -f check-operator.Dockerfile --build-arg LDFLAGS="${LDFLAGS}" --build-arg GOARCH="${GOARCH}" --platform=linux/${GOARCH} --push

# Push the docker images
.PHONY: docker-push
docker-push: docker-push-img docker-push-check-img

.PHONY: docker-push-img
docker-push-img:
	docker push ${IMG}

.PHONY: docker-push-check-img
docker-push-check-img:
	docker push ${IMG_CHECK}

##@ Test

.PHONY: test
test: build manifests generate fmt vet verify-licenses gotest integration-tests integration-tests-v2 ## Run unit tests and integration tests

.PHONY: gotest
gotest:
	go test ./... -coverprofile cover.out

.PHONY: integration-tests
integration-tests: $(ENVTEST) ## Run tests.
	KUBEBUILDER_ASSETS="$(ROOT)/bin/$(PLATFORM)/" go test --tags=integration github.com/DataDog/datadog-operator/controllers -coverprofile cover_integration_v1.out

.PHONY: integration-tests-v2
integration-tests-v2: $(ENVTEST) ## Run tests with reconciler V2
	KUBEBUILDER_ASSETS="$(ROOT)/bin/$(PLATFORM)/" go test --tags=integration_v2 github.com/DataDog/datadog-operator/controllers -coverprofile cover_integration_v2.out

.PHONY: e2e-tests
e2e-tests: manifests $(KUSTOMIZE) ## Run E2E tests and destroy environment stacks after tests complete. To run locally, complete pre-reqs (see docs/how-to-contribute.md) and prepend command with `aws-vault exec sso-agent-sandbox-account-admin --`. E.g. `aws-vault exec sso-agent-sandbox-account-admin -- make e2e-tests`.
	cd config/e2e && $(ROOT)/$(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/e2e
	KUBEBUILDER_ASSETS="$(ROOT)/bin/$(PLATFORM)/" go test -C test/e2e --tags=e2e github.com/DataDog/datadog-operator/e2e -v -timeout 1h -coverprofile cover_e2e.out

.PHONY: e2e-tests-keep-stacks
e2e-tests-keep-stacks: manifests $(KUSTOMIZE) ## Run E2E tests and keep environment stacks running. To run locally, complete pre-reqs (see docs/how-to-contribute.md) and prepend command with `aws-vault exec sso-agent-sandbox-account-admin --`. E.g. `aws-vault exec sso-agent-sandbox-account-admin -- make e2e-tests-keep-stacks`.
	cd config/e2e && $(ROOT)/$(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/e2e
	KUBEBUILDER_ASSETS="$(ROOT)/bin/$(PLATFORM)/" go test -C test/e2e --tags=e2e github.com/DataDog/datadog-operator/e2e -v -timeout 1h -coverprofile cover_e2e_keep_stacks.out -args -keep-stacks=true

.PHONY: bundle
bundle: bin/$(PLATFORM)/operator-sdk bin/$(PLATFORM)/yq $(KUSTOMIZE) manifests ## Generate bundle manifests and metadata, then validate generated files.
	bin/$(PLATFORM)/operator-sdk generate kustomize manifests --apis-dir ./apis -q
	cd config/manager && $(ROOT)/$(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | bin/$(PLATFORM)/operator-sdk generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	hack/patch-bundle.sh
	bin/$(PLATFORM)/operator-sdk bundle validate ./bundle

# Require Skopeo installed
# And to download token from https://console.redhat.com/openshift/downloads#tool-pull-secret saved to ~/.redhat/auths.json
.PHONY: bundle-redhat
bundle-redhat: bin/$(PLATFORM)/operator-manifest-tools
	hack/redhat-bundle.sh

# Build and push the multiarch bundle image.
.PHONY: bundle-build-push
bundle-build-push:
	docker buildx build --platform linux/amd64,linux/arm64 --push -f bundle.Dockerfile -t $(BUNDLE_IMG) .

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
install-tools: bin/$(PLATFORM)/golangci-lint bin/$(PLATFORM)/operator-sdk bin/$(PLATFORM)/yq bin/$(PLATFORM)/kubebuilder bin/$(PLATFORM)/kubebuilder-tools bin/$(PLATFORM)/go-licenses bin/$(PLATFORM)/openapi-gen

.PHONY: generate-openapi
generate-openapi: bin/$(PLATFORM)/openapi-gen
	bin/$(PLATFORM)/openapi-gen --logtostderr=true -o "./" -i ./apis/datadoghq/v1alpha1 -O zz_generated.openapi -p ./apis/datadoghq/v1alpha1 -h ./hack/boilerplate.go.txt -r "-"
	bin/$(PLATFORM)/openapi-gen --logtostderr=true -o "./" -i ./apis/datadoghq/v2alpha1 -O zz_generated.openapi -p ./apis/datadoghq/v2alpha1 -h ./hack/boilerplate.go.txt -r "-"

.PHONY: preflight-redhat-container
preflight-redhat-container: bin/$(PLATFORM)/preflight
	bin/$(PLATFORM)/preflight check container ${IMG} -d ~/.docker/config.json

# Runs only on Linux and requires `docker login` to scan.connect.redhat.com
.PHONY: preflight-redhat-container-submit
preflight-redhat-container-submit: bin/$(PLATFORM)/preflight
	bin/$(PLATFORM)/preflight check container ${IMG} --submit --pyxis-api-token=${RH_PARTNER_API_TOKEN} --certification-project-id=${RH_PARTNER_PROJECT_ID} -d ~/.docker/config.json

.PHONY: patch-crds
patch-crds: bin/$(PLATFORM)/yq ## Patch-crds
	hack/patch-crds.sh

.PHONY: lint
lint: bin/$(PLATFORM)/golangci-lint fmt vet ## Lint
	bin/$(PLATFORM)/golangci-lint run ./...

.PHONY: licenses
licenses: bin/$(PLATFORM)/go-licenses
	./bin/$(PLATFORM)/go-licenses report . --template ./hack/licenses.tpl > LICENSE-3rdparty.csv 2> errors

.PHONY: verify-licenses
verify-licenses: bin/$(PLATFORM)/go-licenses ## Verify licenses
	hack/verify-licenses.sh

.PHONY: tidy
tidy: ## Run go tidy
	go mod tidy -v

kubectl-datadog: lint
	go build -ldflags '${LDFLAGS}' -o bin/kubectl-datadog ./cmd/kubectl-datadog/main.go

.PHONY: check-operator
check-operator: fmt vet lint
	go build -ldflags '${LDFLAGS}' -o bin/check-operator ./cmd/check-operator/main.go

.PHONY: publish-community-bundles
publish-community-bundles: ## Publish bundles to community repositories
	hack/publish-community-bundles.sh

bin/$(PLATFORM)/yq: Makefile
	hack/install-yq.sh v4.31.2

bin/$(PLATFORM)/golangci-lint: Makefile
	hack/golangci-lint.sh -b "bin/$(PLATFORM)" v1.56.0

bin/$(PLATFORM)/operator-sdk: Makefile
	hack/install-operator-sdk.sh v1.34.1

bin/$(PLATFORM)/go-licenses:
	mkdir -p $(ROOT)/bin/$(PLATFORM)
	GOBIN=$(ROOT)/bin/$(PLATFORM) go install github.com/google/go-licenses@v1.5.0

bin/$(PLATFORM)/operator-manifest-tools: Makefile
	hack/install-operator-manifest-tools.sh 0.6.0

bin/$(PLATFORM)/preflight: Makefile
	hack/install-openshift-preflight.sh 1.9.9

bin/$(PLATFORM)/openapi-gen:
	mkdir -p $(ROOT)/bin/$(PLATFORM)
	GOBIN=$(ROOT)/bin/$(PLATFORM) go install k8s.io/kube-openapi/cmd/openapi-gen@v0.0.0-20230717233707-2695361300d9

bin/$(PLATFORM)/kubebuilder:
	./hack/install-kubebuilder.sh 3.13.0 ./bin/$(PLATFORM)

bin/$(PLATFORM)/kubebuilder-tools:
	./hack/install-kubebuilder-tools.sh 1.28.3 ./bin/$(PLATFORM)

.DEFAULT_GOAL := help
.PHONY: help
help: ## Show this help screen.
	@echo 'Usage: make <OPTIONS> ... <TARGETS>'
	@echo ''
	@echo 'Available targets are:'
	@echo ''
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
