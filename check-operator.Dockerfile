# Build the manager binary
FROM golang:1.23.10 AS builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
COPY go.work go.work
COPY go.work.sum go.work.sum

COPY api/go.mod api/go.mod
COPY api/go.sum api/go.sum

COPY test/e2e/go.mod test/e2e/go.mod
COPY test/e2e/go.sum test/e2e/go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download
WORKDIR /workspace/api
RUN go mod download
WORKDIR /workspace/test/e2e
RUN go mod download
WORKDIR /workspace

# Copy the go source
COPY cmd/check-operator/ cmd/check-operator/
COPY api/ api/
COPY pkg/ pkg/

# Build
ARG LDFLAGS
ARG GOARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${GOARCH} GO111MODULE=on go build -a -ldflags "${LDFLAGS}" -o check-operator cmd/check-operator/main.go

FROM registry.access.redhat.com/ubi9/ubi-micro:latest
WORKDIR /
COPY --from=builder /workspace/check-operator .
USER 1001

ENTRYPOINT ["/check-operator"]
