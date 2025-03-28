# Secrets Management

For enhanced security, the Datadog Operator can retrieve Datadog credentials (API key and application key) using [Secrets][4]. 

## Setting up Secrets

Choose one of the following methods to set up Secrets:

### Configure plain credentials in the DatadogAgent resource

**This method is recommended for testing purposes only.**

Add your API and application keys to the `DatadogAgent` spec:

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

The credentials provided here are stored in a Secret created by the Operator. By properly setting the RBAC on the `DatadogAgent` CRD, you can limit who is able to see those credentials.

### Use Secret references

1. Create your Secrets:

   ```yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: datadog-api-secret
   data:
     api_key: <DATADOG_API_KEY>

   ---
   apiVersion: v1
   kind: Secret
   metadata:
     name: datadog-app-secret
   data:
     app_key: <DATADOG_APP_KEY>
   ```

2. Provide the names of these Secrets in your `DatadogAgent` resource:

   ```yaml
   apiVersion: datadoghq.com/v2alpha1
   kind: DatadogAgent
   metadata:
     name: datadog
   spec:
     global:
       credentials:
         apiSecret:
           secretName: datadog-api-secret
           keyName: api-key
         appSecret:
           secretName: datadog-app-secret
           keyName: app-key
     # ...
   ```



**Note**: You can also use the same Secret to store both credentials:

```yaml
---
apiVersion: v1
kind: Secret
metadata:
  name: datadog-secret
data:
  api_key: <DATADOG_API_KEY>
  app_key: <DATADOG_APP_KEY>
```

Then, in your `DatadogAgent` resource:

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
## Use the secret backend

The Datatog Operator is compatible with the [secret backend][1].

### Deploy the Datadog Operator with the secret backend

1. Create a Datadog Operator container image that contains the secret backend command.

   If you'd like to build your own, the following Dockerfile example takes the `latest` image as the base image and copies the `my-secret-backend.sh` script file:

   ```Dockerfile
   FROM gcr.io/datadoghq/operator:latest
   COPY ./my-secret-backend.sh /my-secret-backend.sh
   RUN chmod 755 /my-secret-backend.sh
   ```

   Then, run:

   ```shell
   docker build -t datadog-operator-with-secret-backend:latest .
   ```

2. Install or update the Datadog Operator deployment with the `.Values.secretBackend.command` parameter set to the secret backend command path (inside the container). If you are using a custom image, update the image.

   ```shell
   $ helm [install|upgrade] dd-operator --set "secretBackend.command=/my-secret-backend.sh" --set "image.repository=datadog-operator-with-secret-backend" ./chart/datadog-operator
   ```

### Using the secret helper

**Note**: Requires Datadog Operator v0.5.0+.

Kubernetes supports exposing Secrets as files inside a pod. Datadog provides a helper script in the Datadog Operator image to read the Secrets from files.

1. Mount the Secret in the Operator container. For example, you can mount it at `/etc/secret-volume`. 

2. Install or update the Datadog Operator deployment with the `.Values.secretBackend.command` parameter set to `/readsecret.sh` and the `.Values.secretBackend.arguments` parameter set to `/etc/secret-volume`:

   ```shell
   helm [install|upgrade] dd-operator --set "secretBackend.command=/readsecret.sh" --set "secretBackend.arguments=/etc/secret-volume" ./chart/datadog-operator
   ```

### Deploying Agent components using the secret backend feature with the DatadogAgent 

**Note**: Requires Datadog Operator v1.11+.

#### With a custom script

If you are using a custom script, create a Datadog Agent (or Cluster Agent) image and specify credentials using `ENC[<placeholder>]`, and specify the secret backend command in `spec.global.secretBackend.command`:

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
       secretBackend:
         command: "/my-secret-backend.sh"
     # ...
   ```

The environment variable `DD_SECRET_BACKEND_COMMAND` from this configuration is automatically applied to all the deployed components: node Agent, Cluster Agent, and Cluster Checks Runners. Ensure the image you are using for all the components includes your command.

#### With the helper function

For convenience, the Datadog Agent and its sibling Cluster Agent images include a `readsecret_multiple_providers.sh` [helper function][2] that can be used to read from both files as well as Kubernetes Secrets. After you create the Secret, set `spec.global.secretBackend.command` to `"/readsecret_multiple_providers.sh"`.

For instance, to use the secret backend for the Agent and Cluster Agent, create a Secret called "test-secret":

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

## Additional notes

### ServiceAccount permissions

The `"/readsecret_multiple_providers.sh"` helper enables the Agent to directly read Kubernetes Secrets across both its own and other namespaces. Ensure that the associated ServiceAccount has the necessary permissions by assigning the appropriate Roles and RoleBindings. You can set these manually, or by using the following options:

- `global.secretBackend.enableGlobalPermissions`: Determines if a ClusterRole is created that enables the Agents to read **all** Kubernetes Secrets.

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

- `global.secretBackend.roles`: Replaces `enableGlobalPermissions`, detailing the list of namespace/secrets to which the Agents should have access.

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

   In this example, a Role is created granting read access to the Secret `rabbitmqcluster-sample-default-user` in the `rabbitmq-system` namespace.

   **Note**: Each namespace in the `roles` list must also be configured in the `WATCH_NAMESPACE` or `DD_AGENT_WATCH_NAMESPACE` environment variable on the Datadog Operator deployment.

### Secret backend configuration options

For the Agent and Cluster Agent, there are other configuration options for the secret backend command:
  * `global.secretBackend.args`: These arguments are supplied to the command when the Agent executes the secret backend command.
  * `global.secretBackend.timeout`: Secret backend execution timeout in seconds. The default value is 30 seconds.

For versions prior to Operator 1.11, `spec.global.secretBackend` is unavailable. You should follow [these instructions][3] instead.

[1]: https://docs.datadoghq.com/agent/guide/secrets-management
[2]: https://docs.datadoghq.com/agent/guide/secrets-management/?tab=linux#script-for-reading-from-multiple-secret-providers
[3]: https://github.com/DataDog/datadog-operator/blob/2bbda7adace27de3d397b3d76d87fbd49fa304e3/docs/secret_management.md#how-to-deploy-the-agent-components-using-the-secret-backend-feature-with-datadogagent
[4]: https://kubernetes.io/docs/concepts/configuration/secret/