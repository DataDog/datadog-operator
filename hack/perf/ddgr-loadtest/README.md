# ddgr-loadtest

Perf test driver for the DatadogGenericResource controller.

See spec: `docs/superpowers/specs/2026-05-05-ddgr-perf-test-design.md` (CONTP-1588).

## Build

    go build -o ddgr-loadtest ./hack/perf/ddgr-loadtest

## Setup (once, on a kind cluster)

1. Create the kind cluster and install operator CRDs:

       kind create cluster
       make install   # apply CRDs

2. Build operator image with the profiling/DD_ENV change, load into kind, deploy. `make deploy` puts the operator in the `system` namespace.

       make IMG=controller:latest docker-build
       kind load docker-image controller:latest --name <kind-cluster-name>
       make IMG=controller:latest deploy

3. Create the sandbox creds secret in the `system` namespace (BEFORE the operator rolls out, since `config/manager/manager.yaml` references this secret directly via `valueFrom.secretKeyRef`). Replace with real keys:

       kubectl create secret generic sandbox-datadog-creds \
         --from-literal=api-key=$DD_API_KEY_TEST \
         --from-literal=app-key=$DD_APP_KEY_TEST \
         -n system

4. Apply the DatadogAgent CR (also in `system`):

       kubectl apply -f hack/perf/ddgr-loadtest/manifests/datadogagent.yaml

5. Verify in the sandbox Datadog UI: operator container metrics + profiles (filter `env:ddgr-perf-test`) should appear within ~2 minutes.

The DDGRs themselves are also created in `system` (matches the operator's `WATCH_NAMESPACE`), so no separate namespace YAML is needed.

## Run

Phase 1 — smoke test (always run first):

    ./ddgr-loadtest --count=5 --churn-percent=20 --churn-interval=1m --duration=5m

Verify in the sandbox UI that 5 monitors appear, observe a churn tick patch them (message field changes), and confirm cleanup deletes them.

Phase 2 — full run:

    ./ddgr-loadtest --count=500 --churn-percent=10 --churn-interval=2m --duration=2h

Cleanup-only mode (delete leftover loadtest DDGRs):

    ./ddgr-loadtest --mode=cleanup

## Pass/fail criteria

See spec section "Pass/Fail Criteria". All signals are observed in the sandbox Datadog UI filtered by `env:ddgr-perf-test`.
