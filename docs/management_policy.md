# Stop managing a Datadog resource (orphan policy)

The `datadoghq.com/management-policy: orphan` annotation stops the Datadog Operator from managing a `DatadogGenericResource`, `DatadogMonitor`, `DatadogDashboard`, or `DatadogSLO`, without deleting the Datadog object. `orphan` is the only supported value. Without the annotation, or with any other value, the Operator manages the resource as usual.

While a resource has the annotation:

* The Operator makes no Datadog API call for it: no create, update, periodic force sync, or state refresh. Changes to the spec are not applied.
* The resource has an `Orphaned` condition set to `True`. Its other status fields keep their last values.
* Deleting the resource removes the Operator's finalizer and leaves the Datadog object in place. The Operator records an `Orphaned` event.

Removing the annotation resumes normal reconciliation: a spec changed in the meantime is applied right away, otherwise the next periodic force sync applies the spec again.

## Delete a resource but keep its Datadog object

Annotate the resource, then delete it. The Operator must be running, because its finalizer applies the policy.

```shell
kubectl annotate datadogmonitor/<name> datadoghq.com/management-policy=orphan
kubectl delete datadogmonitor/<name>
```

The annotation can also be added to a resource that is already being deleted, for example one that stays terminating because the Datadog API keeps rejecting the delete. The Operator then removes the finalizer without deleting the Datadog object.

With GitOps tools such as Argo CD or Flux, the annotation must be on the live resource before the tool prunes it. Commit the annotation and wait for it to sync, then remove the manifest in a later commit. Adding the annotation in the same commit that removes the manifest doesn't work: that version of the manifest is never applied.

## Pause a resource

The annotation also pauses a single resource without disabling a whole controller, for example on a passive cluster that deploys the same manifests as the active one. Remove the annotation to resume.

## Older Operator versions

Operator versions without this annotation ignore it. On those versions, disable the controller (for example `-datadogMonitorEnabled=false`), remove the finalizer from the resource, then delete it.
