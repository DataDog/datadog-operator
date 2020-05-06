# kubectl datadog plugin

## install the plugin with krew

The Datatadog operator comes with a kubectl plugin that provides a set of helper went you want to get some inside components information.

The installation is done with the [plugin manager krew](https://krew.sigs.k8s.io/)

The krew plugin manifest url can be found on the project [release page](https://github.com/DataDog/datadog-operator/releases). Each release has an artifact: `datadog-plugin.yaml` manifest file.

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

## available commands

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

### agent sub-commands

```console
$ kubectl datadog agent --help
Usage:
  datadog agent [command]

Available Commands:
  check       Find check errors
  find        Find datadog agent pod monitoring a given pod
  upgrade     Upgrade the Datadog Agent version

```

### clusteragent sub-commands

```console
$ kubectl datadog clusteragent --help
Usage:
  datadog clusteragent [command]

Available Commands:
  leader      Get Datadog Cluster Agent leader
  upgrade     Upgrade the Datadog Cluster Agent version
```
