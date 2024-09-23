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

Apply the `examples/datadogagent/datadog-agent-minimum.yaml` file which contains the mininum configuration needed to deploy the Agent and related services.

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


kubectl -n $KUBE_NAMESPACE  apply -f examples/datadogagent/datadog-agent-minimum.yaml
```


The Operator should start deploying the `agent` and `cluster-agent`.


## Tests

### Unit and Integration Tests

```shell
# Run unit tests and integration tests
$ make test

# Run v1 integration tests
$ make integration-tests

# Run v2 integration tests
$ make integration-tests-v2
```

### End-to-End Tests

The Datadog Operator end-to-end (E2E) tests run on [Pulumi][pulumi]-deployed test infrastructures, defined as "stacks". The test infrastructures are deployed using the [`test-infra-definitions`][test-infra-repo] and [`datadog-agent`][agent-e2e-source] E2E frameworks.

**Prerequisites**

Internal Datadog users may run E2E locally after completing the following prerequisites:

* Access to the AWS `agent-sandbox` account
* AWS keypair with your public SSH key created in the `agent-sandbox` account
* Set environment variable `PULUMI_CONFIG_PASSPHRASE`
* Complete steps 1-4 of the `test-infra-definitions` [Quick start guide][test-infra-quickstart]
* Add `go.work` file to root directory:

```shell
go work init
go work use . ./test/e2e
```

#### Run E2E Tests

```shell
# Run E2E tests and destroy environment stacks after tests complete.
$ aws-vault exec sso-agent-sandbox-account-admin -- make e2e-tests

# Run E2E tests and keep environment stacks running.
$ aws-vault exec sso-agent-sandbox-account-admin -- make e2e-tests-keep-stacks

# Run E2E tests with K8S_VERSION and IMG environment variables.
$ K8S_VERSION=1.25 IMG=your-dockerhub/operator:tag aws-vault exec sso-agent-sandbox-account-admin -- make e2e-tests

# Run E2E tests with K8S_VERSION, IMG, and IMAGE_PULL_PASSWORD environment variables (for pulling operator image from a private registry).
$ K8S_VERSION=1.25 IMG=669783387624.dkr.ecr.us-east-1.amazonaws.com/operator:PIPELINE_ID-COMMIT_HASH IMAGE_PULL_PASSWORD=$(aws-vault exec sso-agent-qa-read-only -- aws ecr get-login-password) aws-vault exec sso-agent-sandbox-account-admin -- make e2e-tests
```


[pulumi]:https://www.pulumi.com/
[test-infra-repo]:https://github.com/DataDog/test-infra-definitions
[agent-e2e-source]:https://github.com/DataDog/datadog-agent/tree/main/test/new-e2e
[test-infra-quickstart]:https://github.com/DataDog/test-infra-definitions#quick-start-guide