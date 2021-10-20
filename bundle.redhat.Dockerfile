FROM scratch

# Core bundle labels.
LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=datadog-operator-certified
LABEL operators.operatorframework.io.bundle.channels.v1=alpha
LABEL operators.operatorframework.io.bundle.channel.default.v1=alpha
LABEL operators.operatorframework.io.metrics.builder=operator-sdk-v1.4.0+git
LABEL operators.operatorframework.io.metrics.project_layout=go.kubebuilder.io/v2
LABEL operators.operatorframework.io.metrics.mediatype.v1=metrics+v1

# Labels for testing.
LABEL operators.operatorframework.io.test.mediatype.v1=scorecard+v1
LABEL operators.operatorframework.io.test.config.v1=tests/scorecard/

# Copy files to locations specified by labels.
COPY bundle-redhat/manifests /manifests/
COPY bundle-redhat/metadata /metadata/
COPY bundle-redhat/tests/scorecard /tests/scorecard/
# RedHat OpenShift specific labels
# Specify which OpenShift version we support
LABEL com.redhat.openshift.versions="v4.5-v4.9"
LABEL com.redhat.delivery.operator.bundle=true
# Specify that we are compatible with OpenShift <= 4.4
LABEL com.redhat.delivery.backport=true
