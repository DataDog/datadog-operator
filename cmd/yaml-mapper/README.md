# README.md

## Purpose

The purpose of this tool is to map a YAML file of a certain structure to another YAML file of a different structure. For instance, migrating a Helm chart `values.yaml` file to another `values.yaml` file after a significant chart update.

## Motivation

The motivation for creating this tool is to provide a way to support Datadog users who want to switch from using the `datadog` Helm chart to using the Datadog Operator controller when deploying the Datadog Agent. It is a significant change that requires creating a new `DatadogAgent` custom resource specification. As a result, we are providing a way to map from a Helm chart `values.yaml` file to a `DatadogAgent` CRD spec, using a provided `mapping.yaml` file.

## How to install

```bash
make yaml-mapper
```

> [!TIP]
> Add the `/bin` directory to your `PATH` to easily access the `yaml-mapper` executable.

## How to use

### Mapping Helm YAML to DatadogAgent CRD Spec

This mapper converts a `datadog` Helm chart values yaml file to the `DatadogAgent` CRD spec.

The resulting file is written to `dda.yaml.<timestamp>`. To specify a destination file, use flag `--destPath=[<FILENAME>.yaml]`.

Content from a file can be optionally prepended to the output. To specify the header file, use flag `--headerPath=[<FILENAME>.yaml]`.

By default, the output is also printed to STDOUT; to disable this use the flag `--printOutput=false`.

## Example usage

```bash
yaml-mapper --sourcePath=<EXAMPLE_SOURCE>.yaml --mappingPath=mapper/mapping_datadog_helm_to_datadogagent_crd.yaml --headerPath=<EXAMPLE_HEADER>.yaml
```

The following command writes the mapped DDA to the provided `destination.yaml` file. 
```bash
yaml-mapper --sourcePath=examples/example_source.yaml --mappingPath=mapper/mapping_datadog_helm_to_datadogagent_crd.yaml --headerPath=examples/example_header.yaml --destPath=examples/destination.yaml
```

### Update Mapping File from a Source YAML

*When updating the mapping file, please be sure to add the [corresponding key!](#updating-mapping-keys)*

Below are different ways to update the mapping file based on your source:

1. **Local `values.yaml` from your branch**

    If you have run into a CI error when adding a new field to `values.yaml`, run this command:
    ```bash
    yaml-mapper --updateMap --sourcePath=<PATH_TO>helm-charts/charts/datadog/values.yaml
    ```
2. **Latest published Datadog Helm chart values**
   
    This pulls the latest `values.yaml` from the [latest published Helm chart](https://github.com/DataDog/helm-charts/releases/latest) and updates the default mapping file.
    ``` bash
    yaml-mapper --updateMap
    ```
3. **Update a custom mapping file with a custom source YAML**
    ```bash
    yaml-mapper --updateMap --sourcePath=<YOUR_SOURCE_FILE> --mappingPath=<YOUR_MAPPING_FILE>
    ```

### Update Mapping Keys

Currently, this process is manual. To update a mapping key, search for it in the [operator configuration](https://github.com/DataDog/datadog-operator/blob/main/docs/configuration.v2alpha1.md).  When adding the corresponding operator value, be sure to prepend it with `spec.`.

If the key does not have a corresponding value in the Datadog Operator configuration, leave the mapping as is with an empty string.

Thank you for helping us keep the mapping accurate and up to date!

## Use a custom mapping

This tooling can be used to perform custom YAML mapping. Supply your own YAML mapping file using the following format:

```
source.key: destination.key
```

Both the key and value are period-delimited instead of nested or indented, as in a typical YAML file.

Optionally, define a headerPath to prepend to the mapped output. 

```
apiVersion: v1
kind: MyCustomResource
```

Pass the source file and mapping file to the command:

```bash
yaml-mapper --sourcePath=source.yaml --mappingPath=mapping.yaml [--headerPath=<your-header-file]
```