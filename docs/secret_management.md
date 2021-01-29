# Secrets Management for the API and APP keys

Datadog Operator can be configured to retrieve the Datadog credentials using secrets for enhanced security. There are three methods you can choose from to set it up

## 1. Plain credentials in `DatadogAgent` resource

This is the simplest way to provide the credentials to the agents. This method is recommended for testing purposes only.

Add credentials to the Agent:

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

The credentials provided here will be stored in a Secret created by the Operator. By setting properly the `RBAC` on the `DatadogAgent` CRD, it is possible to limit who is able to see those credentials.

## 2. Use secret(s) references

Another solution is to provide the name of the secret(s) that store the credentials:

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

Create the secret before deploying the Datadog Agent, or the deployment fails.

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

The data keys used in the secret(s) are important. for the "API key" the key inside the secret should be `api_key`, and for the "APP key" it is `app_key`.

**Note:**

It is possible to use the same secret to store both credentials:

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

The Datatog Operator is compatible with the ["Secret backend" feature][1] implemented.

### How Deploy the Datadog-Operator with the secret backend activated

#### Custom secret backend

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
Also don't forget to use the "custom" Datadog Operator container image.

```console
$ helm [install|upgrade] dd-operator --set "secretBackend.command=/my-secret-backend.sh" --set "image.repository=datadog-operator-with-secret-be" ./chart/datadog-operator
success
```

#### Using the secret helper

Kubernetes supports exposing secrets as files inside a pod, we provide a helper script in the Datadog Operator image to read the secrets from files.

Install or the update of the Datadog Operator deployment, the value parameters `.Values.secretBackend.command` should be set to `/readsecret.sh` and `.Values.secretBackend.arguments` set to `/etc/secret-volume` if your secrets are mounted in `/etc/secret-volume`.

**Note:** This secret helper requires Datadog Operator v0.5.0+

### How deploy agents using the secret backend feature with DatadogAgent

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
      name: "gcr.io/datadoghq/agent:latest"
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
