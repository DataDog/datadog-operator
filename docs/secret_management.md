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

### How to deploy the agent components using the secret backend feature with DatadogAgent

If using a custom script, create a Datadog Agent (or Cluster Agent) image following the example for the Datadog Operator above. Then, to activate the secret backend feature in the `DatadogAgent` configuration, the `spec.credentials.useSecretBackend` parameter should be set to `true`.

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

Then inside the `spec.agent` configuration, the secret backend command can be specified by adding a new environment variable: "DD_SECRET_BACKEND_COMMAND".

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  # ...
  override:
    nodeAgent:
      containers:
        agent:
          env:
            - name: DD_SECRET_BACKEND_COMMAND
              value: "/my-secret-backend.sh"
```

If the "Cluster Agent" and the "Cluster Check Runner" are also deployed, the environment variable needs to be added also in the other environment variables configuration.

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  # ...
  override:
    clusterAgent:
      # ...
      containers:
        cluster-agent:
          env:
            - name: DD_SECRET_BACKEND_COMMAND
              value: "/my-secret-backend.sh"
    clusterChecksRunner:
      # ...
      containers:
        agent:
          env:
            - name: DD_SECRET_BACKEND_COMMAND
              value: "/my-secret-backend.sh"
```

As in the Datadog Operator, the Datadog Agent image includes a helper function `readsecret.sh` that can be used to read mounted secrets. After creating the secret and setting the volume mount (in any container that requires it), set the `DD_SECRET_BACKEND_COMMAND` and `DD_SECRET_BACKEND_ARGUMENTS` environmental variables.

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
    credentials:
      apiKey: ENC[api_key]
      appKey: ENC[app_key]
  override:
    nodeAgent:
      env:
        - name: DD_SECRET_BACKEND_COMMAND
          value: "/readsecret.sh"
        - name: DD_SECRET_BACKEND_ARGUMENTS
          value: "/etc/secret-volume"
      containers:
        agent:
          volumeMounts:
            - name: secret-volume
              mountPath: "/etc/secret-volume"
      volumes:
        - name: secret-volume
          secret:
            secretName: test-secret
    clusterAgent:
      containers:
        cluster-agent:
          env:
            - name: DD_SECRET_BACKEND_COMMAND
              value: "/readsecret.sh"
            - name: DD_SECRET_BACKEND_ARGUMENTS
              value: "/etc/secret-volume"
          volumeMounts:
            - name: secret-volume
              mountPath: "/etc/secret-volume"
      volumes:
        - name: secret-volume
          secret:
            secretName: test-secret
```

The Datadog Agent also includes a script that can be used to read secrets from files mounted from Kubernetes secrets, or directly from Kubernetes secrets. This script can be used by setting `DD_SECRET_BACKEND_COMMAND` to `/readsecret_multiple_providers.sh`. An example of how to configure the DatadogAgent spec is provided below. For more details, see [Secrets Management][2].

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
spec:
  global:
    credentials:
      apiKey: ENC[k8s_secret@default/test-secret/api_key]
      appKey: ENC[k8s_secret@default/test-secret/app_key]
  override:
    nodeAgent:
      env:
        - name: DD_SECRET_BACKEND_COMMAND
          value: "/readsecret_multiple_providers.sh"
```

**Remarks:**

* For the "Agent" and "Cluster Agent", others options exist to configure secret backend command:

  * **DD_SECRET_BACKEND_ARGUMENTS**: those arguments will be specified to the command when the agent executes the secret backend command.
  * **DD_SECRET_BACKEND_OUTPUT_MAX_SIZE**: maximum output size of the secret backend command. The default value is 1048576 (1Mb).
  * **DD_SECRET_BACKEND_TIMEOUT**: secret backend execution timeout in second. The default value is 5 seconds.

[1]: https://docs.datadoghq.com/agent/guide/secrets-management
[2]: https://docs.datadoghq.com/agent/guide/secrets-management/?tab=linux#script-for-reading-from-multiple-secret-providers
