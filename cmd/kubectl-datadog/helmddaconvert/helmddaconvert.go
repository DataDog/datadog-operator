// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helmddaconvert

import (
	"fmt"

	"github.com/DataDog/datadog-operator/pkg/plugin/common"
	"github.com/DataDog/helm-charts/tools/yaml-mapper/pkg/yamlmapper"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

var (
	mappingFile string
	sourceFile  string
	destFile    string
	prefixFile  string
	ddaName     string
	namespace   string
	updateMap   bool
	printPtr    bool
	examples    = `
# Map Datadog Helm values to DatadogAgent CRD schema
Convert a Datadog Helm chart values YAML file to a DatadogAgent custom resource YAML file. 

%[1]s --values-file=<datadog-values>.yaml

# Use custom mapping YAML file to map source YAML file. 
Mapping YAML should follow the YAML map format with key: value pairs where the 
key is the source key and value is the new key. Nested YAML keys should be period-delimited.

%[1]s --mapping-file=<custom-mapping-file> --values-file=<values-file> --dest-file=<dest-file>

Custom YAML mapping file example: 

// <custom-mapping-file>
source.key: new.key

// <values-file>
source: 
  key

// <dest-file>
new:
  key

# Update provided mapping YAML file with keys from source values YAML file.
Keys that are present in the source YAML file, but missing from the mapping YAML file, are 
then added to the mapping YAML file with a placeholder "" value.

%[1]s --mapping-file=<custom-mapping-file> --values-file=<values-file> --update-map

# Use custom Kubernetes apiVersion and kind for destination YAML file, by providing a 
prefix YAML file. 

%[1]s --mapping-file=<custom-mapping-file> --values-file=<values-file> --prefix-file=<prefix-file>

Example:

// <prefix-file>
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
`
)

// options provides information required by helmmapper command
type options struct {
	genericiooptions.IOStreams
	common.Options
}

// newOptions provides an instance of options with default values
func newOptions(streams genericiooptions.IOStreams) *options {
	o := &options{
		IOStreams: streams,
	}
	o.SetConfigFlags()

	return o
}

// New provides a cobra command wrapping options for "check" sub command
func New(streams genericiooptions.IOStreams) *cobra.Command {
	o := newOptions(streams)
	cmd := &cobra.Command{
		Use:          "helmddaconvert [flags]",
		Short:        "Map Datadog Helm values to DatadogAgent CRD schema",
		Example:      fmt.Sprintf(examples, "kubectl datadog helmddaconvert"),
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.complete(c, args); err != nil {
				return err
			}

			return o.run(c)
		},
	}

	cmd.Flags().StringVarP(&sourceFile, "values-file", "", "", "Path to source Helm values YAML file. [Required] Example: values.yaml")
	cmd.Flags().StringVarP(&mappingFile, "mapping-file", "", "", "Path to the YAML mapping file. Example: mapping.yaml")
	cmd.Flags().StringVarP(&destFile, "dest-file", "", "", "Path the the destination DDA YAML manifest file.")
	cmd.Flags().StringVarP(&prefixFile, "prefix-file", "", "", "Path to K8s prefix YAML file. The content in this file will be prepended to the output.")
	cmd.Flags().StringVarP(&ddaName, "dda-name", "", "", "Name to use for the destination DDA custom resource.")
	cmd.Flags().StringVarP(&namespace, "dda-namespace", "", "", "Namespace to use in the destination DDA custom resource.")
	cmd.Flags().BoolVarP(&updateMap, "update-map", "", false, "Update 'mappingFile' with provided 'sourceFile'. (default false) If set to 'true', default mappingFile is %s and default sourceFile is latest published Datadog chart values.yaml.")
	cmd.Flags().BoolVarP(&printPtr, "print-output", "", false, "Print mapper output to stdout.")

	o.ConfigFlags.AddFlags(cmd.Flags())

	return cmd
}

// complete sets all information required for processing the command
func (o *options) complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("this command does not accept args, use flags instead")
	}

	if sourceFile == "" {
		return fmt.Errorf("--values-file is required")
	}
	return o.Init(cmd)
}

// run runs the helmddaconvert command
func (o *options) run(cmd *cobra.Command) error {
	yamlmapper.MapYaml(mappingFile, sourceFile, destFile, prefixFile, ddaName, namespace, updateMap, printPtr)

	return nil
}
