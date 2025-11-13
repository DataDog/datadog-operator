## Override options

The following table lists parameters that can be used to override default or global settings for individual components. `override` is a map with the following possible keys: `nodeAgent`, `clusterAgent`, or `clusterChecksRunner`. Maps and arrays have a type annotation in the table. In the parameter names, `component` refers to one of these component keys, and `container` refers to a specific container name within that component (such as `agent`, `cluster-agent`, `process-agent`, `trace-agent`, or `system-probe`).

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
In the table, `spec.override.nodeAgent.image.name` and `spec.override.nodeAgent.containers.system-probe.resources.limits` appear as `[component].image.name` and `[component].containers.[container].resources.limits`, respectively.