# Build the manager binary
FROM golang:1.26.11 AS builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
COPY go.work go.work
COPY go.work.sum go.work.sum

COPY api/go.mod api/go.mod
COPY api/go.sum api/go.sum

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/check-operator/ cmd/check-operator/
COPY api/ api/
COPY pkg/ pkg/

# Build
ARG LDFLAGS
ARG GOARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${GOARCH} go build -a -ldflags "${LDFLAGS}" -o check-operator cmd/check-operator/main.go

FROM registry.access.redhat.com/ubi10/ubi-micro:10.3-1794483856@sha256:2ad0f98cb13160c4773c945ef93f88b25831c7f27cd3780d72a7e78b210ea72b
WORKDIR /
COPY --from=builder /workspace/check-operator .
USER 1001

ENTRYPOINT ["/check-operator"]
