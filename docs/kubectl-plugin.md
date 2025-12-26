# Datadog Operator Plugin for kubectl

The Datadog Operator has a `kubectl` plugin, which provides a set of helper utilities that give visibility into certain internal components.

## Install the plugin

Run:
```shell
kubectl krew install datadog
```

This uses the [Krew plugin manager](https://krew.sigs.k8s.io/).

```console
$ kubectl krew install datadog
Installing plugin: datadog
Installed plugin: datadog
\
 | Use this plugin:
 | 	kubectl datadog
 | Documentation:
 | 	https://github.com/DataDog/datadog-operator
/
```

## Available commands

```console
$ kubectl datadog --help
Usage:
  datadog [command]

Available Commands:
  agent
  clusteragent
  flare        Collect a Datadog's Operator flare and send it to Datadog
  get          Get DatadogAgent deployment(s)
  helm2dda     Map Datadog Helm values to DatadogAgent CRD schema
  help         Help about any command
  validate

```

### Agent sub-commands

```console
$ kubectl datadog agent --help
Usage:
  datadog agent [command]

Available Commands:
  check       Find check errors
  find        Find datadog agent pod monitoring a given pod
  upgrade     Upgrade the Datadog Agent version

```

### Cluster Agent sub-commands

```console
$ kubectl datadog clusteragent --help
Usage:
  datadog clusteragent [command]

Available Commands:
  leader      Get Datadog Cluster Agent leader
  upgrade     Upgrade the Datadog Cluster Agent version
```

### Validate sub-commands

```console
$ kubectl datadog validate ad --help
Usage:
  datadog validate ad [command]

Available Commands:
  pod         Validate the autodiscovery annotations for a pod
  service     Validate the autodiscovery annotations for a service
```

### Helm2DDA sub-commands

```console
$ kubectl datadog helm2dda --help
Usage:
  datadog helm2dda [DatadogAgent name] --sourcePath <source_values_path> [flags]

Flags:
      --ddaName string       DatadogAgent custom resource name.
  -d, --destPath string      Path to destination YAML file.
  -p, --headerPath string    Path to header YAML file. The content in this file will be prepended to the output.
  -h, --help                 help for helm2dda
  -m, --mappingPath string   Path to mapping YAML file.
  -n, --namespace string     If present, the namespace scope for this CLI request
  -o, --printOutput          print mapped DDA output to stdout (default true)
  -f, --sourcePath string    Path to source YAML file. Required. Example: source.yaml
  -u, --updateMap            Update 'mappingPath' with provided 'sourcePath'. If set to 'true', default mappingPath is mapping_datadog_helm_to_datadogagent_crd.yaml and default sourcePath is latest published Datadog chart values.yaml.
```

Refer to the [Datadog Helm Operator Migration Guide][] for full instructions on how to migrate your Datadog Helm installation to the Datadog Operator.

[]:
