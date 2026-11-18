# Engineering brief: adopting existing Datadog resources with DatadogGenericResource

- **Ticket:** [CONTP-2280](https://datadoghq.atlassian.net/browse/CONTP-2280), part of [CONTP-2191](https://datadoghq.atlassian.net/browse/CONTP-2191)
- **Customer asks:** [FRCONS-1232](https://datadoghq.atlassian.net/browse/FRCONS-1232), [FRCONS-1251](https://datadoghq.atlassian.net/browse/FRCONS-1251), plus an active/passive DR question (see [Side benefit: active/passive clusters](#side-benefit-activepassive-clusters))
- **Status:** Draft for review by Operator maintainers. POCs are not covered here.

## Summary

Today a `DatadogGenericResource` (DDGR) always creates a new Datadog object and stores the new ID in `.status.id`. Customers migrating from `DatadogMonitor`, `DatadogDashboard` or `DatadogSLO`, or bringing resources created in the UI, API or Terraform, lose the ID. With it they lose history and every reference to the ID: composite monitors, SLOs, dashboards, links and automation.

This brief proposes:

1. **A new optional `spec.id` field on DDGR.** When it's set, the controller takes over the existing Datadog object with that ID instead of creating one. It **never creates a replacement** while `spec.id` is set. All validation happens in the controller, because CEL isn't available on every Kubernetes version we support.
2. **A `datadoghq.com/deletion-policy: orphan` annotation**, handled in the shared finalizer package. It works for DDGR, `DatadogMonitor`, `DatadogDashboard` and `DatadogSLO`. Deleting an annotated CR removes the CR but leaves the Datadog object in place. This is the safe handoff that migration needs.
3. **A blocking check against legacy CRs.** A DDGR can't adopt an ID that a `DatadogMonitor`, `DatadogDashboard` or `DatadogSLO` still manages. Two controllers never manage the same object.
4. **A `kubectl datadog monitor migrate` command** that converts a `DatadogMonitor` into an adopting DDGR in the safe order. It's the priority tool. A manual workflow is documented for the other legacy CRDs and for resources created outside Kubernetes.

## Current behavior (baseline)

The behavior below comes from `internal/controller/datadoggenericresource/controller.go` and `internal/controller/finalizer/finalizer.go`.

| Situation | Today |
| --- | --- |
| `status.id` empty | Create a new object and store its ID in `status.id`. |
| Spec hash changed | Update the object identified by `status.id`. |
| Force sync (every 60 min by default) | GET the object. Monitors skip the update when the remote definition hash is unchanged. Other types always update. |
| 404 during force sync or update | **Recreate** the object, which gives it a new ID. |
| CR deleted, finalizer present | Delete the remote object (a 404 counts as success). The delete is skipped when `status.id` is empty. |
| CR without finalizer | The controller re-adds the finalizer on every reconcile unless the CR is being deleted. |
| Pre-setting `status.id` by hand | Not supported: `status` is a subresource, `kubectl apply` ignores it, backups usually don't restore it, and nothing validates it. |

Legacy CRDs behave the same way. One difference: `DatadogMonitor` only deletes monitors it created itself (the `status.primary` field). Both `DatadogMonitor` and `DatadogSLO` also inject a `generated:kubernetes` tag into the CR spec.

## Where the ID goes

### Options

| Option | Pros | Cons |
| --- | --- | --- |
| **A. `spec.id` (dedicated field)** | Typed and part of the schema. Visible in `kubectl get`. GitOps-friendly. Same pattern as Config Connector's `spec.resourceID`. | Requires a CRD change. On downgrade, an older CRD prunes the field (see [Compatibility](#compatibility)). Immutability is enforced by the controller, not at admission (no CEL). |
| B. Annotation (for example `datadoghq.com/id`) | No schema change. Same pattern as Crossplane's `external-name`. | Untyped. Easy to lose or copy by mistake. Mixes identity into metadata. Without CEL, neither A nor B can be validated at admission, so this isn't a difference. |
| C. `id` inside `jsonSpec` | No new field. | `jsonSpec` is the API create/update payload. `id` is response-only for most types, and the handlers unmarshal straight into create bodies. Contradicts the current migration guide ("do not put an existing Datadog ID in `spec.jsonSpec`"). Can't be validated without parsing per type. |

### Recommendation: `spec.id`

```go
type DatadogGenericResourceSpec struct {
    Type     SupportedResourcesType `json:"type"`
    JsonSpec string                 `json:"jsonSpec"`
    // ID of an existing Datadog resource to adopt instead of creating a new one.
    // When set, the Operator never creates a replacement resource.
    // +optional
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=128
    ID string `json:"id,omitempty"`
}
```

**No CEL.** We still support Kubernetes versions where CRD validation rules (`x-kubernetes-validations`) aren't available. The only admission-time validation is plain OpenAPI (`MinLength`, `MaxLength`), which works on every supported version. Everything else is enforced by the controller and reported through conditions:

| Rule | Where it's enforced |
| --- | --- |
| ID format for `spec.type` | Controller, permanent error `AdoptionInvalidID` |
| `spec.id` not allowed for `software_catalog_entity` | Controller, permanent error `AdoptionUnsupportedType` |
| `spec.id` can't change once the object is adopted | Controller: `spec.id != status.id` with `status.id` set gives `IDMismatch` and no remote action |
| `spec.id` removed | Can't be blocked. Allowed, with defined behavior (see [Later reconciles](#later-reconciles)) |

A validating webhook could reject these at admission, but the Operator doesn't run admission webhooks today. Adding one for this alone isn't worth the deployment cost (certificates, availability).

How `spec.id` relates to `status.id`:

- `spec.id` is the **declared** identity. The user asks the controller to manage this object.
- `status.id` is the **observed** identity. It's the object the controller is managing right now.
- After a successful adoption, `status.id == spec.id`. Adding `spec.id` to a CR that already manages an object is accepted only when it equals `status.id`. That lets users write the ID back into their manifests (GitOps) after the first creation. Comparing `spec.id` with `status.id` is also how the controller enforces immutability without CEL.
- **`spec.id` is left out of the spec hash.** The hash (`status.currentHash`) decides whether `jsonSpec` must be written to Datadog, and `spec.id` doesn't change the payload. Leaving it out avoids a pointless update when users write the ID back into their manifests, and it costs nothing:
  - Existing CRs keep the same hash. `id` is `omitempty`, so an unset field serializes exactly as it does today, and upgrading doesn't trigger a wave of updates.
  - Changes to `spec.id` are still noticed. They bump `metadata.generation`, which triggers a reconcile, and the `spec.id` vs `status.id` check runs before the hash comparison.
  - The only cost is hashing a copy of the spec with `id` cleared, instead of hashing the spec directly.

Per-type ID formats (`status.id` is always a string):

| Type | ID format |
| --- | --- |
| `monitor`, `notebook` | integer |
| `dashboard` | `abc-def-ghi` |
| `slo`, `slo_correction` | string |
| `synthetics_*` | public ID `abc-def-ghi` |
| `downtime`, `monitor_notification_rule` | UUID |

The controller validates the format per type. A format error is a permanent error, so the controller doesn't retry until the spec changes.

### Example

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogGenericResource
metadata:
  name: checkout-latency
spec:
  type: monitor
  id: "123456789"
  jsonSpec: |-
    {
      "name": "Checkout latency",
      "type": "query alert",
      "query": "avg(last_5m):avg:trace.http.request.duration{service:checkout} > 0.5",
      "message": "Checkout latency is high @slack-checkout",
      "tags": ["team:checkout", "generated:kubernetes"],
      "options": {"thresholds": {"critical": 0.5}}
    }
```

## Lifecycle when `spec.id` is set

The principle: **`jsonSpec` is the source of truth, and the ID is never silently replaced.** Adoption is explicit (the user wrote the ID), and its consequences are explicit too (the first reconcile writes `jsonSpec` and records an event).

How a reconcile is routed:

```mermaid
flowchart TD
  start([Reconcile DDGR]) --> del{Being deleted?}
  del -- yes --> fin[[Finalizer: see deletion flow]]
  del -- no --> sid{spec.id set?}
  sid -- no --> legacy[Existing behavior based on status.id<br/>create if empty, recreate on 404]
  sid -- yes --> stid{status.id?}
  stid -- empty --> adopt[[Adoption: see first-reconcile flow]]
  stid -- "== spec.id" --> normal[Update / force sync as today<br/>404 → AdoptedResourceNotFound, NO recreate]
  stid -- "!= spec.id" --> mismatch[IDMismatch<br/>no remote action]
```

### First reconcile (adoption)

```mermaid
flowchart TD
  a([spec.id set, status.id empty]) --> v{Type supported and<br/>ID format valid?}
  v -- no --> e1[AdoptionUnsupportedType / AdoptionInvalidID<br/>permanent, no API call]
  v -- yes --> fresh[Fresh read of the CR via apiReader]
  fresh --> stale{CR changed?<br/>UID, deleting, other status.id}
  stale -- yes --> rq[Skip, requeue]
  stale -- no --> dup{Another DDGR owns<br/>same type + id?}
  dup -- yes --> e2[AdoptionConflict<br/>retry at forceSyncPeriod]
  dup -- no --> leg{Legacy CR with same status.id?<br/>uncached List}
  leg -- yes --> e3[AdoptionLegacyOwner<br/>retry at requeuePeriod 60s]
  leg -- forbidden --> e3b[Fail closed: missing list RBAC]
  leg -- "no / CRD absent" --> get{GET spec.id}
  get -- 404 --> e4[AdoptionTargetNotFound<br/>NO create]
  get -- 401/403 --> e5[AdoptionForbidden<br/>NO create]
  get -- "5xx / 429 / network" --> e6[Transient, retry in 5s]
  get -- 200 --> persist[status.id = spec.id<br/>Adopted=True, Event Adopted<br/>currentHash empty]
  persist --> upd[Next reconcile: hash mismatch<br/>existing update path writes jsonSpec]
```

```
spec.id set, status.id empty:
  1. Validate spec.type and the ID format.          → permanent error AdoptionUnsupportedType / AdoptionInvalidID
  2. Fresh read of the CR (apiReader), same stale-cache guard as create().
  3. Check that no other DDGR in the cluster has the same type and id
     (field index on type and id).                  → error AdoptionConflict, retry at forceSyncPeriod
  4. Check that no legacy CR manages the same ID
     (see "Competing owners").                      → error AdoptionLegacyOwner, retry at requeuePeriod (60s)
  5. GET the resource by spec.id.
       404          → error AdoptionTargetNotFound, NO create, retry at forceSyncPeriod
       401/403      → error AdoptionForbidden, NO create, retry at forceSyncPeriod
       5xx/429/net  → transient, retry in 5s
  6. Persist status.id = spec.id, condition Adopted=True, Event "Adopted".
     CurrentHash stays empty.
  7. Next reconcile: the hash mismatch takes the existing update path and
     writes jsonSpec to the object, with Event "Updated".
```

Writing `status.id` before the first update reuses the existing update, force-sync and remote-hash code unchanged. It also means a crash between steps 6 and 7 can't lose the ID.

`status.created` and `status.creator` stay empty for adopted resources. Filling them would mean changing `getResource` in every handler to return metadata, which isn't worth it for informational fields.

### Later reconciles

| Situation | Behavior when `spec.id` is set |
| --- | --- |
| Drift / force sync | Same as today, including the monitor remote-hash skip. |
| 404 on GET or update | **No recreate.** Error condition `AdoptedResourceNotFound`, `status.id` is kept, and the controller retries at `forceSyncPeriod`. The user decides: restore the object, or delete the CR and create it again without `spec.id`. |
| Update fails (400, 403) | Same as today: error condition, retry at `forceSyncPeriod`. The remote object stays as it was. |
| `spec.id` changed after adoption | Error condition `IDMismatch`. No remote action, and the controller keeps `status.id`. The user reverts the change, or deletes and recreates the CR. |
| `spec.id` removed | Accepted. The controller falls back to the existing behavior based on `status.id`: it keeps managing the same object, and a later 404 recreates it. Documented as "removing `spec.id` gives up the no-replacement guarantee". |
| `spec.id` added later, equal to `status.id` | No-op: no remote call, because `spec.id` isn't part of the hash. |
| `spec.id` added later, different from `status.id` | Same as a change: `IDMismatch`, no remote action. |
| `spec.type` doesn't match the object | The GET uses the endpoint for `spec.type`, so a wrong type normally returns 404 and is reported as `AdoptionTargetNotFound`. |

When `spec.id` is not set, behavior doesn't change, including recreating the object on 404.

### Competing owners

| Owner | Detected? |
| --- | --- |
| Another DDGR in the same cluster with the same type and id | Yes, through a field index on the DDGR cache. The CR that already has `status.id` set wins. Otherwise the oldest `creationTimestamp` wins (then the name), so the outcome is stable when two CRs reconcile concurrently. The other CR gets `AdoptionConflict`. |
| A legacy CR (`DatadogMonitor`, `DatadogDashboard`, `DatadogSLO`) in the same cluster | **Yes, blocking.** See below. |
| Another cluster, Terraform, or a script | **Not detected.** A remote ownership marker (for example a tag) isn't available on every type (notebooks have no tags, and dashboards restrict tags). It would also fight with `jsonSpec`. Documented as unsupported. |

#### Blocking check against legacy CRs

<details>
<summary>Show details</summary>

A DDGR of type `monitor`, `dashboard` or `slo` doesn't adopt an ID while a CR of the matching legacy kind has the same `status.id`. The condition is `AdoptionLegacyOwner`, and its message names the CR and points to `kubectl datadog monitor migrate` or the orphan annotation. Adoption goes ahead on its own once the legacy CR is gone.

The controller retries at `requeuePeriod` (60s by default), not at `forceSyncPeriod` (60 min). During a GitOps migration this state can be expected rather than an error: if the GitOps tool creates the DDGR before the old CR is gone, the DDGR waits. It then adopts on its first retry after the old CR's finalizer completes, so at most `requeuePeriod` later. A watch on legacy kinds would react faster, but it brings back the informer problem described below. The list is one uncached call per minute per waiting DDGR, and only while a migration is in progress. A legacy CR that is terminating still blocks until it's deleted, because its finalizer might still delete the object.

**Feasibility.** The DDGR controller doesn't watch legacy kinds, and their informers only exist when the matching controller is enabled (`pkg/config/config.go`, `CacheOptions`). Relying on the shared cache would therefore miss legacy CRs whenever their controller is disabled. Those CRs still matter: they come back to life when someone re-enables the controller. Instead:

- **Use an uncached `List` through `apiReader`, only during adoption.** Adoption runs once per CR, so a cluster-wide list of one kind per adoption is cheap. It needs no new informer, no field index and no watch, and it's independent of whether the legacy controller or its watch-namespace settings are enabled.
- **Match on `status.id`.** `DatadogMonitor.status.id` is an int and is compared as a string. `DatadogDashboard` and `DatadogSLO` use string IDs.
- **CRD not installed** (`NoKindMatch`): there can't be a legacy owner, so the check passes.
- **RBAC missing** (`Forbidden`): **fail closed** with an error naming the missing `list` permission. What each install grants today:
  - The generated role in this repo (`config/rbac/role.yaml`) grants all three kinds.
  - The Helm chart (`helm-charts`, `charts/datadog-operator/templates/clusterrole.yaml`) grants `datadogmonitors` and `datadogslos` **unconditionally**.
  - The chart grants `datadogdashboards` **only when `datadogDashboard.enabled`** is set. With the dashboard controller disabled, every `dashboard` adoption would fail closed.
  - **Required chart change:** grant `get`/`list` on `datadogdashboards` whenever `datadogGenericResource.enabled` is set.
  - With `rbac.create=false`, users manage RBAC themselves and get the explicit error.
- **Adoption is the only point where this needs checking.** Legacy CRDs can't adopt, so a legacy CR can't acquire an adopted ID later, except by setting `status.id` by hand, which isn't supported.

One limitation: the list covers the Operator's own cluster only, and legacy CRs in another cluster aren't visible. That falls under the cross-cluster row above.

</details>

### Deletion and retention

New annotation: `datadoghq.com/deletion-policy: orphan`. `orphan` is the only value: without the annotation, deletion behaves as it does today, so a `delete` value would add nothing. A migration counts as completed once the orphan annotation is gone.

```mermaid
flowchart TD
  d([CR deleted, finalizer present]) --> pol{deletion-policy annotation}
  pol -- orphan --> o[Skip remote delete<br/>Event Orphaned]
  pol -- absent --> id{status.id set?}
  id -- no --> s[Nothing to delete]
  id -- yes --> rd{Remote delete}
  rd -- "ok / 404" --> ok[Deleted]
  rd -- error --> retry[Retry. Setting orphan<br/>releases a stuck CR]
  o --> rm[Remove finalizer]
  s --> rm
  ok --> rm
```

- It's handled in `finalizer.HandleFinalizer`. On deletion, if the annotation is `orphan`, the finalizer skips `deleteFunc`, records an `Orphaned` event and removes the finalizer.
- One change covers DDGR and the three legacy CRDs.
- It's an annotation rather than a spec field so that it works on the legacy CRDs without schema changes. Annotations are also outside the spec hash, so setting one doesn't trigger an update.
- It fixes the race in today's workaround. Today, removing the finalizer and then deleting the CR can fail because the controller re-adds the finalizer in between. With the annotation, the user annotates and deletes while the controller is running, with no need to stop it.
- Annotations can still be edited on a CR that is already being deleted. A CR stuck because its remote delete keeps failing can be released by setting `orphan`.

Should adopted resources default to `orphan`, the way `DatadogMonitor` only deletes what it created? **No**, recommended: without the annotation, deleting an adopted DDGR deletes the object. After migration, DDGR should behave like any other DDGR. A different default for some resources makes `kubectl delete` behavior depend on history the user can't see. See [Open questions](#open-questions).

## Migrating from a legacy CRD (DatadogMonitor, DatadogDashboard, DatadogSLO)

The order is **orphan the old CR first, then adopt.** The [blocking legacy check](#blocking-check-against-legacy-crs) enforces it: a DDGR can't adopt while the old CR exists. The only gap is a short period where the object is unmanaged, which is harmless.

For `DatadogMonitor`, use [`kubectl datadog monitor migrate`](#tooling). The manual equivalent below is also the workflow for `DatadogDashboard` and `DatadogSLO` until the tool covers them.

```shell
# 0. Operator version with spec.id and deletion-policy support; DDGR enabled.
# 1. Build the DDGR jsonSpec from the live object (same jq filters as the migration guide).
#    Keep the generated:kubernetes tag if the old CRD injected it: DDGR updates replace tags.
# 2. Release the old CR without deleting the Datadog object.
kubectl annotate datadogmonitor/checkout-latency datadoghq.com/deletion-policy=orphan
kubectl delete datadogmonitor/checkout-latency
# 3. Check the object still exists in Datadog, then adopt it.
kubectl apply -f checkout-latency-ddgr.yaml        # with spec.id: "<old status.id>"
kubectl wait --for=condition=Adopted ddgr/checkout-latency
```

Recovering from a partial migration:

| Failed at | State | Recovery |
| --- | --- | --- |
| DDGR applied before the old CR was deleted | `AdoptionLegacyOwner`. Nothing happens to the object. | Annotate and delete the old CR. The DDGR adopts on its next retry. |
| Step 2 (annotation missing, object deleted) | Object lost. The DDGR shows `AdoptionTargetNotFound`. Nothing is recreated by surprise. | Same as today: create a new object (drop `spec.id`). The migrate tool exists to remove this mistake. |
| Step 3 (bad `jsonSpec`, 403) | `status.id` is set, the update failed, and the remote object is untouched. | Fix `jsonSpec`. The controller retries. |
| Rollback to the legacy CRD | Not possible with the same ID: legacy CRDs can't adopt. | Fix forward with DDGR. The [migration safety net](#migration-safety-net) makes sure an attempted rollback never deletes the object. |

### GitOps (Argo CD, Flux)

<details>
<summary>Show details</summary>

GitOps users don't delete the old CR themselves: the GitOps tool prunes it when its manifest leaves Git. A prune deletes the object **as it exists in the cluster**. So the orphan annotation must be on the live CR *before* the manifest is removed. Adding it to the manifest that you remove in the same commit doesn't work: that version is never applied.

**Step 1: put the orphan annotation on the live `DatadogMonitor`.** Pick one:

- **(a) Commit the annotation** to the `DatadogMonitor` manifest and wait for the sync. This is recommended: the safety-critical state lives in Git. `migrate -o yaml` prints the annotated manifest.
- **(b) Annotate in the cluster** with `kubectl datadog monitor migrate <name> --annotate-only`. Argo CD and Flux leave alone annotations that Git doesn't set, including with self-heal: client-side apply only manages fields from the manifest, and server-side apply leaves fields owned by another field manager. This saves a commit, but someone reading the repo can't see the protection.

`migrate --check` reports whether the live CR is annotated. Users run it before step 2.

**Step 2: one commit replaces the `DatadogMonitor` manifest with the DDGR manifest** from `migrate -o yaml`. When the tool syncs:

- The prune deletes the `DatadogMonitor`, and the orphan annotation keeps the monitor.
- The DDGR adopts the monitor and writes the same definition, so the monitor doesn't change.

The design doesn't depend on the order in which the GitOps tool creates the DDGR and prunes the old CR, and the brief makes no assumption about it:
- **Old CR gone first:** the DDGR adopts on its first reconcile.
- **DDGR created first,** or while the old CR is still terminating: the DDGR reports `AdoptionLegacyOwner` and adopts on its first retry after the old CR is gone, at most `requeuePeriod` (60s) later.

```mermaid
sequenceDiagram
  actor U as User
  participant G as GitOps tool
  participant DM as DatadogMonitor controller
  participant DG as DDGR controller
  participant DD as Datadog API
  Note over U,G: Step 1 — commit orphan annotation (or migrate --annotate-only)
  U->>G: commit annotated DatadogMonitor
  G->>DM: apply (annotation only, no spec change)
  U->>U: migrate --check (live CR annotated?)
  Note over U,G: Step 2 — one commit replaces DatadogMonitor with DDGR (spec.id)
  U->>G: commit
  par order not guaranteed
    G->>DM: prune DatadogMonitor
    DM->>DM: finalizer sees orphan, no API call, finalizer removed
  and
    G->>DG: create DDGR
    DG-->>DG: AdoptionLegacyOwner while old CR exists (retry 60s)
  end
  DG->>DD: GET spec.id
  DG->>DG: status.id = spec.id, Adopted=True
  DG->>DD: update with same definition (no change)
```

**The `DatadogMonitor` controller must keep running until the old CR is gone.** The orphan annotation is applied by that controller's finalizer. If the controller is disabled while the CR is deleted, the CR stays terminating, and the DDGR stays blocked (a terminating legacy CR still blocks). Users disable `-datadogMonitorEnabled` only after the migration.

**Without auto-prune** (Argo CD without `prune`, Flux with `prune: false`), the `DatadogMonitor` stays in the cluster after its manifest leaves Git. The DDGR stays blocked, and its condition message says to delete the old CR. Deleting it is safe because it's annotated.

**One manifest deployed to several clusters** (ApplicationSet, one Flux `Kustomization` per cluster): each cluster's `DatadogMonitor` created **its own** monitor, so the IDs differ between clusters.
- `migrate` always writes the full DDGR manifest, including `spec.id`, for the cluster it runs against. Users run it once per kube context.
- Users decide how to lay the result out in their repo, for example one manifest per cluster overlay, or the ID in per-cluster Helm values.
- A DDGR deployed without its `spec.id` creates a new monitor, which is today's behavior. The docs must call this out.

**Templated manifests** (Helm charts, Kustomize bases that generate `DatadogMonitor`s): the tool can't edit the user's templates.
- `migrate -o json` prints just the `jsonSpec` and the ID, ready to paste into a template or values file.
- The migration guide gets a short Helm example that renders `jsonSpec` from values.

**Drift goes away.** The `DatadogMonitor` controller writes `generated:kubernetes` into the CR spec, so GitOps tools often show these CRs as OutOfSync. The DDGR has no such problem: the tag is already in the committed `jsonSpec`, and the DDGR controller never writes to the spec.

</details>

### Migration safety net

<details>
<summary>Show details</summary>

Reverting the step-2 commit is the natural GitOps rollback, and today it would be destructive:
- The revert prunes the DDGR, whose finalizer deletes the adopted monitor.
- The recreated `DatadogMonitor` has no ID, so it creates a new monitor.

To prevent this, **the DDGR generated by `migrate` carries `datadoghq.com/deletion-policy: orphan`**. With it, a revert, or any accidental prune during the migration, never deletes the monitor:

- The DDGR is pruned and the monitor is kept.
- The recreated `DatadogMonitor` tries to create an identical monitor. The API rejects it as a duplicate, so it stays in error and creates nothing.
- Recovery: reapply the step-2 commit. The legacy check doesn't block, because the recreated `DatadogMonitor` has no `status.id`, so the DDGR adopts the monitor again. The broken `DatadogMonitor` is removed in that same commit.

```mermaid
sequenceDiagram
  actor U as User
  participant G as GitOps tool
  participant DG as DDGR controller
  participant DM as DatadogMonitor controller
  participant DD as Datadog API
  Note over U,DD: DDGR generated by migrate carries deletion-policy orphan
  U->>G: revert step-2 commit
  G->>DG: prune DDGR
  DG->>DG: orphan, monitor kept
  G->>DM: recreate DatadogMonitor (no status.id)
  DM->>DD: create identical monitor
  DD-->>DM: 400 Duplicate of an existing monitor_id
  Note over DM: stays in error, creates nothing
  U->>G: reapply step-2 commit
  G->>DM: prune broken DatadogMonitor (nothing to delete)
  G->>DG: create DDGR, legacy check passes (no status.id)
  DG->>DD: adopt again
  Note over U,DD: once validated, migrate --finalize removes the orphan annotation
```

When the user has validated the migration, they drop the annotation from the DDGR manifest. From then on, the DDGR behaves like any other DDGR (deleting it deletes the monitor).
- `migrate` prints this as the final step.
- `kubectl datadog monitor migrate --finalize` removes it for users who apply manifests directly.

Rollback stays unsupported: the safety net makes sure the object is never lost, not that the old CR works again.

</details>

### Old Operator versions

The annotation needs the new Operator version on the legacy controllers. On older versions: disable the legacy controller (`-datadogMonitorEnabled=false`, and so on), remove the finalizer, then delete the CR.

## Adopting resources created outside Kubernetes (UI, API, Terraform)

<details>
<summary>Show details</summary>

1. **Export the definition.** Use the existing `jq` recipes from the migration guide, which strip response-only fields, and add `spec.id`.
2. **Hand off from the previous manager.**
   - **Terraform:** run `terraform state rm`, or use a `removed` block with `lifecycle { destroy = false }`, *before* applying the DDGR. Otherwise both tools fight on every apply and force sync.
   - **UI:** nothing to remove. Optionally restrict edits (monitor/dashboard restriction policies) so that force sync doesn't keep reverting UI changes.
3. **Apply and verify** the `Adopted` and `Updated` conditions.

**Read-only / observe phase: deferred.** A mode that adopts without writing, and reports the diff between `jsonSpec` and the remote object, would make the first update less surprising. It needs a per-type normalization of API responses against create payloads, which we only have for monitors (the definition hash). The first update is already explicit and visible in Events. Users who want a dry run can compare the exported JSON with their `jsonSpec` before applying.

</details>

## Tooling

<details>
<summary>Show details</summary>

| Path | Plan |
| --- | --- |
| `DatadogMonitor` → DDGR | **`kubectl datadog monitor migrate`: priority**, shipped with the feature. |
| `DatadogDashboard` / `DatadogSLO` → DDGR | Manual workflow first. The same command pattern can be extended later. |
| External → DDGR | Manual workflow (API export + `jq` + `spec.id`). `import --type monitor --id 123` is deferred: it would need per-type field-stripping lists kept up to date in code. |

### `kubectl datadog monitor migrate`

It's a new subcommand of the existing plugin (`cmd/kubectl-datadog`).

```shell
kubectl datadog monitor migrate <name> [-n <namespace>] [--dry-run] [-o yaml|json]
kubectl datadog monitor migrate <name> --annotate-only   # GitOps option (b)
kubectl datadog monitor migrate <name> --check           # is the live CR annotated? is the DDGR adopted?
kubectl datadog monitor migrate <name> --finalize        # drop the safety-net annotation from the DDGR
```

1. **Preflight.** Read the `DatadogMonitor` and refuse to continue unless:
   - `status.id` is set.
   - The CR is in sync: `status.currentHash` matches its spec, and there's no error condition. Otherwise the live monitor may not match the CR.
   - It isn't already terminating.
   - No DDGR in the cluster already uses that ID.
2. **Build `jsonSpec` from the CR, not from the API.** Reuse the controller's own conversion (`buildMonitor` in `internal/controller/datadogmonitor/monitor.go`, which needs to be exported). This gives three guarantees:
   - The DDGR sends the same payload the old controller sent, so the first DDGR update doesn't change the monitor's definition.
   - The `generated:kubernetes` tag the old controller injected is kept.
   - The tool needs no Datadog API or application keys.
3. **Build the DDGR:**
   - Same name and namespace (it's a different kind, so they don't clash).
   - User labels and annotations are copied over. GitOps tracking metadata (`argocd.argoproj.io/*`, `kustomize.toolkit.fluxcd.io/*`, `kubectl.kubernetes.io/last-applied-configuration`) is not.
   - `type: monitor`, `id: <status.id>`.
   - `datadoghq.com/deletion-policy: orphan`, the [migration safety net](#migration-safety-net).
4. **Apply** (skipped with `--dry-run` and with `-o`):
   1. Annotate the old CR with `deletion-policy=orphan`.
   2. Delete it and wait until it's gone.
   3. Create the DDGR and wait for `Adopted=True` and `Updated=True`.
   4. Print the `--finalize` step.
5. **Resumable.** Rerunning the command after a failure detects where it stopped and continues from there. The possible states are: old CR annotated, old CR gone with no DDGR, and DDGR created but not yet adopted.
6. **Output modes for GitOps.** With any of these, the tool changes nothing in the cluster:

| Output | Contents |
| --- | --- |
| `-o yaml` | The annotated `DatadogMonitor` (step 1a) and the DDGR (step 2) |
| `-o json` | Only `jsonSpec` and the ID, for Helm or other templates |

The command runs with the user's kubeconfig and needs `get/update/delete` on `datadogmonitors` and `get/create/update` on `datadoggenericresources`.

`--all` (migrating every `DatadogMonitor` in a namespace) is a straightforward follow-up once the single-resource flow is proven.

</details>

## Supported types and limitations

Adoption doesn't depend on the type: it's a GET by ID followed by the existing update path. Every type addressed by ID can be enabled at once. Tests and docs focus on the migration targets.

| Type | Initial support | Notes |
| --- | --- | --- |
| `monitor` | Yes, priority | Integer ID. Monitor `type` changes are limited by the API. `restriction_policy` disables the hash skip. |
| `dashboard` | Yes, priority | The update replaces the whole dashboard (PUT). Check layout constraints in the POC. |
| `slo` | Yes, priority | Keeps `monitor_ids` references, which is a main reason for adoption. Check type immutability in the POC. |
| `notebook`, `synthetics_api_test`, `synthetics_browser_test`, `downtime`, `monitor_notification_rule` | Yes | Generic path. Light test coverage. |
| `slo_correction` | Yes | `slo_id` is create-only. The handler already rejects changing it, and the same check applies after adoption. |
| `software_catalog_entity` | **No** (the controller rejects `spec.id` with `AdoptionUnsupportedType`) | Upsert is keyed by `kind/namespace/name`, so creating the CR already "adopts" an existing entity. `spec.id` would be redundant or conflicting. |

## Non-goals

- Adopting by name or definition matching, and adopt-on-duplicate. The monitor API rejects identical monitors with `Duplicate of an existing monitor_id:<id>`. Using that ID implicitly would be monitor-only, rely on undocumented matching rules, and could take over an object owned by someone else.
- Detecting owners across clusters or tools, and multiple active owners.
- Adoption in legacy CRDs.
- Bulk discovery or import of every resource in an org.
- Supported active/passive disaster recovery (see below).

## Side benefit: active/passive clusters

<details>
<summary>Show details</summary>

Supporting DR isn't a goal. Even so, `spec.id` plus the orphan policy give the customer's active/passive setup a reasonable pattern without any DR-specific code:

1. Create the resources once in the active cluster, then write each `status.id` back into the manifests as `spec.id`.
2. Deploy **identical manifests to both clusters**. Run the DDGR controller only in the active one: in the passive cluster, set `-datadogGenericResourceEnabled=false` or scale the Operator to 0.
3. **Failover:** enable the controller in the new active cluster. It adopts the existing objects by ID, so there's no duplicate, no new ID and no lost history.
4. **Retire the old cluster:** stop its controller, or annotate its CRs with `orphan` and delete them.

Still unsupported: both controllers active at the same time (they overwrite each other on force sync, and this isn't detected), and recovering from an out-of-band delete. With `spec.id` set, an out-of-band delete produces an error instead of a recreate. That's what keeps the two clusters from splitting into two monitors.

</details>

## Compatibility

- `spec.id` is optional. Without it, nothing changes for existing CRs.
- No CEL or webhook, so the CRD keeps working on every Kubernetes version we support.
- The annotation is opt-in. Without it, finalizer behavior doesn't change.
- **Downgrade risk:** an older CRD prunes `spec.id`. An *already adopted* CR keeps `status.id` and keeps working. A CR that was *not yet adopted* (empty `status.id`) would be **created as a new object** by an older Operator. Document this: finish adoptions before downgrading.
- The migration guide needs updating: remove "DDGR does not adopt an existing object" and describe the new workflow (CONTP-2283).

## Test matrix

<details>
<summary>Show details</summary>

| # | Case | Expected |
| --- | --- | --- |
| 1 | `spec.id` set, object exists (monitor, dashboard, slo) | `Adopted=True`, same ID, `jsonSpec` applied once, no create call |
| 2 | `spec.id` set, object missing | `AdoptionTargetNotFound`, no create, retry at force sync |
| 3 | `spec.id` set, 403 | `AdoptionForbidden`, no create, remote object untouched |
| 4 | Invalid ID format (for example `abc` for a monitor) | Permanent error, no API call |
| 5 | Wrong `spec.type` for the ID | `AdoptionTargetNotFound` |
| 6 | Two DDGRs with the same type and id | The one with `status.id` set, or else the older one, wins. The other gets `AdoptionConflict`. |
| 7a | `spec.id` changed after adoption | `IDMismatch`, no remote action, `status.id` kept |
| 7b | `spec.id` removed after adoption | Accepted. Existing `status.id`-based behavior resumes (a 404 recreates). |
| 8 | `spec.id` added later, equal to `status.id` | Accepted, no remote call |
| 8b | Upgrade with existing DDGRs that have no `spec.id` | Same `currentHash`, no updates triggered |
| 9 | `spec.id` added later, different from `status.id` | `IDMismatch`, no remote action |
| 10 | Adopted object deleted out of band | Error, **no recreate** |
| 11 | Stale cache during adoption (`status.id` already persisted) | No second adoption or update from the stale object |
| 12 | Crash between persisting `status.id` and the first update | Next reconcile updates. No duplicate. |
| 13 | `deletion-policy: orphan` on DDGR, DatadogMonitor, DatadogDashboard, DatadogSLO | CR deleted, remote object kept, `Orphaned` event |
| 14 | Annotation added to a CR already terminating with a failing delete | Finalizer released |
| 15 | Full legacy migration (annotate → delete → adopt) for monitor, dashboard, slo | Same ID. References (composite monitor, SLO `monitor_ids`) still resolve. |
| 16 | `software_catalog_entity` with `spec.id` | `AdoptionUnsupportedType`, no API call |
| 17 | No `spec.id` | Unchanged behavior, including recreate on 404 |
| 18 | `slo_correction` adopted, `slo_id` changed | Existing rejection still applies |
| 19 | DDGR adopts an ID held by a `DatadogMonitor` (also with the `DatadogMonitor` controller disabled) | `AdoptionLegacyOwner`, no remote action. Adopts after the legacy CR is deleted with `orphan`. |
| 20 | Legacy CR is terminating with the same ID | Still blocked until it's gone |
| 21 | Legacy CRD not installed | Check passes |
| 22 | Operator can't `list` the legacy kind | Fails closed with an explicit RBAC error |
| 23 | `monitor migrate` happy path | Same monitor ID, no definition change on the first DDGR update, `generated:kubernetes` tag kept |
| 24 | `monitor migrate` preflight: CR out of sync, in error, or without `status.id` | Refuses, changes nothing |
| 25 | `monitor migrate` interrupted after each step, then rerun | Picks up where it stopped. Monitor never deleted. |
| 26 | `monitor migrate -o yaml` / `--dry-run` | Prints manifests and changes nothing in the cluster |
| 27a | GitOps: DDGR created before the `DatadogMonitor` is pruned | `AdoptionLegacyOwner`, then adopts at most `requeuePeriod` after the old CR is gone |
| 27b | GitOps: `DatadogMonitor` pruned before the DDGR is created | Adopts on the first reconcile |
| 27c | `DatadogMonitor` controller disabled while the annotated CR is deleted | CR stays terminating. DDGR blocked with an explicit message. Monitor kept. |
| 27d | `dashboard` adoption with the Helm chart and `datadogDashboard.enabled=false` | Adoption proceeds once the chart change grants `list` on `datadogdashboards` |
| 28 | GitOps: annotation and manifest removal in the same commit | Monitor deleted (documented failure mode). `migrate --check` would have reported "not annotated". |
| 29 | `--annotate-only`, then Argo CD self-heal / Flux reconcile (client-side and server-side apply) | Annotation survives |
| 30 | No auto-prune | DDGR blocked until the old CR is deleted by hand, with an explicit message |
| 31 | Same `DatadogMonitor` manifest in 2 clusters, `migrate -o yaml` run per cluster | Each output has that cluster's `spec.id`. Each DDGR adopts its own cluster's monitor. |
| 32 | Revert of the step-2 commit with the safety-net annotation | DDGR pruned, monitor kept. Recreated `DatadogMonitor` fails as a duplicate. Reapplying step 2 adopts again. |
| 33 | `--finalize` | Annotation removed. Deleting the DDGR afterwards deletes the monitor. |

</details>

## Open questions

1. **Field name:** `spec.id` (mirrors `status.id`) or something more explicit (`spec.datadogID`, `spec.adoptID`)?
2. **Default deletion behavior for adopted resources:** delete the object like any other DDGR (recommended, uniform) or orphan it by default (safer, matches `DatadogMonitor`'s `primary`)?
3. Is a **per-object pause annotation** (for example `datadoghq.com/reconcile: paused`) worth adding? It would make the passive-cluster pattern work without disabling a whole controller. It's a separate feature, deferred unless reviewers disagree.
4. Should the **safety-net annotation** on migrated DDGRs stay on by default? It protects against reverts and accidental prunes, but a user who never runs `--finalize` keeps orphan-on-delete forever. An alternative: on by default for the `-o` outputs only, since reverts are a GitOps risk.
5. The recovery after a reverted migration relies on the monitor API rejecting an identical monitor. If the recreated `DatadogMonitor` differs in any field, it creates a second monitor instead. That's acceptable for an unsupported rollback, but worth confirming the duplicate rules with the monitors team.
