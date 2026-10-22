# Release notes

## Unreleased

### Upgrade notes

- Operator profiles now use the same service name as APM traces: `DD_SERVICE`, defaulting to `datadog-operator`. If profiling is enabled and `DD_SERVICE` is unset, new profiles appear under `datadog-operator` instead of the executable name (`manager` in the shipped image), even when tracing is disabled. Historical profiles remain under the previous service name; update saved profile queries accordingly. Set `DD_SERVICE=manager` to retain the previous profiling service name for the shipped image; this also sets the service name for traces. Existing nonempty `DD_SERVICE` values are unchanged.
