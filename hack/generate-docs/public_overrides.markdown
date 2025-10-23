## Override options

The following table lists parameters that can be used to override default or global settings. Maps and arrays have a type annotation in the table; properties that are configured as map values contain a `[key]` element, to be replaced with an actual map key. `override` itself is a map with the following possible keys: `nodeAgent`, `clusterAgent`, or `clusterChecksRunner`. 

For example: the following manifest overrides the Node Agent's image and tag, in addition to the resource limits of the system probe container:

{{< highlight yaml "hl_lines=6-16" >}}
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  override:
    nodeAgent:
      image:
        name: agent
        tag: 7.41.0-rc.5
      containers:
        system-probe:
          resources:
            limits:
              cpu: "2"
              memory: 1Gi
{{< /highlight >}}
In the table, `spec.override.nodeAgent.image.name` and `spec.override.nodeAgent.containers.system-probe.resources.limits` appear as `[key].image.name` and `[key].containers.[key].resources.limits`, respectively.

{{% collapse-content title="Parameters" level="h4" expanded=true id="override-options-list" %}}
`[key].annotations`
: _type_: `map[string]string`
<br /> Annotations provide annotations that are added to the different component (Datadog Agent, Cluster Agent, Cluster Check Runner) pods.

`[key].containers.[key].healthPort`
: HealthPort of the container for the internal liveness probe. Must be the same as the Liveness/Readiness probes.

`[key].tolerations`
: _type_: `[]object`
<br /> Configure the component tolerations.
{{% /collapse-content %}}

For a complete list of override parameters, see the [Operator configuration spec][9].