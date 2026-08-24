# Migrating from ExtendedDaemonSet

ExtendedDaemonSet support has been removed from the Datadog Operator. The
Operator now deploys every node Agent with a native Kubernetes DaemonSet and no
longer installs EDS API types, watches EDS resources, or accepts
`--supportExtendedDaemonset` and `--eds*` flags.

Do not upgrade directly while the current Operator is managing an
ExtendedDaemonSet. The new Operator does not read, migrate, or delete existing
EDS resources.

## Upgrade procedure

1. While running the last EDS-capable Operator release, disable EDS by removing
   `--supportExtendedDaemonset=true` or setting it to `false`. Remove any
   `--eds*` options as well.
2. Wait for that Operator release to migrate the node Agent to a native
   DaemonSet. Confirm the DaemonSet is fully ready:

   ```shell
   kubectl -n <agent-namespace> get daemonset
   kubectl -n <agent-namespace> rollout status daemonset/<agent-name>
   ```

3. Confirm that no ExtendedDaemonSet or ExtendedDaemonSetReplicaSet remains in
   the Agent namespace. Do not remove the EDS controller or CRDs until the
   native DaemonSet is healthy.
4. Upgrade the Datadog Operator and remove the obsolete EDS controller and CRDs
   if no other workload uses them.

Custom Operator Deployment manifests must not pass the removed EDS flags. Helm
or OLM configuration that previously enabled EDS must be updated to render a
DaemonSet-only Operator deployment before the upgrade.
