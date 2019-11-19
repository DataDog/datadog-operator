# How to contribute

This project uses the ```go module```. Be sure to have it activated: ```export GO111MODULE=on```.

```console
$ make build
CGO_ENABLED=0 go build -i -installsuffix cgo -ldflags '-w' -o controller ./cmd/manager/main.go

# unit-tests
$ make test

# linter validation
$ make validate

# e2e test
$ make KINDPUSH=true e2e

# build you own docker image
$ make TAG=latest container
```