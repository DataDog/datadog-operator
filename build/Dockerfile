FROM registry.access.redhat.com/ubi7/ubi-minimal:latest

ENV OPERATOR=/usr/local/bin/datadog-operator \
    USER_UID=1001 \
    USER_NAME=datadog-operator

# install operator binary
COPY build/_output/bin/datadog-operator ${OPERATOR}

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup
RUN microdnf update && microdnf install curl

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
