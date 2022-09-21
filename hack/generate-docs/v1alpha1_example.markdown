```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  clusterName: my-test-cluster
  credentials:
    apiSecret:
      secretName: datadog-secret
      keyName: api-key
    appSecret:
      secretName: datadog-secret
      keyName: app-key
```
