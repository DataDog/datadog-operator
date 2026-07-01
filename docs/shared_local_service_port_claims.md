# Shared local Service port claims

The node Agent local Service (`<datadogagent>-agent`) aggregates ports from
several features (APM, DogStatsD, OTLP, OTel Collector). With
`DatadogAgentProfile` overrides, those features can differ per node group, so the
Service's desired ports are a function of **all** `DatadogAgentInternal` (DDAI)
resources (default + one per applied profile).

A single DDAI reconcile only sees its own spec, and the dependency store applies
Services by overwrite (it doesn't merge live ports). So we can't build the
Service from one DDAI, and we can't let every DDAI write it directly without
clobbering. Instead, each DDAI publishes a **port claim** on the Service, and one
writer merges them.

> Status: APM is the first feature on this mechanism. DogStatsD, OTLP, and the
> OTel Collector still add ports via the legacy in-store path and migrate later.

## Annotation schema

One annotation per `(DDAI, feature)`, value = managed `corev1.ServicePort` fields
(`name`, `port`, `targetPort`, `protocol`) as JSON:

```
operator.datadoghq.com/port-claim.<ddai-name>.<feature-id>: [{"name":"traceport","protocol":"TCP","port":8126,"targetPort":8126}]
```

Merge rule: union by port **name**. Same name + identical spec = shared; same name
+ different spec = conflict (`merger.ErrServicePortConflict`).

## Flow

- **Default DDAI** (owns the Service, sole writer of `Spec.Ports`): collects its
  own feature claims, reads the live `port-claim.*` annotations (claims published
  by profile DDAIs), writes own claims + preserves the others, merges all claims
  into `Spec.Ports`, and applies via the dependency store (owner ref + labels).
- **Profile DDAI** (publisher only): JSON-merge-patches *only* its own
  `port-claim.<ddai>.<feature>` key onto the Service.

Both touch the Service but on disjoint fields (profile DDAIs → their annotation
key; default DDAI → `Spec.Ports`), so they don't clobber. The Service converges
over reconciles: profile publishes its claim → default DDAI (which owns/watches
the Service) re-reconciles and merges it.

Features opt in by implementing `feature.LocalServicePortClaimer`
(`LocalServicePortClaim() []corev1.ServicePort`). The DDAI controller collects
claims generically, so it has no feature-specific imports — spec→ports logic
stays in the feature, the controller only does a dumb, conflict-checked merge.

## Conflicts, ownership, cleanup

- **Conflict**: the offending profile gets a dedicated `ServicePortConflict=True`
  / reason `Conflict` condition (attributed to the claim that sorts later); the
  default DDAI reconcile errors and requeues; the conflicting port is not
  written. A distinct condition is used — not `Applied` — because the DDA
  controller's profile reconciliation owns `Applied`, so the two would contend.
  The condition clears to `False` once the claims merge cleanly.
- **Ownership**: the Service is owned by the default DDAI (single owner) and GC'd
  with it — no multi-owner refs.
- **Cleanup**: a deleted profile DDAI's finalizer removes its own
  `port-claim.<ddai>.*` annotations, so its ports stop being merged.

## Code map

| Concern | Location |
| --- | --- |
| Annotation schema, encode/decode, claim merge | `merger/port_claim.go` |
| Conflict-detecting port union | `merger/service.go` (`MergeServicePorts`, `ErrServicePortConflict`) |
| Feature opt-in interface | `feature/types.go` (`LocalServicePortClaimer`) |
| APM claim | `feature/apm/feature.go` (`LocalServicePortClaim`) |
| Publish / merge / cleanup | `datadogagentinternal/controller_reconcile_localservice.go` |
| Finalizer claim removal | `datadogagentinternal/finalizer.go` |

See [DatadogAgentInternal](datadog_agent_internal.md) and
[DatadogAgentProfiles](datadog_agent_profiles.md) for the DDA/DDAI split this
builds on.
