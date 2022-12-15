### Override

Below table lists parameters which can be used to override default agent settings. Maps and arrays have a type annotation in the table; properties which are configured as map values contain a `[key]` element which should be replaced by actual map key. `override` itself is a map with following possible keys `nodeAgent`, `clusterAgent` or `clusterChecksRunner`. Other keys can be added but it will not have any effect.

For example below manifest can be used to override node agent image and tag. Configuration `spec.override.nodeAgent.image.name` will appear as `[key].image.name` in the table.

```yaml
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
```