## Overview

The Datadog Operator is [certified by RedHat's Marketplace][1].

## Deploy the Datadog Operator in an OpenShift cluster

Use the [Operator Lifecycle Manager][2] to deploy the Datadog Operator from OperatorHub in your OpenShift Cluster web console.

1. In OperatorHub or the OpenShift Web Console, search for the Datadog Operator and click **Install**.

![Datadog Operator in the OperatorHub](assets/operatorhub.png)

Installation includes the creation of a `ServiceAccount` called `datadog-agent-scc` that is bound to two default OpenShift `SecurityContextConstraints` (`hostaccess` and `privileged`), which are required for the Datadog Agent to run.

2. Specify the namespace to install the Datadog Operator in, you can use the default `openshift-operators` or a different existing one:

![Deploy the operator in the openshift-operators namespace](assets/openshiftoperatornamespace.png)

**Note**: Prior to version 1.0, multiple `InstallModes` were supported in the `ClusterServiceVersion` (see the [OLM operator install doc][3] as a reference). Due to the introduction of the conversion webhook in 1.0, only the `AllNamespaces` `InstallMode` [is supported][4] in versions 1.0 and later.

## Deploy the Datadog Agent with the Operator

After deploying the Datadog Operator, create a `DatadogAgent` resource that triggers a deployment of the Datadog Agent in your OpenShift cluster. The Agent is deployed as a `DaemonSet`. Datadog recommends that you use the Cluster Agent to manage cluster-level monitoring, which will automatically be deployed by default as an additional `Deployment`.


**Notes**:
- In Datadog Operator versions `1.1` and later, the conversion webhook is **disabled** by default. To enable the webhook, use the command argument `--webhookEnabled`.
- In Datadog Operator version `1.0`, listing the conversion webhook is **enabled** by default. The conversion allows a smooth transition from the (deprecated) `v1alpha1` `DatadogAgent` CRD to `v2alpha1`.


1. In the namespace where the Datadog Operator was deployed, create a Kubernetes secret with your API and App keys:

   ```shell
   oc create secret generic datadog-secret -n openshift-operators --from-literal api-key=<DATADOG_API_KEY> --from-literal app-key=<DATADOG_APP_KEY>
   ```
   Replace `<DATADOG_API_KEY>` and `<DATADOG_APP_KEY>` with your [Datadog API][5] and [Application keys][6].


2. Create a file with the manifest of your `DatadogAgent` deployment.

The following is the simplest recommended configuration for the `DatadogAgent` in OpenShift:

  ```yaml
  apiVersion: datadoghq.com/v2alpha1
  kind: DatadogAgent
  metadata:
    name: datadog
    namespace: openshift-operators # same namespace as where the Datadog Operator was deployed
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
        # This is needed if the kubelet certificate is self-signed.
        # Alternatively, the CA certificate used to sign the kubelet certificate can be mounted.
        tlsVerify: false
    override:
      nodeAgent:
        # In OpenShift 4.0+, set the hostNetwork parameter to allow access to your cloud provider metadata API endpoint.
        # This is necessary to retrieve host tags and aliases collected by Datadog cloud provider integrations. 
        hostNetwork: true
        image:
          name: gcr.io/datadoghq/agent:latest
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
        replicas: 2 # High-availability
        image:
          name: gcr.io/datadoghq/cluster-agent:latest
  ```

Setting the `serviceAccountName` in the `nodeAgent` and `clusterAgent` `override` section ensures that these pods are associated with the necessary `SecurityContextConstraints` and RBACs.

3. Apply the Datadog Agent manifest:
   ```shell
   oc apply -f path/to/your/datadog-agent.yaml
   ```

The Datadog Agent and Cluster Agent should now be running and collecting data. This data can be viewed and alerted on in the Datadog web app.


## Known issues
### Datadog Operator 1.1.0-1.4.1 on OpenShift

When upgrading from versions 1.1.0-1.4.1 of the Datadog Operator bundle provided to OperatorHub in OpenShift, the following error occurs:

```
message: 'retrying execution due to error: error validating existing CRs against new CRD''s schema for "datadogagents.datadoghq.com": error listing resources in GroupVersionResource schema.GroupVersionResource{Group:"datadoghq.com", Version:"v1alpha1", Resource:"datadogagents"}: conversion webhook for datadoghq.com/v2alpha1, Kind=DatadogAgent failed: Post "https://datadog-operator-webhook-service.openshift-operators.svc:443/convert?timeout=30s": no endpoints available for service "datadog-operator-webhook-service"'
```

#### Background

Datadog Operator 1.0 made significant changes to the DatadogAgent CRD, and thus included a conversion webhook (enabled by default) to assist users in converting the v1alpha1 CRD version to v2alpha1. The conversion webhook is disabled by default in 1.1.0.

Datadog Operator bundles 1.1.0-1.4.1 provided to OperatorHub in OpenShift include incomplete references to the conversion pathway. As a result, in the presence of a DatadogAgent custom resource (CR), pre-flight validation by Operator Lifecycle Manager attempts to run the conversion on the CR and then fails due to an undefined endpoint, as seen in the error message.

#### Resolution

This issue is resolved in Datadog Operator bundle 1.4.2+. Use the OperatorHub UI to uninstall the Datadog Operator and reinstall 1.4.2+. Do not delete the DatadogAgent resource.

For further help, contact [Datadog Support][7].



[1]: https://catalog.redhat.com/software/operators/detail/5e9874986c5dcb34dfbb1a12#deploy-instructions
[2]: https://olm.operatorframework.io/
[3]: https://olm.operatorframework.io/docs/tasks/install-operator-with-olm/
[4]: https://olm.operatorframework.io/docs/advanced-tasks/adding-admission-and-conversion-webhooks/#conversion-webhook-rules-requirements
[5]: https://app.datadoghq.com/organization-settings/api-keys
[6]: https://app.datadoghq.com/organization-settings/application-keys
[7]: https://www.datadoghq.com/support/


