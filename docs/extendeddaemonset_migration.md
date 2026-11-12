# Migrating from ExtendedDaemonSet

ExtendedDaemonSet support has been removed from the Datadog Operator. The
Operator now deploys every node Agent with a native Kubernetes DaemonSet and no
longer installs EDS API types, watches EDS resources, or accepts
`--supportExtendedDaemonset` and `--eds*` flags.

Do not upgrade directly while the current Operator is managing an
ExtendedDaemonSet. The new Operator does not read, migrate, or delete existing
EDS resources.

## Upgrade procedure

1. Upgrade to Datadog Operator `1.30.0`, keeping EDS enabled during this
   upgrade. Version `1.30.0` is the last EDS-capable release and contains the
   migration that safely retires the Agent EDS and lets the native DaemonSet
   adopt its pods.
2. If `--edsMaxPodUnavailable` is configured, preserve its value in the
   DatadogAgent before removing the EDS options:

   ```yaml
   spec:
     override:
       nodeAgent:
         updateStrategy:
           rollingUpdate:
             maxUnavailable: <edsMaxPodUnavailable-value>
   ```

3. While running Operator `1.30.0`, disable EDS by removing
   `--supportExtendedDaemonset=true` or setting it to `false`. Remove the
   `--eds*` options after translating `--edsMaxPodUnavailable` as described
   above.
4. Wait for Operator `1.30.0` to migrate the node Agent to a native
   DaemonSet. Confirm the DaemonSet is fully ready:

   ```shell
   kubectl -n <agent-namespace> get daemonset
   kubectl -n <agent-namespace> rollout status daemonset/<agent-name>
   ```

5. Confirm that the migrated Agent's ExtendedDaemonSet and its replica sets are
   gone. Other workloads may still use EDS resources in the same namespace.

   ```shell
   kubectl -n <agent-namespace> get extendeddaemonset <agent-name>
   kubectl -n <agent-namespace> get extendeddaemonsetreplicaset \
     -l extendeddaemonset.datadoghq.com/name=<agent-name>
   ```

   The first command should report `NotFound` and the second should return no
   resources. Do not remove the EDS controller or CRDs until the native
   DaemonSet is healthy.
6. Upgrade the Datadog Operator and remove the obsolete EDS controller and CRDs
   if no other workload uses them.

Custom Operator Deployment manifests must not pass the removed EDS flags. Helm
or OLM configuration that previously enabled EDS must be updated to render a
DaemonSet-only Operator deployment before the upgrade.
