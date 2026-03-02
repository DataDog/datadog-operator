# Datadog Plugin for kubectl

Datadog provides a `kubectl` plugin with helper utilities that gives visibility into internal components. You can use the plugin with Operator installations or with the Datadog [Helm chart][1].

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

### Cluster autoscaling

```console
$ kubectl datadog autoscaling cluster install --help
Install autoscaling on an EKS cluster

Usage:
  datadog autoscaling cluster install [flags]

Examples:

  # install autoscaling
  kubectl datadog autoscaling cluster install

Flags:
      --as string
            Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --as-group stringArray
            Group to impersonate for the operation, this flag can be repeated to specify multiple groups.
      --as-uid string
            UID to impersonate for the operation.
      --cache-dir string
            Default cache directory (default "/home/lenaic/.kube/cache")
      --certificate-authority string
            Path to a cert file for the certificate authority
      --client-certificate string
            Path to a client certificate file for TLS
      --client-key string
            Path to a client key file for TLS
      --cluster string
            The name of the kubeconfig cluster to use
      --cluster-name string
            Name of the EKS cluster
      --context string
            The name of the kubeconfig context to use
      --create-karpenter-resources CreateKarpenterResources
            Which Karpenter resources to create: none, ec2nodeclass, all
            (default all)
      --debug
            Enable debug logs
      --disable-compression
            If true, opt-out of response compression for all requests to the server
  -h, --help
            help for install
      --inference-method InferenceMethod
            Method to infer EC2NodeClass and NodePool properties: nodes, nodegroups (default nodegroups)
      --insecure-skip-tls-verify
            If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --karpenter-namespace string
            Name of the Kubernetes namespace to deploy Karpenter into (default "dd-karpenter")
      --karpenter-version string
            Version of Karpenter to install (default to latest)
      --kubeconfig string
            Path to the kubeconfig file to use for CLI requests.
  -n, --namespace string
            If present, the namespace scope for this CLI request
      --request-timeout string
            The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string
            The address and port of the Kubernetes API server
      --tls-server-name string
            Server name to use for server certificate validation. If it is not provided, the hostname used to contact the server is used
      --token string
            Bearer token for authentication to the API server
      --user string
            The name of the kubeconfig user to use
```

[1]: https://github.com/DataDog/helm-charts/tree/main/charts/datadog
