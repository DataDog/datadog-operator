# Test setup: DDOT in Agent+DDOT and standalone modes

**Goal:** collect node, pod and container tags plus kubelet and cAdvisor metrics with DDOT in both Agent+DDOT and standalone modes, and show that telemetry keeps flowing across the switch. The setup needs only `features.otelCollector.conf`.

The setup is adapted from `guides/kubernetes/configuration/daemonset-collector.yaml` in [DataDog/opentelemetry-examples](https://github.com/DataDog/opentelemetry-examples). The sample app is `apps/kubernetes-express-otel` from the same repo.

## Files

- `dda-standalone-rich.yaml`: a standalone DatadogAgent with the full collector conf and no overrides.
- `dda-agent-ddot-rich.yaml`: the Agent+DDOT version of the same setup, to switch to and from standalone (see below).
- `express-app.yaml`: the express app plus a curl load generator. The app sends traces through the node-local Service, along with its `k8s.pod.uid`.

```sh
docker build -t otel-js-app:1.0 <examples>/apps/kubernetes-express-otel/express-otel-app
kind load docker-image otel-js-app:1.0 --name ddot2
kubectl apply -f dda-standalone-rich.yaml -f express-app.yaml
# switch modes on the same DaemonSet
kubectl apply -f dda-agent-ddot-rich.yaml
kubectl apply -f dda-standalone-rich.yaml
```

## Agent+DDOT version

Applying the standalone conf without the annotation crash-loops otel-agent. Its `k8s_attributes` filter needs `K8S_NODE_NAME`, which the operator only sets in standalone mode. The Agent+DDOT version relies on the core Agent wherever the core Agent already does the job:

| | Standalone | Agent+DDOT |
|---|---|---|
| Workload tags | `k8s_attributes` (Kubernetes API) | `infraattributes` (core Agent tagger; Datadog tag names such as `kube_deployment`, `pod_name`, `container_id`, `kube_service`) |
| Host metrics | none (optional, see below) | core Agent (`system.*`) |
| Kubelet and cAdvisor metrics | DDOT `kubeletstats` and `prometheus/cadvisor` | same in DDOT |
| OTLP, APM stats, self-metrics | DDOT | DDOT |
| Host | `datadog.host.name` = `<node>-<cluster>` | same; this is the core Agent's hostname when a cluster name is set |
| Overrides | none | `K8S_NODE_NAME` env only |

The two confs differ only in the tagging processor:
- **Agent+DDOT:** `infraattributes` asks the core Agent tagger, over IPC, for the tags of the pod or container the telemetry comes from. The core Agent already tracks pods and containers through the kubelet and the Cluster Agent.
- **Standalone:** there is no tagger, so `k8s_attributes` reads pod metadata from the Kubernetes API itself.
- **Same tags in both modes:** `k8s_attributes` could also run in Agent+DDOT. It would need `K8S_NODE_NAME` and the pods/namespaces/replicasets RBAC, which the operator only grants in standalone, and it would duplicate the tagger.
 Both `infraattributes` and `k8s_attributes` find the app's pod through its `k8s.pod.uid`, so the app config is the same in both modes.

**Switch test** (Agent+DDOT → standalone → Agent+DDOT on kind-ddot2):
- The DaemonSet is never recreated.
- DDOT emits the same 42 `kubeletstats` metric names in both modes, plus cAdvisor, APM stats and self-metrics.
- Express spans are fully tagged in both modes.
- Nothing is refused, there are no exporter failures, and there are no ERROR lines.

## What the conf adds

| Area | Component | Result |
|---|---|---|
| Kubelet metrics | `kubeletstats` (`${env:DD_KUBERNETES_KUBELET_HOST}:10250`) | `k8s.node.*`, `k8s.pod.*`, `container.*` and `k8s.volume.*` |
| Container metrics | `prometheus/cadvisor` (kubelet `/metrics/cadvisor`) | `container_cpu_*`, `container_memory_*` and `container_fs_*`; `transform` and `groupbyattrs` map them to pods |
| Workload tags | `k8s_attributes` | namespace, pod, uid, deployment, replicaset, node, container name and ID, image name and tag, and the `app` label |
| Cluster and host | `resource/k8s` | `k8s.cluster.name` (from `DD_CLUSTER_NAME`) and `k8s.node.name` on every signal. `datadog.host.name` is set to the node name. |
| Metric tags | `datadog` exporter `metrics.resource_attributes_as_tags: true` | resource attributes become metric tags |
| APM stats | `datadog/connector` | same as the default config |

**Optional host metrics in standalone.** These aren't needed to show continuity, so the conf leaves them out. To add them:
- add a `hostmetrics` receiver with `root_path: /hostfs` to the `metrics/node` pipeline;
- mount a `/` hostPath at `/hostfs` on otel-agent through `override.nodeAgent`.

The metrics come out with OTel names (`system.cpu.time`, ...), which are not the core Agent's Datadog names. On kind the node's root filesystem is overlay, so there are no `system.filesystem.*` metrics.

## Findings

1. **`DD_OTEL_STANDALONE` must be `true` without the core Agent.** With `false`, DDOT assumes a core Agent is present; for example, it warns that a `hostmetrics` receiver should only run in standalone mode. The operator now sets it from the node Agent mode. It stays `false` in Agent+DDOT.
2. **Point the app at the node-local Service, not the hostPort.** The operator already creates `<dda>-agent` with `internalTrafficPolicy: Local` for the OTLP ports.
   - Through the hostPort, the connection's source IP doesn't match the pod, so `k8s_attributes` can't associate the telemetry: `ip_lookup_miss` keeps increasing and there are no pod tags.
   - Through the Service, the pod IP is kept and all workload tags are added.
   - Instead of using the Service, the app can send `k8s.pod.uid` or `k8s.pod.ip` in `OTEL_RESOURCE_ATTRIBUTES`.
3. **Set `datadog.host.name` to the core Agent's hostname.** The Node SDK sets `host.name` to the pod name, so without this every pod shows up as its own host.
   - When a cluster name is set, the core Agent reports `<node>-<cluster>` (`ddot2-worker-ddot2-parity` here). The conf uses `${env:K8S_NODE_NAME}-${env:DD_CLUSTER_NAME}`, so both modes report the same host.
   - The operator's standalone `DD_HOSTNAME` is the plain node name. It only applies when telemetry has no host attribute, and it diverges from the core Agent hostname. That's an operator follow-up: the core Agent hostname depends on the provider (on cloud it's the instance hostname).
4. **No volume overrides needed.** DDOT warns that it can't read `/host/proc/mounts` and that APM data may be missing container IDs. The Agent+DDOT otel-agent logs the same warning. Spans get `container.id` and the pod tags from `k8s_attributes` (or `infraattributes`), verified on kind with no extra mounts.
5. **RBAC:** the operator's standalone rules (pods, namespaces, replicasets) plus the existing node Agent ClusterRole (`nodes/stats`, `nodes/metrics`) are enough. Static kubelet targets avoid the `nodes` list/watch that `kubernetes_sd` would need.

## Verified on kind-ddot2

- otel-agent is Ready, with 0 ERROR lines.
- **Express spans** carry `k8s.namespace.name`, `k8s.pod.name`, `k8s.pod.uid`, `k8s.deployment.name`, `k8s.replicaset.name`, `k8s.node.name`, `k8s.container.name`, `container.id`, `container.image.name`, `container.image.tag`, `k8s.cluster.name` and `datadog.host.name`.
- **Container-level kubelet metrics** carry the same workload and container tags.
- `otelcol_receiver_accepted_*` increases for `otlp`, `kubeletstats`, `prometheus/cadvisor` and `prometheus`. Refused counts stay at 0 and there are no exporter failures.

The Datadog UI was not checked. To check, look at `service:otel-js` and at the `host:ddot2-worker-ddot2-parity` metrics.

## Follow-ups to consider

- **Generate this config in the operator.** Make the standalone default config this richer set, which would need its RBAC to be operator-managed. Or ship it as a documented preset.
- **Prefer the node-local Service.** Document it as the endpoint, or inject it, for example through admission-controller OTLP injection once standalone supports it.
- **Kubelet container metrics in Agent+DDOT.** The core Agent already collects them, so DDOT `kubeletstats` container metrics are duplicates there (see the name collisions below).

## Metric comparison

Measured on kind-ddot2 with both rich configs and the express app (traces plus SDK metrics). The standalone run had `hostmetrics` enabled; without it, the standalone total is 147. DDOT names come from a temporary `debug` exporter on every metric pipeline. Core Agent names come from `agent check <check> --json` for the host, kubelet and container checks. The raw lists are in `metric-names/`.

| Family | Source | Agent+DDOT | Standalone |
|---|---|---|---|
| App OTLP metrics (`http.server.request.duration`, `http.client.request.duration`, `nodejs.eventloop.*`, `v8js.*`) | express SDK → DDOT `otlp` | 17 | 17 |
| APM stats and trace pipeline (`dd.internal.stats.payload`, `datadog.trace_agent.*`) | DDOT `datadog/connector` | 52 | 52 |
| Kubelet (`k8s.node.*`, `k8s.pod.*`, `k8s.volume.*`) | DDOT `kubeletstats` | 31 | 31 |
| Kubelet containers (`container.*`) | DDOT `kubeletstats` | 11 (6 collide with the core Agent) | 11 |
| cAdvisor (`container_*`) | DDOT `prometheus/cadvisor` | 6 | 6 |
| DDOT self-metrics (`otelcol*`) | DDOT `prometheus` | 29 (incl. `cumulativetodelta`) | 30 (incl. `k8s_attributes`) |
| Host, OTel names (`system.cpu.time`, `system.memory.usage`, ...) | DDOT `hostmetrics` (optional) | none | 25 when enabled |
| **DDOT total** | | **146** | **147** without `hostmetrics`, 172 with it (143 in common) |
| Host, Datadog names (`system.cpu.user`, `system.mem.used`, ...) | core Agent `cpu`/`memory`/`load`/`disk`/`io`/`network`/`uptime`/`file_handle` | 155 | none |
| Kubelet, Datadog names (`kubernetes.*`) | core Agent `kubelet` | 78 | none |
| Containers, Datadog names (`container.*`, `containerd.*`, `cri.*`) | core Agent `container`/`containerd`/`cri` | 54 | none |

### Takeaways

- **Nothing DDOT collects is dropped by the switch.** Standalone adds the `k8s_attributes` self-metrics, plus `hostmetrics` when enabled. Agent+DDOT only has 3 extra `cumulativetodelta` self-metrics.
- **The core Agent's metrics are what disappear in standalone.** That covers about 287 host, kubelet and container metrics with Datadog names. DDOT's OTel-named replacements have different names and do not overlap: no `system.*` name is shared. Dashboards and monitors built on Datadog names (`system.cpu.user`, `kubernetes.cpu.usage.total`) would go empty.
  - DDOT exports OTel names as-is because the `exporter.datadogexporter.metricremappingdisabled` feature gate is on by default. Whether re-enabling the Datadog remapping in standalone closes this gap is worth testing next.
- **Name collisions in Agent+DDOT.** DDOT `kubeletstats` and the core Agent `container` check both send `container.cpu.usage` and `container.memory.{usage,working_set,rss,page_faults,major_page_faults}`, with different semantics.
  - In Agent+DDOT, disable the kubeletstats `container` metric group, or leave container metrics to the core Agent.
  - Don't use these names to compare the two modes.

### App metrics to compare across modes

These are collected by DDOT in both setups, with the same name and the same workload tags. In Agent+DDOT the tags come from `infraattributes` (`kube_deployment:app`, `kube_namespace:demo`). In standalone they come from `k8s_attributes` (`k8s.deployment.name:app`, `k8s.namespace.name:demo`).

| Metric | Source | Filter |
|---|---|---|
| `http.server.request.duration` (histogram by route and status) | express SDK OTLP metrics | `service:otel-js` |
| `nodejs.eventloop.delay.p99`, `v8js.memory.heap.used` | express SDK runtime metrics | `service:otel-js` |
| `k8s.pod.cpu.usage`, `k8s.pod.memory.working_set` | DDOT `kubeletstats` (no collision) | the app pod |
| APM stats `trace.*` (hits, errors, latency) | DDOT `datadog/connector` from express spans | `service:otel-js` |

The SDK metrics are turned on with env vars only: `OTEL_METRICS_EXPORTER=otlp` with an endpoint on the node-local Service, delta temporality and a 10s interval (see `express-app.yaml`).
