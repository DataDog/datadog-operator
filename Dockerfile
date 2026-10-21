#
ARG FIPS_ENABLED=false

# Build the manager binary
FROM golang:1.26.10 AS builder

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

# Distrust roots used by compromised eMudhra intermediates until UBI ships an updated CA bundle.
# https://emudhra.we-have-a-problem.com/#linux
RUN trust extract --overwrite --format=pem-bundle \
      --filter='pkcs11:object=Comodo%20AAA%20Services%20root;type=cert' \
      /etc/pki/ca-trust/source/blocklist/Comodo_AAA_Services_root.pem && \
    trust extract --overwrite --format=pem-bundle \
      --filter='pkcs11:object=emSign%20ECC%20Root%20CA%20-%20G3;type=cert' \
      /etc/pki/ca-trust/source/blocklist/emSign_ECC_Root_CA_-_G3.pem && \
    update-ca-trust

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
COPY --from=builder --chown=:0 /workspace/manager .

COPY --from=builder --chown=:0 /workspace/helpers .
COPY --chmod=550 scripts/readsecret.sh .

COPY --from=builder --chown=:0 /workspace/yaml-mapper .

COPY --chmod=644 --chown=:0 ./LICENSE ./LICENSE-3rdparty.csv /licenses/

USER 1001

ENTRYPOINT ["/manager"]
