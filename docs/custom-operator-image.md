# Customer Operator container image

In some case, building a customer Operator image is needed, for example: to add a customer secret backend implementation, or to add CA certificat.

## Add a CA certificat

Here is an example of a Dockerfile allow to copy a local CA certificat into a container image based on the Datadog Operator image

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