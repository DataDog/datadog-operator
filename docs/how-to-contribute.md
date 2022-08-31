# How to contribute

This project uses the `go module`. Be sure to have it activated with: `export GO111MODULE=on`.

To list the available `make` commands, run:

```shell
make help
```

Some important commands:

```shell
$ make build
CGO_ENABLED=0 go build -i -installsuffix cgo -ldflags '-w' -o controller ./cmd/manager/main.go

# unit-tests
$ make test

# linter validation
$ make lint

# build docker image defined as {IMG}
$ make IMG=test/operator:test docker-build

# push the {IMG} to a configured docker hub
$ make IMG=test/operator:test docker-push

# generate manifest from /config and apply to current cluster
make IMG=test/operator:test deploy
```

Note: `IMG` currently defaults to: `datadog/datadog-operator:latest`

## \[TMP\] how to test `v2alpha`

* Install `cert-manager` needed for the webhook.

```shell
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.8.0/cert-manager.yaml
```

* Deploy with `v2alpha1` enabled and configured as the storage version.

```console
KUSTOMIZE_CONFIG=config/test-v2 make deploy
```
### Deploy a basic `v2alpha1.DatadogAgent` resource.

Create a secret that contains an `api-key` and an `app-key`. By default the Operator is installed in the
`system` namespace, and only watches resources in this namespace. As a result, the secret and deployment must be within the same namespace.

Then apply the `examples/v2alpha1/min.yaml` file which contains the mininum information needed to deploy the agent and related services.

The following commands show how to execute these steps:

```console
kubens system
```

```console
#!/bin/bash

export KUBE_NAMESPACE=system
export DD_API_KEY=<api-key>
export DD_APP_KEY=<app-key>
export DD_TOKEN=<32-chars-token>

kubectl -n $KUBE_NAMESPACE create secret generic datadog-secret --from-literal api-key=$DD_API_KEY --from-literal app-key=$DD_APP_KEY


kubectl -n $KUBE_NAMESPACE create secret generic datadog-token --from-literal token=$DD_TOKEN


kubectl -n $KUBE_NAMESPACE  apply -f examples/v2alpha1/min.yaml
```


The Operator should start deploying the `agent` and `cluster-agent`.
