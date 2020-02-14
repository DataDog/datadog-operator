# How to contribute

This project uses the `go module`. Be sure to have it activated with: `export GO111MODULE=on`.

```shell
$ make build
CGO_ENABLED=0 go build -i -installsuffix cgo -ldflags '-w' -o controller ./cmd/manager/main.go

# unit-tests
$ make test

# linter validation
$ make validate

# build you own docker image
$ make TAG=latest container

# build your own docker image and push it in a local Kind cluster
# KIND_CLUSTER_NAME can be omitted if you use Kind default cluster name (i.e. "kind")
$ make TAG=latest KINDPUSH=true KIND_CLUSTER_NAME="mycluster-local" container

# e2e test
$ make e2e
```
