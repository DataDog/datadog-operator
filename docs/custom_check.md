# Custom Check

To configure one of Datadog 400+ integrations, leverage the [Agent Autodiscovery feature][1]. But if you want to run a Custom Check the `DatadogAgent` resource can be configured to provide custom checks (`checks.d`) and their configuration files (`conf.d`) at initialization time. A `ConfigMap` resource needs to be configured for each of these settings before the `DatadogAgent` resource using them is created.

Below is an example of configuring these `ConfigMaps` for a single check `hello` that submits the `hello.world` metrics to Datadog. See the [Introduction to Integrations][2] to learn what is a check in the Datadog ecosystem.

## Create the check files

At its core a check needs a configuration file `hello.yaml` and a script file `hello.py`:

1. Create the **`hello.yaml`** with the following content:

   ```yaml
   init_config:

   instances: [{}]
   ```

2. Create the **`hello.py`** with the following content:

   ```python
   # the following try/except block will make the custom check compatible with any Agent version
   try:
       # first, try to import the base class from new versions of the Agent...
       from datadog_checks.base import AgentCheck
   except ImportError:
       # ...if the above failed, the check is running in Agent version < 6.6.0
       from checks import AgentCheck

   # content of the special variable __version__ will be shown in the Agent status page
   __version__ = "1.0.0"
   class HelloCheck(AgentCheck):
       def check(self, instance):
           self.gauge('hello.world', 1, tags=['env:dev'])
   ```

## Create the check ConfigMaps

Once the `hello` check files are created, create the associated `ConfigMaps`:

1. Create the config map for the custom check yaml configuration file `hello.yaml`:

   ```shell
   $ kubectl create configmap -n $DD_NAMESPACE confd-config --from-file=hello.yaml
   configmap/confd-config created
   ```

2. Verify that the config map has been correctly created:

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

3. Create the config map for the custom check python file `hello.py`:

   ```shell
   $ kubectl create configmap -n $DD_NAMESPACE checksd-config --from-file=hello.py
   configmap/checksd-config created
   ```

4. Verify that the config map has been correctly created:

   ```shell
   $ kubectl get configmap -n $DD_NAMESPACE checksd-config -o yaml
   apiVersion: v1
   data:
     hello.py: |
       # the following try/except block will make the custom check compatible with any Agent version
       try:
           # first, try to import the base class from new versions of the Agent...
           from datadog_checks.base import AgentCheck
       except ImportError:
           # ...if the above failed, the check is running in Agent version < 6.6.0
           from checks import AgentCheck

       # content of the special variable __version__ will be shown in the Agent status page
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

Once the `ConfigMaps` are configured, a `DatadogAgent` resource can be created to use them with the following chart:

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgent
metadata:
  name: datadog-agent
spec:
  credentials:
    apiKey: <DATADOG_API_KEY>
    appKey: <DATADOG_APP_KEY>
  agent:
    image:
      name: "datadog/agent:latest"
    confd:
      configMapName: "confd-config"
    checksd:
      configMapName: "checksd-config"
```

**Note**: ConfigMaps created needs to be in the same `DD_NAMESPACE` as the `DatadogAgent` resource.

This deploys the Datadog Agent with your custom check.

### Multiple checks

In order to populate `ConfigMaps` with content of multiple checks or their respective configurations files, the following approach can be used:

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

Additional user-configured volumes can be mounted in either the node or Cluster Agent containers by setting the `volumes` and `volumeMounts` properties. Find below an example of using a volume to mount a secret:

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgent
metadata:
  name: datadog-agent
spec:
  credentials:
    apiKey: <DATADOG_API_KEY>
    appKey: <DATADOG_APP_KEY>
  agent:
    image:
      name: "datadog/agent:latest"
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
