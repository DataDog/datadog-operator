# Datadog Dashboards
This feature is in beta.

## Overview
The Datadog Dashboards CRD allows users to create dashboards using the operator and manage them as Kubernetes resources. 

All configurable Dashboard fields are available in the CRD: `Description`, `LayoutType`, `NotifyList`, `ReflowType`, `Tags`, `TemplateVariables`, `TemplateVariablePresets`, `Title`, and `Widgets`. For more information, see the [Dashboard](https://docs.datadoghq.com/dashboards/) documentation.

There are two main ways to define the Spec of a Datadog Dashboard. The first way is by specifying each field in a yaml file like so:
```
apiVersion: datadoghq.com/v1alpha1
kind: DatadogDashboard
metadata:
  name: example-dashboard
  namespace: system 
spec:
  title: metricsTest
  layoutType: ordered

  widgets:
    - timeseries:
        title: "itsTime"
        titleAlign: center
        titleSize: "250"
        requests: 
          - queries:
              - metricQuery: 
                  query: avg:system.cpu.user{*}
                  name: metrics
                  dataSource: metrics
                  aggregator: avg
            responseFormat: timeseries
            style:
              lineType: dotted
              lineWidth: thin
            onRightYaxis: false
      id: 124
```
However, because adding widgets is a very big pull, only two widget types (`Timeseries` and `QueryValue`) are supported this way.

The second way is by copying the JSON of a dashboard in app, escaping it and just putting that into the `Data` field. Since this string isn't as easy to configure, there
are a fields that if specified override the same values in `Data`: `Title` `NotifyList` and `TemplateVariables`. Here is an example:
```
apiVersion: datadoghq.com/v1alpha1
kind: DatadogDashboard
metadata:
  name: example-dashboard
  namespace: system
spec:
  title: test
  layoutType: ordered
  tags:
    - "team:tagtest"
  templateVariables:
    - availableValues: 
        - host1
        - host2
        - host3
      name: first
      prefix: bar-foo
  notifyList:
    - foobar@datadoghq.com
  data:  "{\"title\":\"title\",\"description\":null,\"widgets\":[{\"id\":1234,\"definition\":{\"title\":\"\",\"type\":\"query_value\",\"requests\":[{\"queries\":[{\"compute\":{\"aggregation\":\"count\"},\"data_source\":\"logs\",\"indexes\":[\"*\"],\"name\":\"query1\",\"search\":{\"query\":\"\"},\"storage\":\"hot\"}],\"response_format\":\"scalar\"}],\"autoscale\":true,\"precision\":2,\"timeseries_background\":{\"type\":\"area\"}}}],\"template_variables\":[{\"name\":\"first\",\"prefix\":\"bar-foo\",\"available_values\":[\"host1\",\"host2\",\"host3\"],\"default\":\"*\"}],\"layout_type\":\"ordered\",\"notify_list\":[],\"template_variable_presets\":[{\"name\":\"saved_view1\",\"template_variables\":[{\"name\":\"foo-bar\",\"values\":[\"foo\",\"bar\"]}]}],\"reflow_type\":\"auto\",\"tags\":[\"team:tagtest\"]}"
```
Here, the created dashboard's title would be "test" instead of "title" (which is what is in the `Data` field).


## Prerequisites
* Operator v1.9.0+
* Tests were performed on Kubernetes versions v>=`1.30.0`

## Enabling DatadogDashboards

DAP is disabled by default. To enable DAP using the [datadog-operator helm chart](https://github.com/DataDog/helm-charts/tree/main/charts/datadog-operator), set `datadogDashboard.enabled=true` in your `values.yaml` file or as a flag in the command line arguments `--set datadogDashboard.enabled=true`.








