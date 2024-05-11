#
# Datadog custom variables
#
BUILDINFOPKG=github.com/DataDog/extendeddaemonset/pkg/version
GIT_TAG?=$(shell git tag -l --contains HEAD | tail -1)
TAG_HASH=$(shell git tag | tail -1)_$(shell git rev-parse --short HEAD)
VERSION?=$(if $(GIT_TAG),$(GIT_TAG),$(TAG_HASH))
BUNDLE_VERSION?=$(VERSION:v%=%)
GIT_COMMIT?=$(shell git rev-parse HEAD)
DATE=$(shell date +%Y-%m-%d/%H:%M:%S )
LDFLAGS=-w -s -X ${BUILDINFOPKG}.Commit=${GIT_COMMIT} -X ${BUILDINFOPKG}.Version=${VERSION} -X ${BUILDINFOPKG}.BuildTime=${DATE}
GOARCH?=amd64
ROOT_DIR:=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
KUSTOMIZE = bin/kustomize

# Default bundle image tag
BUNDLE_IMG ?= controller-bundle:$(BUNDLE_VERSION)
# Options for 'bundle-build'
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# Image URL to use all building/pushing image targets
IMG ?= datadog/extendeddaemonset:latest
IMG_CHECK ?= datadog/extendeddaemonset-check:latest

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: build test

build: manager kubectl-eds

# Run tests
test: build manifests verify-license gotest

gotest:
	go test ./... -coverprofile cover.out

e2e: build manifests verify-license goe2e

# Runs e2e tests (expects a configured cluster)
goe2e: bin/kubebuilder-tools
	go test --tags=e2e ./controllers -ginkgo.progress -ginkgo.v -test.v

# Build manager binary
manager: generate lint
	go build -ldflags '${LDFLAGS}' -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate lint manifests
	go run ./main.go

# Install CRDs into a cluster
install: manifests $(KUSTOMIZE)
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests $(KUSTOMIZE)
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests $(KUSTOMIZE)
	cd config/manager && $(ROOT_DIR)/bin/kustomize edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: generate-manifests patch-crds

generate-manifests: controller-gen
	$(CONTROLLER_GEN) crd:trivialVersions=true,crdVersions=v1 rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases/v1
	$(CONTROLLER_GEN) crd:trivialVersions=true,crdVersions=v1beta1 rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases/v1beta1

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen generate-openapi
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Build the docker image
# For use locally
docker-build: generate docker-build-ci docker-build-check-ci

# For use locally
docker-build-ci:
	docker build . -t ${IMG} --build-arg LDFLAGS="${LDFLAGS}" --build-arg GOARCH="${GOARCH}"

# For use locally
docker-build-check-ci:
	docker build . -t ${IMG_CHECK} -f check-eds.Dockerfile --build-arg LDFLAGS="${LDFLAGS}" --build-arg GOARCH="${GOARCH}"


# For use in Gitlab
docker-build-push-ci:
	docker buildx build . -t ${IMG} --build-arg LDFLAGS="${LDFLAGS}" --build-arg GOARCH="${GOARCH}" --platform=linux/${GOARCH} --push

# For use in Gitlab
docker-build-push-check-ci:
	docker buildx build . -t ${IMG_CHECK} -f check-eds.Dockerfile --build-arg LDFLAGS="${LDFLAGS}" --build-arg GOARCH="${GOARCH}" --platform=linux/${GOARCH} --push

# Push the docker images
docker-push: docker-push-img docker-push-check-img

docker-push-img:
	docker push ${IMG}

docker-push-check-img:
	docker push ${IMG_CHECK}

# find or download controller-gen
# download controller-gen if necessary
controller-gen: install-tools
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.3.0 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif


# Generate bundle manifests and metadata, then validate generated files.
.PHONY: bundle
bundle: manifests bin/kustomize
	./bin/operator-sdk generate kustomize manifests -q
	cd config/manager && $(ROOT_DIR)/bin/kustomize edit set image controller=$(IMG)
	./bin/kustomize build config/manifests | ./bin/operator-sdk generate bundle -q --overwrite --version $(BUNDLE_VERSION) $(BUNDLE_METADATA_OPTS)
	./bin/operator-sdk bundle validate ./bundle

# Build the bundle image.
.PHONY: bundle-build
bundle-build:
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

#
# Datadog Custom part
#
.PHONY: install-tools
install-tools: bin/golangci-lint bin/operator-sdk bin/yq bin/kubebuilder bin/kubebuilder-tools

.PHONY: generate-openapi
generate-openapi: bin/openapi-gen
	./bin/openapi-gen --logtostderr=true -o "" -i ./api/v1alpha1 -O zz_generated.openapi -p ./api/v1alpha1 -h ./hack/boilerplate.go.txt -r "-"

.PHONY: patch-crds
patch-crds: bin/yq
	./hack/patch-crds.sh

.PHONY: lint
lint: bin/golangci-lint fmt vet
	./bin/golangci-lint run ./...

.PHONY: license
license: bin/wwhrd vendor
	./hack/license.sh

.PHONY: verify-license
verify-license: bin/wwhrd vendor
	./hack/verify-license.sh

.PHONY: tidy
tidy:
	go mod tidy -v

.PHONY: vendor
vendor:
	go mod vendor

kubectl-eds: fmt vet lint
	CGO_ENABLED=1 go build -ldflags '${LDFLAGS}' -o bin/kubectl-eds ./cmd/kubectl-eds/main.go

check-eds: fmt vet lint
	go build -ldflags '${LDFLAGS}' -o bin/check-eds ./cmd/check-eds/main.go

bin/kubebuilder:
	./hack/install-kubebuilder.sh 3.4.0 ./bin

bin/kubebuilder-tools:
	./hack/install-kubebuilder-tools.sh 1.24.1

bin/openapi-gen:
	go build -o ./bin/openapi-gen k8s.io/kube-openapi/cmd/openapi-gen

bin/yq:
	./hack/install-yq.sh 3.3.0

bin/golangci-lint:
	hack/install-golangci-lint.sh v1.55.2

bin/operator-sdk:
	./hack/install-operator-sdk.sh v1.5.0

bin/wwhrd:
	./hack/install-wwhrd.sh 0.2.4

bin/kustomize:
	./hack/install-kustomize.sh ./bin
