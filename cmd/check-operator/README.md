# check-operator

`check-operator` is a CLI to run checks against the operator.
The main use case is to run it as a [Helm chart test](https://helm.sh/docs/topics/chart_tests/) to validate the rolling update of the Agent, based on the DatadogAgent custom resource status.
