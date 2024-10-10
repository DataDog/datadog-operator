# Build the manager binary
FROM golang:1.22.7 AS builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/controller/ internal/controller/
COPY pkg/ pkg/
COPY cmd/helpers/ cmd/helpers/

# Build
ARG LDFLAGS
ARG GOARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${GOARCH} GO111MODULE=on go build -a -ldflags "${LDFLAGS}" -o manager cmd/main.go
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

COPY --from=builder /workspace/helpers .
COPY scripts/readsecret.sh .
RUN chmod 550 readsecret.sh && chmod 550 helpers

RUN mkdir -p /licences
COPY ./LICENSE ./LICENSE-3rdparty.csv /licenses/
RUN chmod -R 755 /licences

USER 1001

ENTRYPOINT ["/manager"]
