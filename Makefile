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

# Default bundle image tag
BUNDLE_IMG ?= controller-bundle:$(VERSION)
# Options for 'bundle-build'
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# Image URL to use all building/pushing image targets
IMG ?= datadog/operator:$(IMG_VERSION)

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
	$(KUSTOMIZE) build config/crd | kubectl apply -f -


uninstall: manifests kustomize ## Uninstall CRDs from a cluster
	$(KUSTOMIZE) build config/crd | kubectl delete -f -


deploy: manifests kustomize ## Deploy controller in the configured Kubernetes cluster in ~/.kube/config
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

manifests: generate-manifests patch-crds ## Generate manifestcd s e.g. CRD, RBAC etc.

generate-manifests: controller-gen
	$(CONTROLLER_GEN) crd:trivialVersions=true,crdVersions=v1 rbac:roleName=manager webhook paths="./..." output:crd:artifacts:config=config/crd/bases/v1
	$(CONTROLLER_GEN) crd:trivialVersions=true,crdVersions=v1beta1 rbac:roleName=manager webhook paths="./..." output:crd:artifacts:config=config/crd/bases/v1beta1


generate: controller-gen generate-openapi ## Generate code
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."


docker-build: generate docker-build-ci ## Build the docker image

docker-build-ci:
	docker build . -t ${IMG} --build-arg LDFLAGS="${LDFLAGS}"

docker-push: ## Push the docker image
	docker push ${IMG}

##@ Test

test: build manifests verify-license gotest ## Run unit tests and E2E tests

gotest:
	go test ./... -coverprofile cover.out


controller-gen: install-tools ## Find or download controller-gen, download controller-gen if necessary
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.3.0 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

kustomize:
ifeq (, $(shell which kustomize))
	@{ \
	set -e ;\
	KUSTOMIZE_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$KUSTOMIZE_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/kustomize/kustomize/v3@v3.5.4 ;\
	rm -rf $$KUSTOMIZE_GEN_TMP_DIR ;\
	}
KUSTOMIZE=$(GOBIN)/kustomize
else
KUSTOMIZE=$(shell which kustomize)
endif

.PHONY: bundle
bundle: manifests ## Generate bundle manifests and metadata, then validate generated files.
	./bin/operator-sdk generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | ./bin/operator-sdk generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	./bin/operator-sdk bundle validate ./bundle
	./hack/redhat-bundle.sh

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-push
bundle-push:
	docker push $(BUNDLE_IMG)

.PHONY: bundle-redhat-build
bundle-redhat-build:
	docker build -f bundle.redhat.Dockerfile -t scan.connect.redhat.com/ospid-1125a16e-7487-49a2-93ae-f6a21920e804/operator-bundle:$(VERSION) .

##@ Datadog Custom part
.PHONY: install-tools
install-tools: bin/golangci-lint bin/operator-sdk bin/yq bin/kubebuilder

.PHONY: generate-openapi
generate-openapi: bin/openapi-gen
	./bin/openapi-gen --logtostderr=true -o "" -i ./api/v1alpha1 -O zz_generated.openapi -p ./api/v1alpha1 -h ./hack/boilerplate.go.txt -r "-"

.PHONY: patch-crds
patch-crds: bin/yq ## Patch-crds
	./hack/patch-crds.sh

.PHONY: lint
lint: bin/golangci-lint fmt vet ## Lint
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

kubectl-datadog: fmt vet lint
	go build -ldflags '${LDFLAGS}' -o bin/kubectl-datadog ./cmd/kubectl-datadog/main.go

bin/kubebuilder:
	./hack/install-kubebuilder.sh 2.3.1

bin/openapi-gen:
	go build -o ./bin/openapi-gen k8s.io/kube-openapi/cmd/openapi-gen

bin/yq:
	./hack/install-yq.sh 3.3.0

bin/golangci-lint:
	hack/golangci-lint.sh v1.18.0

bin/operator-sdk:
	./hack/install-operator-sdk.sh v1.2.0

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
