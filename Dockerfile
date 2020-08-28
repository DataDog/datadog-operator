ARG TAG=0.3.0-rc.3
FROM golang as build-env
ARG TAG

WORKDIR /src
COPY . .
RUN make TAG=$TAG GOMOD="-mod=vendor" build

FROM registry.access.redhat.com/ubi7/ubi-minimal:latest AS final
ARG TAG

LABEL name="datadog/operator"
LABEL vendor="Datadog Inc."
LABEL version=$TAG
LABEL release=$TAG
LABEL summary="The Datadog Operator aims at providing a new way to deploy the Datadog Agent on Kubernetes"
LABEL description="Datadog provides a modern monitoring and analytics platform. Gather \
      metrics, logs and traces for full observability of your Kubernetes cluster with \
      Datadog Operator."

RUN mkdir -p /licences

ENV OPERATOR=/usr/local/bin/datadog-operator \
    USER_UID=1001 \
    USER_NAME=datadog-operator

# install operator binary
COPY --from=build-env /src/controller ${OPERATOR}

COPY ./LICENSE /licenses/LICENSE
COPY ./LICENSE-3rdparty.csv /licenses/LICENSE-3rdparty
RUN chmod -R 755 /licences

COPY --from=build-env /src/build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
