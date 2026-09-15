# Operator QA gotchas

Each entry was hit during real QA runs. Symptom first — that is how you will meet it.

## Environment / setup

**`helm install datadog/datadog-operator --devel` fails with `EOF`**
GitHub release download is flaky. `helm pull` usually works when install does not:
```bash
helm pull datadog/datadog-operator --devel --untar --untardir /tmp/ddop-chart
helm install ddop /tmp/ddop-chart/datadog-operator -n datadog --create-namespace --wait
```
Verify you got the RC: `grep -E '^(version|appVersion)' /tmp/ddop-chart/datadog-operator/Chart.yaml`.

**`zsh: no matches found: env[0].name=DD_FOO`**
zsh globs `[0]`. Quote every indexed `--set`: `--set 'env[0].name=DD_FOO' --set 'env[0].value=true'`.
Same class: `$ref:path` in zsh applies a history modifier — use `"${ref}:path"` with `git show`.

**Agent container crashloops: `Error while getting hostname, exiting: unable to reliably determine the host name`**
kind nodes have no cloud metadata and a non-FQDN hostname. `global.clusterName` does not fix it. Add the `DD_HOSTNAME` override from `spec.nodeName`. Hits every fresh DDA on kind.

**Operator crashloops, `Liveness probe failed: statuscode: 500`, log `too many goroutines: 402 > limit: 400`**
The healthz check is only a goroutine ceiling (default 400). Enabling many features can cause us to surpass the limit.
```bash
helm upgrade ddop <chart> -n datadog --reuse-values --set maximumGoroutines=1000
```
Confirm: `kubectl port-forward -n datadog deploy/ddop-datadog-operator 18383:8383` then `curl -s localhost:18383/metrics | grep '^go_goroutines '`.

**`kubectl exec ... -- curl` fails** — no curl/wget in the operator image. Use `port-forward` + local curl, or bash's `/dev/tcp`:
```bash
kubectl exec -n datadog deploy/ddop-datadog-operator -- \
  bash -c 'exec 3<>/dev/tcp/127.0.0.1/8383; printf "GET /metrics HTTP/1.0\r\nHost: localhost\r\n\r\n" >&3; cat <&3' | grep '^go_goroutines '
```

**helm upgrade refuses to run**
`another operation in progress` → `helm -n datadog rollback ddop` first. `no deployed releases` → use `helm install`, not `upgrade`.

**Need more than one node** (profile targeting, node affinity, DAP spread):
```bash
cat > /tmp/kind-qa.yaml <<'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes: [{role: control-plane}, {role: worker}, {role: worker}]
EOF
kind create cluster --name qa --config /tmp/kind-qa.yaml
```

## Operator model

**Never apply a DatadogAgentInternal directly.** Apply a DatadogAgent; the operator generates and owns the DDAI. `addDependencies` skips install-info, APM-telemetry and credentials when `fromDDAI` is true (the DDA controller owns them), so a hand-made DDAI yields workloads mounting a `<name>-install-info` ConfigMap nothing creates — pods hang in `Init`/`ContainerCreating` forever while the DDAI status still says `reconcile succeed`.

**The DDAI is where rendering happens.** For "what did the operator produce" questions inspect the DaemonSet/Deployment plus `kubectl get datadogagentinternal <name> -o yaml`. The DDAI carries the *defaulted* spec; the DDA carries what the user wrote. That distinction matters for anything about defaulting.

**Transient first-reconcile errors are normal.** On DDA creation expect a burst of `clusterrolebindings ... not found`, `DatadogAgentInternal ... not found`, and DDAI optimistic-lock conflicts, logged at ERROR with stack traces, resolving in ~1–2s. Check timestamps against your action before reporting them.

**Provider annotations.** GKE Autopilot is enabled by `agent.datadoghq.com/cluster-provider: gke-autopilot` **or** `experimental.agent.datadoghq.com/autopilot: "true"`.

**Flags vs env vars.** Unknown CLI flags crash the operator at startup — never invent one. Some settings are env-var only; check `cmd/main.go` for `boolEnv(...)` wiring.

| Setting | Env var | Default |
|---|---|---|
| Controller revisions | `DD_CREATE_CONTROLLER_REVISIONS` | `false` |
| Untaint readiness timeout | `DD_UNTAINT_CONTROLLER_TIMEOUT` | `10m` |
| Untaint scheduling timeout | `DD_UNTAINT_CONTROLLER_SCHEDULING_TIMEOUT` | `5m` |
| Untaint timeout policy | `DD_UNTAINT_CONTROLLER_TIMEOUT_POLICY` | `remove` |
| Untaint events | `DD_UNTAINT_CONTROLLER_EVENTS_ENABLED` | `false` |

Patching the deployment directly is faster than a helm upgrade for a one-off, but a later
`helm upgrade` reverts it — use `--set 'env[0].name=...'` (quoted) for anything that must persist.

**Namespace.** This skill defaults to `datadog`; many operator PR test plans use `system`. Either works — just be consistent, and pass `--ns` / `QA_NS` rather than mixing.

## Correctness of your own measurements

**Reading a workload before the operator reconciled**
Produces confident false results. Two real instances: a DaemonSet reporting the old registry, and a "partial" AppArmor migration that was just a mid-rollout read. If a result is internally inconsistent (Autopilot on but registry is not GCR), suspect staleness before suspecting the product. Settle on a value-specific predicate.

**Recovery looks broken but is backoff**
After ~24 consecutive reconcile failures controller-runtime's backoff reaches ~1000s, so conditions stay stale for many minutes after you fix the cause. Reset it:
```bash
kubectl delete pod -n datadog -l app.kubernetes.io/name=datadog-operator --wait=false
```
Restarting alone is not always enough: an error condition is only recomputed when the reconcile actually does work, so also bump the spec (`bump_dda` in `qalib.sh`) to force a real update.

## Useful probes

```bash
# operator flags actually in effect
kubectl get deploy ddop-datadog-operator -n datadog -o jsonpath='{.spec.template.spec.containers[0].args}' | tr ',' '\n'

# real errors only, minus the expected fake-key noise
kubectl logs -n datadog deploy/ddop-datadog-operator | jq -r 'select(.level=="ERROR")|"\(.ts) \(.msg) :: \(.error//"")"' | grep -v "403 Forbidden"

# what the operator rendered, per container
kubectl get ds datadog-agent -n datadog -o json | jq -c '[.spec.template.spec.containers[]|{name,image,env:[.env[]?|select(.name|test("DD_"))|.name]}]'
```

Fake credentials keep the agent from reporting to Datadog but it still runs. If a test needs
the agent pod to reach `Ready` (e.g. an `agent_ready` code path), drop the probe or ask for an API key:

```bash
kubectl patch ds datadog-agent -n datadog --type=json \
  -p='[{"op":"remove","path":"/spec/template/spec/containers/0/readinessProbe"}]'
```
