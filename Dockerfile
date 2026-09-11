#
ARG FIPS_ENABLED=false

# Pin SGC by digest so released operator images cannot change without review.
# hadolint ignore=DL3026
FROM docker.io/datadog/secret-generic-connector:7.84.0-rc.2@sha256:00a3a7f53ddeaeac7864c41009c8d36c25fe5babeab2498e02c111e83ab27860 AS sgc-false
# hadolint ignore=DL3026
FROM docker.io/datadog/secret-generic-connector:7.84.0-rc.2-fips@sha256:f79d4d94e7c43bfc1152f4b19c9d64a9d6caea9ae64da2bbb8d7dd2483673e6e AS sgc-true
FROM sgc-${FIPS_ENABLED} AS sgc

# Build the manager binary
FROM golang:1.26.7 AS builder

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
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/controller/ internal/controller/
COPY pkg/ pkg/
COPY cmd/helpers/ cmd/helpers/
COPY cmd/yaml-mapper/ cmd/yaml-mapper/

# Build
ARG LDFLAGS
ARG GOARCH
ARG FIPS_ENABLED
RUN echo "FIPS_ENABLED is: $FIPS_ENABLED"
RUN if [ "$FIPS_ENABLED" = "true" ]; then \
    CGO_ENABLED=1 GOEXPERIMENT=boringcrypto GOOS=linux GOARCH=${GOARCH} go build -tags fips -a -ldflags "${LDFLAGS}" -o manager cmd/main.go; \
    else \
    CGO_ENABLED=0 GOOS=linux GOARCH=${GOARCH} go build -a -ldflags "${LDFLAGS}" -o manager cmd/main.go; \
    fi

RUN CGO_ENABLED=0 GOOS=linux GOARCH=${GOARCH} go build -a -ldflags "${LDFLAGS}" -o helpers cmd/helpers/main.go
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${GOARCH} go build -a -ldflags "${LDFLAGS}" -o yaml-mapper cmd/yaml-mapper/main.go

FROM registry.access.redhat.com/ubi10/ubi-minimal:latest AS certs

FROM registry.access.redhat.com/ubi10/ubi-micro:latest

LABEL name="datadog/operator"
LABEL vendor="Datadog Inc."
LABEL summary="The Datadog Operator aims at providing a new way to deploy the Datadog Agent on Kubernetes"
LABEL description="Datadog provides a modern monitoring and analytics platform. Gather \
    metrics, logs and traces for full observability of your Kubernetes cluster with \
    Datadog Operator."
LABEL maintainer="Datadog Inc."

# ubi-micro variant does not have CA certificates installed
COPY --from=certs /etc/pki/tls/certs/ca-bundle.crt /etc/ssl/certs/ca-bundle.crt

WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=sgc --chmod=755 /secret-generic-connector /usr/local/bin/secret-generic-connector

COPY --from=builder --chmod=550 /workspace/helpers .
COPY --chmod=550 scripts/readsecret.sh .

COPY --from=builder --chmod=550 /workspace/yaml-mapper .

COPY --chmod=755 ./LICENSE ./LICENSE-3rdparty.csv /licenses/

USER 1001

ENTRYPOINT ["/manager"]
