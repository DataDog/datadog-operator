# Secrets Management for the API and APP keys

The Datadog-Operator is able to retrieves the datadog credentials securely thanks to 3 different methods.

## 1. Plain credentials in DatadogAgent resource

this is the simplest way to provide the credentials that will be used by the agents.

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  credentials:
    apiKey: "<api-key>"
    appKey: "<app-key>"
  # ...
```

The credentials provided here will be stored in a Secret created by the Operator. By setting properly the `RBAC` on the `DatadogAgent` CRD. It is possible to limit who is able to see those credentials.
But still, it is not the best solution in terms of security. This solution is good for testing purposes.

## 2. Use secret(s) references

Another solution is to provide the name of the secret(s) that store the credentials. like this the `DatadogAgent` resource doesn't contain any credentials as plain text.

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  credentials:
    apiKeyExistingSecret: "my-api-key-secret"
    appKeyExistingSecret: "my-app-key-secret"
  # ...
```

In this case, the secret(s) should exist before trying to create the `DatadogAgent`, else the deployment will failed.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-api-key-secret
data:
  api_key: <api-key>

---
apiVersion: v1
kind: Secret
metadata:
  name: my-app-key-secret
data:
  app_key: <app-key>
```

The keys used in the secret(s) are important. for the "API key" the key inside the secret should be `api_key`, and for the "APP key" it is `app_key`.

**Remarks:**

* It is possible to use the same secret to store both credentials

    ```yaml
    ---
    apiVersion: v1
    kind: Secret
    metadata:
      name: my-credentials
    data:
      api_key: <api-key>
      app_key: <app-key>
    ```

## 3. Use the Secret backend feature

The Datatog Operator is compatible with the ["Secret backend" feature][1] implemented initialy in the datadog agent.

### How Deploy the Datadog-Operator with the secret backend activated

The first step is to create a container image from the `datadog/operator` that contains the secret backend command.

The following Dockerfile example takes the `datadog/operator:latest` image as the base image and copies the `my-secret-backend.sh` script file.

```Dockerfile
FROM datadog/operator:latest
COPY ./my-secret-backend.sh /my-secret-backend.sh
RUN chmod 755 /my-secret-backend.sh
```

```console
$ docker build -t datadog-operator-with-secret-be:latest .
success
```

Then, during the installation or the update of the "Datadog Operator" deployment the value parameter: `.Values.secretBackend.command` should be set with the secret backend command path (inside the container).
Also don't for get use the "custom" Datadog Operator container image.

```console
$ helm [install|upgrade] dd-operator --set "secretBackend.command=/my-secret-backend.sh" --set "image.repository=datadog-operator-with-secret-be" ./chart/datadog-operator
success
```

### How deploy agents using the secret backend feature with DatadogDeployment

To activate the secret backend feature in the `DatadogAgent` configuration, the `spec.credentials.useSecretBackend` parameter should be set to `true`.

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  credentials:
    apiKey: ENC[<api-key-secret-id>]
    appKey: ENC[<app-key-secret-id>]
    useSecretBackend: true
  # ...
```

Then inside the `spec.agent` configuration part, the secret backend command can be specified by adding a new environment variable: "DD_SECRET_BACKEND_COMMAND".

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  # ..
  agent:
    image:
      name: "datadog/agent:latest"
    config:
      env:
      - name: DD_SECRET_BACKEND_COMMAND
        value: "/my-secret-backend.sh"
```

If the "Cluster Agent" and the "Cluster Check Runner" are also deployed, the environment variable needs to be added also in the other environment variables configuration.

```yaml
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  # ...
  clusterAgent:
    # ...
    config:
      env:
      - name: DD_SECRET_BACKEND_COMMAND
        value: "/my-secret-backend.sh"
  clusterChecksRunner:
    # ...
    config:
      env:
      - name: DD_SECRET_BACKEND_COMMAND
        value: "/my-secret-backend.sh"
```

**Remarks:**

* Like for the "Datadog Operator", the "Agent" and "Cluster Agent" container images need to contain the "secret backend" command.
* For the "Agent" and "Cluster Agent", others options exist to configure secret backend command:

  * **DD_SECRET_BACKEND_ARGUMENTS**: those arguments will be specified to the command when the agent executes the secret backend command.
  * **DD_SECRET_BACKEND_OUTPUT_MAX_SIZE**: maximum output size of the secret backend command. The default value is 1048576 (1Mb).
  * **DD_SECRET_BACKEND_TIMEOUT**: secret backend execution timeout in second. The default value is 5 seconds.

[1]: https://docs.datadoghq.com/agent/guide/secrets-management
