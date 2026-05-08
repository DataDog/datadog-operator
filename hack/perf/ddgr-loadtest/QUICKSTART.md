# Quickstart — local CRD test (no operator)

Verify the binary creates `DatadogGenericResource` CRs the way you'd expect,
without running the operator. CRs sit in etcd unreconciled (no `.status.id`,
no real monitors in Datadog), but the binary's create/patch/delete behavior
is fully observable via `kubectl`.

## Setup

    kind create cluster --name ddgr-test
    make install         # installs the DDGR CRD into the cluster

## Build

    go build -o ddgr-loadtest ./hack/perf/ddgr-loadtest

## Run a small local test

    ./ddgr-loadtest --count=5 --churn-percent=40 --churn-interval=30s --duration=2m

What happens (no operator → no Datadog API calls):

1. Creates namespace `ddgr-loadtest` if missing.
2. **Fill** — creates 5 DDGRs labeled `loadtest=ddgr-perf`, each with
   `spec.type: monitor` and a populated `spec.jsonSpec`.
3. **Churn** — every 30s, patches ~2 DDGRs (bumps `rev=N` in the embedded
   JSON's `message` field).
4. **Cleanup** — after 2m, label-scoped delete. With no operator running,
   no finalizer is added, so deletion is immediate.

## Verify in another terminal

List during the run:

    kubectl get ddgr -n ddgr-loadtest

Inspect one (look for `spec.type: monitor`, valid JSON in `spec.jsonSpec`,
the `loadtest: ddgr-perf` label):

    kubectl get ddgr -n ddgr-loadtest loadtest-0000 -o yaml

Watch the churn mutation — `message` should change after each churn tick:

    kubectl get ddgr -n ddgr-loadtest loadtest-0000 \
      -o jsonpath='{.spec.jsonSpec}' | jq .message

After the binary exits, the namespace should be empty:

    kubectl get ddgr -n ddgr-loadtest

## Cleanup-only (if a previous run left leftovers)

    ./ddgr-loadtest --mode=cleanup
