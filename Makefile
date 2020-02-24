PROJECT_NAME=datadog-operator
ARTIFACT=controller
ARTIFACT_PLUGIN=kubectl-datadog

# 0.0 shouldn't clobber any released builds
DOCKER_REGISTRY=datadog
PREFIX=${DOCKER_REGISTRY}/operator
SOURCEDIR="."

SOURCES := $(shell find $(SOURCEDIR) ! -name "*_test.go" -name '*.go')

BUILDINFOPKG=github.com/DataDog/${PROJECT_NAME}/version
GIT_TAG?=$(shell git tag -l --contains HEAD | tail -1)
LAST_TAG=$(or $(shell git tag | tail -1),v0.0.0)
TAG?=${GIT_TAG}
TAG_HASH=${LAST_TAG}_$(shell git rev-parse --short HEAD)
VERSION?=$(if $(GIT_TAG),$(GIT_TAG),$(TAG_HASH))
GIT_COMMIT?=$(shell git rev-parse HEAD)
DATE=$(shell date +%Y-%m-%d/%H:%M:%S )
GOMOD?="-mod=vendor"
LDFLAGS=-ldflags "-w -X ${BUILDINFOPKG}.Tag=${TAG} -X ${BUILDINFOPKG}.Commit=${GIT_COMMIT} -X ${BUILDINFOPKG}.Version=${VERSION} -X ${BUILDINFOPKG}.BuildTime=${DATE} -s"

KIND_CLUSTER_NAME="kind"

export GO111MODULE=on

all: test build

vendor:
	go mod vendor

tidy:
	go mod tidy -v

build: ${ARTIFACT}

${ARTIFACT}: ${SOURCES}
	CGO_ENABLED=0 go build ${GOMOD} -i -installsuffix cgo ${LDFLAGS} -o ${ARTIFACT} ./cmd/manager/main.go

build-plugin: ${ARTIFACT_PLUGIN}

${ARTIFACT_PLUGIN}: ${SOURCES}
	CGO_ENABLED=0 go build -i -installsuffix cgo ${LDFLAGS} -o ${ARTIFACT_PLUGIN} ./cmd/${ARTIFACT_PLUGIN}/main.go

container:
	./bin/operator-sdk build $(PREFIX):$(TAG)
    ifeq ($(KINDPUSH), true)
	 kind load docker-image --name $(KIND_CLUSTER_NAME) $(PREFIX):$(TAG)
    endif

container-ci:
	docker build -t $(PREFIX):$(TAG) --build-arg "TAG=$(TAG)" .

test:
	./go.test.sh

e2e:
	kubectl apply -f deploy/datadoghq_v1alpha1_extendeddaemonset_crd.yaml
	./bin/operator-sdk test local --verbose ./test/e2e --image $(PREFIX):$(TAG)

push: container
	docker push $(PREFIX):$(TAG)

clean:
	rm -f ${ARTIFACT}

validate: bin/golangci-lint bin/wwhrd
	./bin/golangci-lint run ./...
	./hack/verify-license.sh > /dev/null

generate: bin/operator-sdk bin/openapi-gen bin/client-gen bin/informer-gen bin/lister-gen
	./bin/operator-sdk generate k8s
	./bin/operator-sdk generate crds
	./bin/openapi-gen --logtostderr=true -o "" -i ./pkg/apis/datadoghq/v1alpha1 -O zz_generated.openapi -p ./pkg/apis/datadoghq/v1alpha1 -h ./hack/boilerplate.go.txt -r "-"
	./hack/generate-groups.sh client,lister,informer \
  github.com/DataDog/datadog-operator/pkg/generated github.com/DataDog/datadog-operator/pkg/apis datadoghq:v1alpha1 \
  --go-header-file ./hack/boilerplate.go.txt

generate-olm: bin/operator-sdk
	./bin/operator-sdk olm-catalog gen-csv --csv-version $(VERSION) --update-crds

CRDS = $(wildcard deploy/crds/*_crd.yaml)
local-load: $(CRDS)
		for f in $^; do kubectl apply -f $$f; done
		kubectl apply -f deploy/
		kubectl delete pod -l name=${PROJECT_NAME}

$(filter %.yaml,$(files)): %.yaml: %yaml
	kubectl apply -f $@

install-tools: bin/golangci-lint bin/operator-sdk

bin/golangci-lint:
	./hack/golangci-lint.sh v1.18.0

bin/operator-sdk:
	./hack/install-operator-sdk.sh

bin/wwhrd:
	./hack/install-wwhrd.sh

bin/openapi-gen:
	go build -o ./bin/openapi-gen k8s.io/kube-openapi/cmd/openapi-gen

bin/client-gen:
	go build -o ./bin/client-gen ./vendor/k8s.io/code-generator/cmd/client-gen

bin/informer-gen:
	go build -o ./bin/informer-gen ./vendor/k8s.io/code-generator/cmd/informer-gen

bin/lister-gen:
	go build -o ./bin/lister-gen ./vendor/k8s.io/code-generator/cmd/lister-gen

license: bin/wwhrd
	./hack/license.sh

license-verify: bin/wwhrd
	./hack/verify-license.sh

.PHONY: vendor build push clean test e2e validate local-load install-tools list container container-ci license license-verify
