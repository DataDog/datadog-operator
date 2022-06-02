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
$ make IMG=test/operator:tes docker-push

# generate manifest from /config and apply to current cluster
make IMG=test/operator:tes deploy
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
