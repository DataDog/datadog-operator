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

Installation includes the creation of a `ServiceAccount` called `datadog-agent-scc` that is bound to two default OpenShift `SecurityContextConstraints`.

3. Specify the namespace to install the Datadog Operator in, you can use `datadog` if you previously created the project or an existing one, such as `openshift-operators`:

![Deploy the operator in the datadog namespace](assets/datadognamespace.png)

## Deploy the Datadog Agent with the Operator

After deploying the Datadog Operator, create a `DatadogAgent` resource that triggers a deployment of the Datadog Agent in your OpenShift cluster. The Agent is deployed as a `Daemonset`. It is recommended to use the Cluster Agent to manage cluster-level monitoring, and this will also be deployed by default.

1. Create a Kubernetes secret with your API and App keys:

   ```shell
   oc create secret generic datadog-secret -n datadog --from-literal api-key=<DATADOG_API_KEY> --from-literal app-key=<DATADOG_APP_KEY>
   ```
   Replace `<DATADOG_API_KEY>` and `<DATADOG_APP_KEY>` with your [Datadog API][3] and [Application keys][4]

**Note**: In Datadog Operator versions `1.1.0` and above, the Webhook Conversion is **disabled** by default, and can be enabled with the command argument `--webhookEnabled`. In version `1.0.3` of the Datadog Operator, listing the Webhook Conversion is **enabled** by default. The conversion allows a smooth transition from the `v1alpha1` (deprecated) `DatadogAgent` CRD to `v2alpha1`.

2. Create a file with the manifest of your `DatadogAgent` deployment.

The following is the simplest recommended configuration for the `DatadogAgent` in OpenShift:

  ```yaml
  apiVersion: datadoghq.com/v2alpha1
  kind: DatadogAgent
  metadata:
    name: datadog
    namespace: datadog # or openshift-operators depending on where the Datadog Operator was deployed
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
      kubelet:
        tlsVerify: false
    override:
      nodeAgent:
        securityContext:
          runAsUser: 0
          seLinuxOptions:
            level: s0
            role: system_r
            type: spc_t
            user: system_u
        serviceAccountName: datadog-agent-scc
      clusterAgent:
        serviceAccountName: datadog-agent-scc
        replicas: 2
  ```

Setting the `serviceAccountName` in the `nodeAgent` and `clusterAgent` `override` section ensures that these pods are associated with the necessary `SecurityContextConstraints` and RBACs.

3. Apply the Datadog Agent manifest:
   ```shell
   oc apply -f path/to/your/datadog-agent.yaml
   ```

The Datadog Agent and Cluster Agent should now be running and collecting data to be viewed and alerted on in the Datadog web app.

[1]: https://catalog.redhat.com/software/operators/detail/5e9874986c5dcb34dfbb1a12#deploy-instructions
[2]: https://olm.operatorframework.io/
[3]: https://app.datadoghq.com/organization-settings/api-keys
[4]: https://app.datadoghq.com/organization-settings/application-keys
