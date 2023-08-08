# Custom Operator container images

When adding a custom secret backend implementation or adding a CA certificate, for example, you may need to build a custom Operator image.

## Add a Certificate Authority (CA) certificate

Below is an example of a Dockerfile that's allowed to copy a local CA certificate into a container image based on the Datadog Operator image.

```dockerfile
FROM gcr.io/datadoghq/operator:latest

COPY my-ca-certificat.crt /usr/share/pki/ca-trust-source/anchors/

## update-ca-trust needs to run as root
USER root
RUN update-ca-trust
## Run trust list to check that the certificat was properly added
RUN trust list

# Move back to random user
USER 1001
```