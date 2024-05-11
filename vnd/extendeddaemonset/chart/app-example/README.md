# APP Example

## Introduction

This chart allows you to deploy a "dummy" application that uses the ExtendedDaemonset.

Thanks to this chart you can test several ExtendedDaemonset features, such as:

* The Canary deployment strategy
* The `extendeddaemonset-check` util pod with the `helm test` command.
* The possibility to override a `Pod` resources for a specific `Node` thanks to an
  `ExtendedDaemonsetSettings` resource.

## Deployment

Before deploying this chart, you should have deployed the `ExtendedDaemonset` Controller with the following command: `helm install eds-controller ./chart/extendeddaemonset`.

Now you can deploy the `app-example` with default values this: `helm install foo ./chart/app-example`.

### Canary deployment

### Check if a canary deployment finished

Helm3 introduces the `helm test` command, which can be used to validate a "complex" deployment, was looking only
at the state if the pod is not enough. It is also useful when the chart contains CRDs because Helm is not aware
of how to understand the status of a CRD.

This chart contains a test "Pod" (manifests present in `app-example/templates/tests`) that can check if a
Extendedaemonset update is finished.

To simulate an update, the following command will update the docker image tag for the "foo" application:
`helm upgrade foo ./chart/app-example --set image.tag=stable`.

The `ExtendedDaemonset` status should have been moved to `canary`

```console
$ kubectl get eds
NAME              DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   IGNORED UNRESPONSIVE NODES   STATUS   REASON   ACTIVE RS               CANARY RS               AGE
foo-app-example   3         3         3       3            3                                        Canary            foo-app-example-db2sk   foo-app-example-76kz8   1h
```

With the default configuration, the Canary deployment is set to 3min. During this period only one pod has been updated.

The command `helm test foo` starts a Pod with a specific container that checks the `ExtendedDaemonset` status. The command returns when the canary deployment and the rolling-up are finished.
