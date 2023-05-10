## Overview

The Datadog Operator is [certified by RedHat's Marketplace][1].

## Deploy the Datadog Operator in an OpenShift cluster

Use the [Operator Lifecycle Manager][2] to deploy the Datadog Operator from OperatorHub in your OpenShift Cluster web console.

1. You can create a `datadog` project in your OpenShift cluster:

   ```shell
   oc new-project datadog
   ```

2. In OperatorHub or the OpenShift Web Console, search for the Datadog Operator and click **Install**.

![Datadog Operator in the OperatorHub](assets/operatorhub.png)

3. Specify the namespace to install the Datadog Operator in, you can use `datadog` if you previously created the project or an existing one, e.g. `openshift-operators`:

![Deploy the operator in the datadog namespace](assets/datadognamespace.png)

## Deploy the Datadog Agents with the operator

After deploying the Datadog Operator, create a `DatadogAgent` resource that triggers the Datadog Agent's deployment in your OpenShift cluster. With this resource, the Agent deploys as a `DaemonSet` on every `Node` of your cluster.

1. Create a Kubernetes secret with your API and APP keys:

   ```shell
   kubectl create secret generic datadog-secret -n datadog --from-literal api-key=<DATADOG_API_KEY> --from-literal app-key=<DATADOG_APP_KEY>
   ```
   Replace `<DATADOG_API_KEY>` and `<DATADOG_APP_KEY>` with your [Datadog API and application keys][3]

**Note**: Starting with the version `1.0.3` of the Datadog Operator listing the Webhook Conversion is enabled by default. This will let you create DatadogAgent objects with the v1alpha1 or the new v2alpha1.

2. Create a file with the spec of your `DatadogAgent` deployment configuration. 

The following contains the simplest configuration using the v2alpha1 object:

  ```yaml
  apiVersion: datadoghq.com/v2alpha1
  kind: DatadogAgent
  metadata:
    name: datadog
    namespace: openshift-operators
  spec:
    global:
      credentials:
        apiSecret:
          keyName: api-key
          secretName: datadog-secret
        appSecret:
          keyName: app-key
          secretName: datadog-secret
      criSocketPath: /var/run/crio/crio.sock
    override:
      nodeAgent:
        containers:
          agent:
            env:
            - name: DD_KUBELET_TLS_VERIFY
              value: "false"
        securityContext:
          runAsUser: 0
          seLinuxOptions:
            level: s0
            role: system_r
            type: spc_t
            user: system_u
        serviceAccountName: datadog-agent-scc
  ```

3. Deploy the Datadog Agent with the configuration file above:
   ```shell
   oc apply -f path/to/your/datadog-agent.yaml
   ```

The Datadog Agent runs with the `datadog-agent-scc` service account created by the Datadog Operator and is connected to the right SCCs.

[1]: https://catalog.redhat.com/software/operators/detail/5e9874986c5dcb34dfbb1a12#deploy-instructions
[2]: https://olm.operatorframework.io/
[3]: https://app.datadoghq.com/organization-settings/api-keys
