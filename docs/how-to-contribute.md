# How to contribute

# Testing the Operator for development

The recommended way to test the Operator is by creating a [kind](https://kind.sigs.k8s.io/) cluster.

To list the available `make` commands, run:

```shell
make help
```

Some important commands:

```shell
# build binary and plugin, and generate CRDs
$ make build

# unit-tests
$ make test

# linter validation
$ make lint

# build docker image defined as {IMG}
$ make IMG=test/operator:test IMG_CHECK=test/operator-check:test docker-build

# push the {IMG} to a configured docker hub
$ make IMG=test/operator:test IMG_CHECK=test/operator-check:test docker-push

# alternatively, if using a kind cluster, the images can be loaded using the `kind` commands
$ kind load docker-image test/operator:test
$ kind load docker-image test/operator-check:test

# generate manifest from /config and apply to current cluster
$ make IMG=test/operator:test IMG_CHECK=test/operator-check:test deploy
```

Notes: 
- If `IMG` is not set, it currently defaults to: `datadog/datadog-operator:latest`

- If testing the webhook, install the cert-manager using:
`$ kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.8.0/cert-manager.yaml`


### Deploy a basic `v2alpha1.DatadogAgent` resource.

Create a secret that contains an `api-key` and an `app-key`. By default the Operator is installed in the
`system` namespace, and only watches resources in this namespace. As a result, the secret and deployment must be within the same namespace.

Apply the `examples/datadogagent/v2alpha1/min.yaml` file which contains the mininum configuration needed to deploy the Agent and related services.

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


kubectl -n $KUBE_NAMESPACE  apply -f examples/datadogagent/v2alpha1/min.yaml
```


The Operator should start deploying the `agent` and `cluster-agent`.
