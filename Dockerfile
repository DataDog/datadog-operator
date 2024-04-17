# Build the manager binary
FROM golang:1.19 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY apis/ apis/
COPY controllers/ controllers/
COPY pkg/ pkg/
COPY cmd/helpers/ cmd/helpers/

# Build
ARG LDFLAGS
ARG GOARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${GOARCH} GO111MODULE=on go build -a -ldflags "${LDFLAGS}" -o manager main.go
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${GOARCH} GO111MODULE=on go build -a -ldflags "${LDFLAGS}" -o helpers cmd/helpers/main.go

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

LABEL name="datadog/operator"
LABEL vendor="Datadog Inc."
LABEL summary="The Datadog Operator aims at providing a new way to deploy the Datadog Agent on Kubernetes"
LABEL description="Datadog provides a modern monitoring and analytics platform. Gather \
      metrics, logs and traces for full observability of your Kubernetes cluster with \
      Datadog Operator."

WORKDIR /
COPY --from=builder /workspace/manager .

RUN microdnf --nodocs -y install shadow-utils \
  && adduser --no-create-home --uid 1001 dd-operator \
  && addgroup --gid 1002 --users dd-operator secret-manager \
  && microdnf -y remove shadow-utils \
  && microdnf clean all


COPY --chown=1001:1002 --chmod=550 --from=builder /workspace/helpers .
COPY --chown=1001:1002 --chmod=550 scripts/readsecret.sh .

RUN mkdir -p /licences
COPY ./LICENSE ./LICENSE-3rdparty.csv /licenses/
RUN chmod -R 755 /licences

USER 1001

ENTRYPOINT ["/manager"]
