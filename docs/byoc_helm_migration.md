# Migrate CloudPrem from the Helm charts

`kubectl byoc-migrate` migrates the CloudPrem and Observability Pipelines Worker
objects rendered by the `cloudprem` and `observability-pipelines-worker` Helm
charts to a DatadogBYOCCluster. It reads the objects from manifest files, as
`helm template` or `helm get manifest` prints them, so it works whether the Helm
CLI or another tool, such as a deployment pipeline that runs `kubectl apply`,
applied them.

- The cluster takes over the CloudPrem objects in place, so the data, the
  PersistentVolumeClaims, and the Service addresses are kept. Each workload
  rolls out once, because the Pod templates change. The read-only Metastore is
  recreated as `<cluster>-read-only-metastore`, next to the one of the chart.
- Each pipeline runs a new Worker, `<cluster>-pipeline-<pipeline>`, next to the
  objects of its Observability Pipelines Worker manifest. The plugin does not
  move the clients, scale down, or delete the objects of the manifest: point
  the clients to the Worker yourself, for example by changing the selector of a
  load balancer Service or a DNS record, then scale the StatefulSet of the
  manifest to zero and delete the objects of the manifest. The Worker does not
  take over the PersistentVolumeClaims of the manifest.
- Without Observability Pipelines Worker objects, omit `--opw-manifest`. The
  cluster then has no pipeline and runs no Worker.
- The plugin does not create, change, or delete the cluster: apply and delete
  it yourself, for example from the deployment pipeline that applies your
  other resources.

## Install

`kubectl-byoc-migrate` is a kubectl plugin, separate from `kubectl-datadog`.
Build the version of the Datadog Operator that runs in the cluster: the plugin
renders the resources of the cluster with the code of the Operator, so a
different version renders different resources.

Build it from a clone of the repository, at the tag of that version. Do not use
`go install github.com/DataDog/datadog-operator/cmd/kubectl-byoc-migrate@<version>`:
it ignores the `go.work` file of the repository, so it builds with the version
of the `api` module that `go.mod` requires, which can be older than the `api`
directory of the same commit, and the build fails.

```console
git clone https://github.com/DataDog/datadog-operator.git
cd datadog-operator
git checkout v<operator version>
go build -o kubectl-byoc_migrate ./cmd/kubectl-byoc-migrate
```

It needs the Go version of the `go` directive of `go.mod`, or later. Name the
binary `kubectl-byoc_migrate`: kubectl reads a dash in the name of a plugin
binary as a space between subcommands, so it would run `kubectl-byoc-migrate`
as `kubectl byoc migrate`, and an underscore as a dash.

Move the binary to a directory of `PATH`, for example `/usr/local/bin`:

```console
sudo mv kubectl-byoc_migrate /usr/local/bin/
```

Check that kubectl finds the plugin: `kubectl plugin list` lists
`kubectl-byoc_migrate`, and `kubectl byoc-migrate --help` lists the `convert`,
`diff`, `adopt`, and `abort` commands.

```console
kubectl plugin list
kubectl byoc-migrate --help
```

## Manifests

Write the objects that each chart rendered to a file, one file for CloudPrem
and one for each Observability Pipelines Worker release:

- For a Helm release, run `helm get manifest <release> > <file>`.
- For objects that another tool applies, use the output of `helm template` that
  the tool applies.

The plugin adopts and replaces only the objects of the manifests. `diff`
compares the objects that run in the cluster, not the manifests, with the
objects after the migration, so it also shows what differs between the
manifests and the cluster. Objects without a namespace are in the namespace of
the command. Pass the same manifests to every command.

## Requirements

- The cluster is in the namespace of the objects and is named after the
  CloudPrem Helm fullname, for example `cloudprem` for a release named
  `cloudprem`.
- The CloudPrem release name equals its fullname, and `nameOverride` is not
  used. Otherwise the selectors of the existing workloads, which cannot change,
  differ from the selectors of the controller.
- The volume claim templates of the CloudPrem StatefulSets, and the type of the
  CloudPrem Services, match the cluster.
- The API key and the Metastore URIs come from Secrets.

## What every migration changes

Apart from the settings of the manifests, the cluster runs the objects with
the defaults of the Operator. `diff` shows these changes too:

- Each CloudPrem workload without an affinity gets a preferred pod
  anti-affinity on the node, so that the scheduler spreads its Pods across the
  nodes. The chart sets no affinity. To keep the scheduling of the chart, set
  `affinity: {}` on the component in `cluster.yaml`.
- The Service of each CloudPrem component exposes the `health` port, `7284`,
  including the compactor and the read-only Metastore, whose Services in the
  chart do not.
- Each Worker sends to the indexer of the cluster: the cluster sets its
  `DD_OP_DESTINATION_CLOUDPREM_ENDPOINT_URL` variable to the indexer Service,
  unless the pipeline or `global` sets the variable. The converter keeps the
  value of the manifest in `env` of the pipeline when it differs.
- When the Metastore of the chart imports the Secret of its URI with
  `envFrom`, as the chart recommends, the cluster also sets the
  `QW_METASTORE_URI` variable of the Metastore, from the same key of the same
  Secret. It keeps the `envFrom` of the chart, because the other keys of the
  Secret may set other variables. The variable takes precedence over
  `envFrom`, and has the same value, so the Metastore keeps its URI.
- The Pods of the Workers and of the read-only Metastore have other labels
  than the Pods of the charts, so the objects that select the Pods of the
  charts by label, such as NetworkPolicies and ServiceMonitors, do not select
  them.

## Steps

1. Convert the manifests. `convert` reads nothing from the API server: it
   prints the cluster, and the findings that `diff` does not show. It fails
   when a `Blocker` prevents the migration. `diff`, in the next step, shows the
   settings of the manifests that the cluster loses or changes. Edit
   `cluster.yaml` if needed.

   ```console
   kubectl byoc-migrate convert -n <namespace> --cloudprem-manifest cloudprem.yaml \
     --opw-manifest <file>=<pipeline> > cluster.yaml
   ```

2. Review what the migration changes. `diff` has the API server compute the
   objects after the migration with dry-run requests, and compares them with
   the objects that run in the cluster. It writes nothing, but it needs the
   permissions of `adopt`, because the API server authorizes a dry-run request
   as the request itself, and it fails when an admission webhook does not
   support dry-run requests. It prints:

   - a unified diff from each live CloudPrem object to the object that the
     cluster adopts, and from each live object of the manifests to the resource
     that replaces it, such as the read-only Metastore and the Workers. The
     diff lists the environment variables by name, so a variable that only
     moves in the list does not show,
   - the objects of the manifests that the cluster replaces, to delete after
     the migration, and the objects that the migration leaves as is,
   - the blockers, such as an object of the manifest that does not exist, or a
     resource that the API server refuses.

   Edit `cluster.yaml` if needed, and run `diff` again. Do not rename the
   cluster: the commands fail when it is not named after the CloudPrem Helm
   fullname.

   ```console
   kubectl byoc-migrate diff -n <namespace> --cloudprem-manifest cloudprem.yaml \
     --opw-manifest <file>=<pipeline> --from-file cluster.yaml
   ```

3. Stop applying the manifests: stop running `helm upgrade` on the releases, or
   remove the objects from the deployment pipeline without deleting them. A
   tool that prunes the objects it no longer applies, such as
   `kubectl apply --prune` or a GitOps controller, deletes them, including the
   objects that the cluster adopts. Keep the Helm releases installed so that the
   migration can be aborted.
4. Apply `cluster.yaml`, for example from your deployment pipeline. Until
   `adopt` gives the cluster the CloudPrem objects, its `Reconciled` condition
   is `False` with reason `Conflict`: the controller writes only to the objects
   that the cluster controls, so it does not change the CloudPrem objects. It
   may create resources that the manifests do not have, such as the read-only
   Metastore, but it creates the Workers only once it applied the CloudPrem
   objects.

   ```console
   kubectl apply -n <namespace> -f cluster.yaml
   ```

   Then run `diff` again without `--from-file`, right before `adopt`: it renders
   the cluster that runs in the API server, which `adopt` gives the objects to,
   and fails when `--from-file` is passed once the cluster exists. `adopt` does
   not compare the objects again, so what changes in the cluster after `diff`
   is not reviewed.

   ```console
   kubectl byoc-migrate diff -n <namespace> --cloudprem-manifest cloudprem.yaml \
     --opw-manifest <file>=<pipeline>
   ```

5. Give the CloudPrem objects to the cluster. `adopt` reads the cluster from the
   API server, and does not change it. It checks that every object of the
   CloudPrem manifest that the cluster adopts exists and has no other
   controller, and that the user of the context may patch it, as
   `kubectl auth can-i` does. It writes nothing when a check fails, for example
   with another context or namespace than the manifest, and lists the objects
   that the user may not patch. It does not check the admission webhooks. It then gives the objects to the
   cluster one by one. Then wait for the cluster to be available.

   ```console
   kubectl byoc-migrate adopt -n <namespace> --cloudprem-manifest cloudprem.yaml \
     --opw-manifest <file>=<pipeline>
   kubectl wait -n <namespace> --for=condition=Available --timeout=10m \
     datadogbyoccluster/<cluster>
   ```

6. Once the cluster and the Workers are available, point the clients of the
   Observability Pipelines Worker objects to the Workers, for example by
   changing the selector of a load balancer Service or a DNS record. The
   Service of a Worker is `<cluster>-pipeline-<pipeline>`. To change the
   selector of a Service of a chart, replace the whole selector with the
   selector of the Service of the Worker: the selectors of the charts include
   `app.kubernetes.io/managed-by: Helm`, which the Pods of the Workers do not
   have, so a merge patch keeps it, and the Service then selects no Pod.

   ```console
   selector=$(kubectl get service -n <namespace> <cluster>-pipeline-<pipeline> -o jsonpath='{.spec.selector}')
   kubectl patch service -n <namespace> <service> --type=json \
     -p "[{\"op\":\"replace\",\"path\":\"/spec/selector\",\"value\":${selector}}]"
   ```

   To move the clients back before `abort`, replace it the same way with the
   selector of the manifest. The Pods of the
   Workers and of the read-only Metastore have other labels than the Pods of
   the charts: update the objects that select the Pods of the charts by label,
   such as NetworkPolicies and ServiceMonitors, to select the new Pods too. Each
   Worker runs
   next to the Pods of its manifest until they are scaled to zero, which
   doubles the Pods for a while. Then scale the StatefulSets of the manifests
   to zero:

   ```console
   kubectl scale statefulset -n <namespace> <statefulset> --replicas=0
   ```

7. Check that the data still flows. To revert the migration, first scale the
   StatefulSets of the Observability Pipelines Worker manifests back, wait for
   their Pods, and move the clients back to them: deleting the cluster deletes
   the Workers, so clients that still send to the Workers lose them. For a
   StatefulSet with a HorizontalPodAutoscaler, scale it to at least the replicas
   of its Worker, which reflect the current load. `abort` checks that the Pods
   of the manifests are ready, and that the user of the context may patch the
   CloudPrem objects that the cluster owns, and writes nothing otherwise. Then
   run `abort` with the same flags. It removes the cluster from the owners of
   the CloudPrem objects, but does not delete the cluster:

   ```console
   kubectl scale statefulset -n <namespace> <statefulset> --replicas=<replicas>
   kubectl byoc-migrate abort -n <namespace> --cloudprem-manifest cloudprem.yaml \
     --opw-manifest <file>=<pipeline>
   ```

   Once `abort` succeeds, delete the cluster, for example by removing it from
   your deployment pipeline. Its deletion deletes only the resources that it
   created, such as the Workers and the read-only Metastore. Do not delete the
   cluster before `abort` succeeds, nor let a deployment pipeline prune it:
   its deletion would delete the CloudPrem objects that it still owns.

   ```console
   kubectl delete -n <namespace> -f cluster.yaml
   ```

   Apply the manifests again afterwards, for example with `helm upgrade` and the
   same values. With a server-side apply, as with Helm 4 and its
   `--force-conflicts`, force the conflicts: the controller still manages the
   fields of the CloudPrem objects, so the apply fails with conflicts otherwise.

   A StatefulSet of the CloudPrem chart does not replace a Pod that is not
   ready, for example one that the cluster ran with a setting that fails:
   with its `OrderedReady` Pod management policy, it waits for the Pod to be
   ready before it updates it. Once the manifests are applied again, delete
   the Pods of the StatefulSets that are not ready, so that they are recreated
   with the settings of the chart:

   ```console
   kubectl delete pod -n <namespace> <pod>
   ```

   Applying the manifests again restores the settings of the charts, but keeps
   the fields that the cluster added and that the charts do not set, such as the
   `app.kubernetes.io/component` labels, the
   `app.kubernetes.io/managed-by: datadog-operator` label of the Pod templates,
   the `QW_METASTORE_URI` variable of the Metastore, and the `health` port of
   some Services. They do not change how CloudPrem runs. Deleting the cluster
   keeps the PersistentVolumeClaims of the Workers: they may hold data that the
   Workers buffered while they took the traffic, so delete them once the data is
   no longer needed. List them with:

   ```console
   kubectl get pvc -n <namespace> -l app.kubernetes.io/instance=<cluster>-pipeline-<pipeline>
   ```

8. Delete the objects that the cluster replaced, which `diff` lists at its
   end: the read-only Metastore of the chart, the PodDisruptionBudgets
   that the cluster does not set, and the objects of the Observability Pipelines
   Worker manifests, their Services included. Delete only these: the other
   objects of the CloudPrem manifest belong to the cluster, which lists itself in
   their `ownerReferences`. Keep the objects that the cluster does not manage,
   which `diff` lists as left as is, such as Ingresses, as long as you need
   them.
   The PersistentVolumeClaims of the Observability Pipelines Worker StatefulSets
   are not deleted with them; delete them once their data is no longer needed.
   Once the replaced objects are deleted, `abort` cannot restore them: apply the
   manifests again first.
9. Do not apply the manifests again. For Helm releases, delete their records
   without uninstalling them: `helm uninstall` deletes every object of a
   release, including the adopted ones. With the default Secret storage of
   Helm, run for each release:

   ```console
   kubectl delete secret -n <namespace> -l owner=helm,name=<release>
   ```

   With `HELM_DRIVER=configmap`, delete the ConfigMaps with the same labels
   instead.

`adopt` and `abort` print the Kubernetes context, and ask for a confirmation
once their checks pass, before they write anything; `--yes` skips it. `abort`
accepts `--dry-run`, which runs its checks and prints the changes without
writing anything; `diff` shows what `adopt` changes. A command that stopped
halfway can run again. To revert the migration, run `abort` before deleting
the cluster: deleting the cluster first deletes the adopted CloudPrem objects.
