# Custom Checks with Datadog Operator

This page discusses [custom checks][4]. To configure a [Datadog Integration][3], use the [Agent Autodiscovery feature][1]. 

The `DatadogAgent` resource can be configured to provide custom checks (`checks.d`) and their configuration files (`conf.d`) at initialization time. A `ConfigMap` resource needs to be configured for each of these settings before the `DatadogAgent` resource using them is created.

Below is an example of configuring these `ConfigMaps` for a single check `hello` that submits a `hello.world` metric to Datadog. See [Introduction to Integrations][2] to learn about checks in the Datadog ecosystem.

## Create the check files

This check needs a configuration file `hello.yaml` and a script file `hello.py`:

1. Create **`hello.yaml`** with the following content:

   ```yaml
   init_config:

   instances: [{}]
   ```

2. Create **`hello.py`** with the following content:

   ```python
   from datadog_checks.base import AgentCheck

   __version__ = "1.0.0"
   class HelloCheck(AgentCheck):
       def check(self, instance):
           self.gauge('hello.world', 1, tags=['env:dev'])
   ```

## Create the check ConfigMaps

Once you have created the `hello` check files, create the associated `ConfigMaps`:

1. Create the ConfigMap for the custom check YAML configuration file `hello.yaml`:

   ```shell
   $ kubectl create configmap -n $DD_NAMESPACE confd-config --from-file=hello.yaml
   configmap/confd-config created
   ```

2. Verify that the ConfigMap has been correctly created:

   ```shell
   $ kubectl get configmap -n $DD_NAMESPACE confd-config -o yaml
   apiVersion: v1
   data:
     hello.yaml: |
       init_config:

       instances: [{}]
   kind: ConfigMap
   metadata:
     name: confd-config
     namespace: datadog
   ```

3. Create the ConfigMap for the custom check Python file `hello.py`:

   ```shell
   $ kubectl create configmap -n $DD_NAMESPACE checksd-config --from-file=hello.py
   configmap/checksd-config created
   ```

4. Verify that the ConfigMap has been correctly created:

   ```shell
   $ kubectl get configmap -n $DD_NAMESPACE checksd-config -o yaml
   apiVersion: v1
   data:
     hello.py: |
      from datadog_checks.base import AgentCheck

      __version__ = "1.0.0"
      class HelloCheck(AgentCheck):
        def check(self, instance):
          self.gauge('hello.world', 1, tags=['env:dev'])
    kind: ConfigMap
    metadata:
      name: checksd-config
      namespace: datadog
   ```

## Configure the Agent

Once you have configured the `ConfigMaps` , you can create a `DatadogAgent` resource to use them with the following chart:

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  credentials:
    apiKey: "<DATADOG_API_KEY>"
    appKey: "<DATADOG_APP_KEY>"
  agent:
    config:
      confd:
        configMapName: "confd-config"
      checksd:
        configMapName: "checksd-config"
```

**Note**: Any ConfigMaps you create must be in the same `DD_NAMESPACE` as the `DatadogAgent` resource.

This deploys the Datadog Agent with your custom check.

### Multiple checks

To populate `ConfigMaps` with the content of multiple checks or their respective configuration files, you can use the following approach:

- Populating all check configuration files:

  ```shell
  $ kubectl create cm -n $DD_NAMESPACE confd-config $(find ./conf.d -name "*.yaml" | xargs -I'{}' echo -n '--from-file={} ')
  configmap/confd-config created
  ```

- Populating all check script files:

  ```shell
  $ kubectl create cm -n $DD_NAMESPACE checksd-config $(find ./checks.d -name "*.py" | xargs -I'{}' echo -n '--from-file={} ')
  configmap/checksd-config created
  ```

## Providing additional volumes

Additional user-configured volumes can be mounted in either the Node or Cluster Agent containers by setting the `volumes` and `volumeMounts` properties. 

The following is an example of using a volume to mount a secret:

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  credentials:
    apiKey: "<DATADOG_API_KEY>"
    appKey: "<DATADOG_APP_KEY>"
  agent:
    image:
      name: "gcr.io/datadoghq/agent:latest"
    volumes:
      - name: secrets
        secret:
          secretName: secrets
    volumeMounts:
      - name: secrets
        mountPath: /etc/secrets
        readOnly: true
```

[1]: https://docs.datadoghq.com/agent/autodiscovery/
[2]: https://docs.datadoghq.com/getting_started/integrations/
[3]: https://docs.datadoghq.com/integrations/
[4]: https://docs.datadoghq.com/developers/custom_checks