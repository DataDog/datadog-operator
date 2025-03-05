# Custom Checks

To run a [custom check][1], you can configure the `DatadogAgent` resource to provide custom checks (`checks.d`) and their corresponding configuration files (`conf.d`) at initialization time. You must configure a ConfigMap resource for each check script file and its configuration file.

This page explains how to set up a custom check, `hello`, that submits a `hello.world` metric to Datadog.

To learn more about checks in the Datadog ecosystem, see [Introduction to Integrations][2]. To configure a [Datadog integration][3], see [Kubernetes and Integrations][4]

## Create the check files

Each check needs a configuration file (`hello.yaml`) and a script file (`hello.py`).

1. Create `hello.yaml` with the following content:

   ```yaml
   init_config:

   instances: [{}]
   ```

2. Create `hello.py` with the following content:

   ```python
   from datadog_checks.base import AgentCheck

   __version__ = "1.0.0"
   class HelloCheck(AgentCheck):
       def check(self, instance):
           self.gauge('hello.world', 1, tags=['env:dev'])
   ```

## Create the check ConfigMaps

After you create the `hello` check files, create the associated ConfigMaps:

1. Create the ConfigMap for the custom check YAML configuration file `hello.yaml`:

   ```bash
   $ kubectl create configmap -n $DD_NAMESPACE confd-config --from-file=hello.yaml
   configmap/confd-config created
   ```

2. Verify that the ConfigMap has been correctly created:

   ```bash
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

   ```bash
   $ kubectl create configmap -n $DD_NAMESPACE checksd-config --from-file=hello.py
   configmap/checksd-config created
   ```

4. Verify that the ConfigMap has been correctly created:

   ```bash
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

## Configure the Datadog Agent

After you create your ConfigMaps, create a `DatadogAgent` resource to use them:

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
      extraChecksd:
        configMap:
          name: checksd-config
```

**Note**: Any ConfigMaps you create need to be in the same `DD_NAMESPACE` as the `DatadogAgent` resource.

This deploys the Datadog Agent with your custom check.

### ConfigMaps for multiple checks

You can populate ConfigMaps with the content of multiple checks or their respective configuration files.

#### Populating all check script files

```bash
$ kubectl create cm -n $DD_NAMESPACE checksd-config $(find ./checks.d -name "*.py" | xargs -I'{}' echo -n '--from-file={} ')
configmap/checksd-config created
```

#### Populating all check configuration files

```bash
$ kubectl create cm -n $DD_NAMESPACE confd-config $(find ./conf.d -name "*.yaml" | xargs -I'{}' echo -n '--from-file={} ')
configmap/confd-config created
```

## Provide additional volumes

You can mount additional user-configured volumes in either the node or Cluster Agent containers by setting the `volumes` and `volumeMounts` properties. 

**Example**: Using a volume to mount a secret

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
      image:
        name: "gcr.io/datadoghq/agent:latest"
      volumes:
        - name: secrets
          secret:
            secretName: secrets
      containers:
        agent:
          volumeMounts:
            - name: secrets
              mountPath: /etc/secrets
              readOnly: true
```
[1]: https://docs.datadoghq.com/developers/custom_checks/
[2]: https://docs.datadoghq.com/getting_started/integrations/
[3]: https://docs.datadoghq.com/integrations/
[4]: https://docs.datadoghq.com/containers/kubernetes/integrations/?tab=annotations
