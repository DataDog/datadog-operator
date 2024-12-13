# Secrets Management for the API and App keys

Datadog Operator can be configured to retrieve Datadog credentials using secrets for enhanced security. There are three methods you can choose from to set it up:

## 1. Configure plain credentials in `DatadogAgent` resource

This is the simplest way to provide credentials to the agents. This method is recommended for testing purposes only.

Directly add the API and App keys to the DatadogAgent spec:

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  global:
    credentials:
      apiKey: <DATADOG_API_KEY>
      appKey: <DATADOG_APP_KEY>
  # ...
```

The credentials provided here will be stored in a Secret created by the Operator. By properly setting the `RBAC` on the `DatadogAgent` CRD, one can limit who is able to see those credentials.

## 2. Use secret(s) references

Another solution is to provide the name of the secret(s) that contains the credentials:

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  global:
    credentials:
      apiSecret:
        secretName: datadog-secret
        keyName: api-key
      appSecret:
        secretName: datadog-secret
        keyName: app-key
  # ...
```

Create the secret(s) before applying the DatadogAgent manifest, or the deployment will fail.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: test-api-secret
data:
  api_key: <api-key>

---
apiVersion: v1
kind: Secret
metadata:
  name: test-app-secret
data:
  app_key: <app-key>
```

**Note:**

It is possible to use the same secret to store both credentials:

    ```yaml
    ---
    apiVersion: v1
    kind: Secret
    metadata:
      name: test-secret
    data:
      api_key: <api-key>
      app_key: <app-key>
    ```

## 3. Use the secret backend feature

The Datatog Operator is compatible with the ["secret backend" feature][1] implemented.

### How to deploy the Datadog-Operator with the "secret backend" activated

#### Custom secret backend

The first step is to create a `datadog/operator` container image that contains the secret backend command.

If you'd like to build your own, the following Dockerfile example takes the `datadog/operator:latest` image as the base image and copies the `my-secret-backend.sh` script file.

```Dockerfile
FROM datadog/operator:latest
COPY ./my-secret-backend.sh /my-secret-backend.sh
RUN chmod 755 /my-secret-backend.sh
```

```console
$ docker build -t datadog-operator-with-secret-backend:latest .
success
```

Then, install or update the Datadog Operator deployment with the `.Values.secretBackend.command` parameter set to the secret backend command path (inside the container). Don't forget to update the image if using a custom one.

```console
$ helm [install|upgrade] dd-operator --set "secretBackend.command=/my-secret-backend.sh" --set "image.repository=datadog-operator-with-secret-backend" ./chart/datadog-operator
success
```

#### Using the secret helper

Kubernetes supports exposing secrets as files inside a pod, and we provide a helper script in the Datadog Operator image to read the secrets from files.

First, mount the secret in the Operator container, for instance at `/etc/secret-volume`. Then install or update the Datadog Operator deployment with the `.Values.secretBackend.command` parameter set to `/readsecret.sh` and the `.Values.secretBackend.arguments` parameter set to `/etc/secret-volume`.

**Note:** This secret helper requires Datadog Operator v0.5.0+

### How to deploy Agent components using the secret backend feature with the DatadogAgent (Operator 1.11+)

If using a custom script, create a Datadog Agent (or Cluster Agent) image following the example for the Datadog Operator above, and specify credentials using `ENC[<placeholder>]`.

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  global:
    credentials:
      apiKey: ENC[<api-key-secret-id>]
      appKey: ENC[<app-key-secret-id>]
  # ...
```

The secret backend command can be specified in the `spec.global.secretBackend.command`:

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  global:
    secretBackend:
      command: "/my-secret-backend.sh"
  # ...
```

The environment variable `DD_SECRET_BACKEND_COMMAND` from this configuration is automatically applied to all the deployed components: node Agent, Cluster Agent, and Cluster Checks Runners. Ensure the image you are using for all the components includes your command.

For convenience, the Datadog Agent and its sibling Cluster Agent images already include a `readsecret_multiple_providers.sh` [helper function][2] that can be used to read from both files as well as Kubernetes Secrets. After creating the Secret, set `spec.global.secretBackend.command` to `"/readsecret_multiple_providers.sh"`.

For instance, to use the secret backend for the Agent and Cluster Agent, create a secret called "test-secret":

`kubectl create secret generic test-secret --from-literal=api_key='<api-key>' --from-literal=app_key='<app-key>'`

And then set the DatadogAgent spec:

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  global:
    secretBackend:
      command: "/readsecret_multiple_providers.sh"
    credentials:
      apiKey: ENC[k8s_secret@default/test-secret/api_key]
      appKey: ENC[k8s_secret@default/test-secret/app_key]
```

**Remarks:**

* The `"/readsecret_multiple_providers.sh"` helper enables the Agent to directly read Kubernetes Secrets across both its own and other namespaces. Ensure that the associated ServiceAccount has the necessary permissions by assigning the appropriate Roles and RoleBindings, which can be set manually or using the following options:
    * `global.secretBackend.enableGlobalPermissions`: Determines if a ClusterRole is created that enables the Agents to read **all** Kubernetes Secrets.
      ```yaml
      apiVersion: datadoghq.com/v2alpha1
      kind: DatadogAgent
      metadata:
        name: datadog
      spec:
        global:
          secretBackend:
            command: "/readsecret_multiple_providers.sh"
            enableGlobalPermissions: true
      # ...
      ```
    * `global.secretBackend.roles`: Replaces `enableGlobalPermissions`, detailing the list of namespaces/secrets to which the Agents should have access.
      ```yaml
      apiVersion: datadoghq.com/v2alpha1
      kind: DatadogAgent
      metadata:
        name: datadog
      spec:
        global:
          secretBackend:
            command: "/readsecret_multiple_providers.sh"
            roles:
            - namespace: rabbitmq-system
              secrets:
              - "rabbitmqcluster-sample-default-user"
      # ...
      ```
      In this example, a Role is created granting read access to the secret `rabbitmqcluster-sample-default-user` in the `rabbitmq-system` namespace.

      **Note**: Each namespace in the list must be included in the DatadogAgent controller by setting `WATCH_NAMESPACE` or `DD_AGENT_WATCH_NAMESPACE` environment variables on the **Datadog Operator** container.


* For the Agent and Cluster Agent, others options exist to configure the secret backend command:
  * `global.secretBackend.args`: These arguments are supplied to the command when the Agent executes the secret backend command.
  * `global.secretBackend.timeout`: secret backend execution timeout in second. The default value is 30 seconds.
* For versions prior to Operator 1.11+, `spec.global.secretBackend` is unavailable. You should follow [these instructions][3] instead.

[1]: https://docs.datadoghq.com/agent/guide/secrets-management
[2]: https://docs.datadoghq.com/agent/guide/secrets-management/?tab=linux#script-for-reading-from-multiple-secret-providers
[3]: https://github.com/DataDog/datadog-operator/blob/2bbda7adace27de3d397b3d76d87fbd49fa304e3/docs/secret_management.md#how-to-deploy-the-agent-components-using-the-secret-backend-feature-with-datadogagent
