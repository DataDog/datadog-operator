# Integrations Autodiscovery

The Datadog Operator makes it easy for you to configure Datadog [integrations][2] for [Autodiscovery][1] in your Datadog Agent. The `DatadogAgent` resource can be configured to provide configuration files (`conf.d`) at initialization time.

## Define ConfigMap in the `DatadogAgent` Resource

Use the `spec.override.nodeAgent.extraConfd.configDataMap` field to define your check's configuration:

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
 global:
   credentials:
     apiKey: "<DATADOG_API_KEY>"
     appKey: "<DATADOG_APP_KEY>"
 override:
  nodeAgent:
    extraConfd:
      configDataMap:
        http_check.yaml: |-
          init_config:
          instances:
            - url: "http://%%host%%"
              name: "My service"
```

## Static ConfigMap

Alternatively, you can define your check's configuration with a `ConfigMap` and mount it to the node Agent using the `DatadogAgent` resource. Below is an example of configuring your own `ConfigMap` for the HTTP Check.

### Create the check configuration file

Create the HTTP Check configuration file `http_check.yaml`:

 ```yaml
 init_config:
 instances:
   - url: "http://%%host%%"
     name: "My service"
 ```

### Create the ConfigMap

1. Create the ConfigMap for the HTTP Check YAML configuration file `http_check.yaml`:

```shell
$ kubectl create configmap -n $DD_NAMESPACE confd-config --from-file=http_check.yaml
configmap/confd-config created
```

2. Verify that the ConfigMap has been correctly created:

```shell
$ kubectl get configmap -n $DD_NAMESPACE confd-config -o yaml
apiVersion: v1
data:
  http_check.yaml: |-
    init_config:
    instances:
      - url: "http://%%host%%"
        name: "My service"
kind: ConfigMap
metadata:
  name: confd-config
  namespace: datadog
```

### Configure the Agent

Once the `ConfigMap` is configured, create a `DatadogAgent` resource and specify the `ConfigMap` using the `spec.override.nodeAgent.extraConfd.configMap.name` field:

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  global:
    credentials:
      apiKey: "<DATADOG_API_KEY>"
      appKey: "<DATADOG_APP_KEY>"
  override:
    nodeAgent:
      extraConfd:
        configMap:
          name: confd-config
```

If your `ConfigMap` has multiple data keys defined, you can specify them using the `spec.override.nodeAgent.extraConfd.configMap.items` field:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
   name: confd-config
   namespace: datadog
data:
  http_check.yaml: |-
    init_config:
    instances:
      - url: "http://%%host%%"
        name: "My service"
  redisdb.yaml: |-
     init_config:
     instances:
       - host: %%host%%
         port: "6379"
         username: default
         password: <PASSWORD>
```

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  global:
    credentials:
      apiKey: "<DATADOG_API_KEY>"
      appKey: "<DATADOG_APP_KEY>"
  override:
    nodeAgent:
      extraConfd:
        configMap:
          name: confd-config
          items:
            - key: http_check.yaml
              path: http_check.yaml
            - key: redisdb.yaml
              path: redisdb.yaml
```

## Validation

After configuring your check using one of the above methods and [deploying][3] the Datadog Agent with the `DatadogAgent` resource file, validate that the check is running in the node Agent:

```shell
$ kubectl exec -it <NODE_AGENT_POD_NAME> -- agent status
```

Look for the check under the `Running Checks` section:

```shell
  ...
      http_check (3.1.1)
    ------------------
      Instance ID: http_check:My service:5b948dee172af830 [OK]
      Total Runs: 234
      Metric Samples: Last Run: 3, Total: 702
      Events: Last Run: 0, Total: 0
      Service Checks: Last Run: 1, Total: 234
      Average Execution Time : 90ms
```


[1]: https://docs.datadoghq.com/agent/autodiscovery/
[2]: https://docs.datadoghq.com/getting_started/integrations/
[3]: https://docs.datadoghq.com/getting_started/containers/datadog_operator/#installation-and-deployment
