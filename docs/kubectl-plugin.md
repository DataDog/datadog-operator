# Datadog Operator kubectl plugin

## Install the plugin with krew

The Datadog Operator comes with a kubectl plugin providing a set of helper utilities giving visibility into certain internal components which some users may find useful.

To install, use the [krew plugin manager](https://krew.sigs.k8s.io/).

The krew plugin manifest url can be found on the project [release page](https://github.com/DataDog/datadog-operator/releases). Each release has its own `datadog-plugin.yaml` manifest file.

```console
$ kubectl krew install --manifest-url https://github.com/DataDog/datadog-operator/releases/download/<release-version>/datadog-plugin.yaml
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
