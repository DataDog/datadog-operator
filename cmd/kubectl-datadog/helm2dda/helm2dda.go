// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helm2dda

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/DataDog/datadog-operator/cmd/yaml-mapper/constants"
	"github.com/DataDog/datadog-operator/cmd/yaml-mapper/mapper"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

// options provides information required by helm2dda command
type options struct {
	genericiooptions.IOStreams
	common.Options

	args        []string
	mappingPath string
	sourcePath  string
	destPath    string
	ddaName     string
	namespace   string
	updateMap   bool
	printOutput bool
}

// newOptions provides an instance of options with default values
func newOptions(streams genericiooptions.IOStreams) *options {
	o := &options{
		IOStreams:   streams,
		updateMap:   false,
		printOutput: false,
	}
	o.SetConfigFlags()

	return o
}

// New provides a cobra command wrapping options for "check" sub command
func New(streams genericiooptions.IOStreams) *cobra.Command {
	o := newOptions(streams)

	usageExample := `
kubectl datadog helm2dda --sourcePath=example_source.yaml
`

	cmd := &cobra.Command{
		Use:          "helm2dda [DatadogAgent name] --sourcePath <path> [flags]",
		Short:        "Map Datadog Helm values to DatadogAgent CRD schema",
		Example:      usageExample,
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.complete(c, args); err != nil {
				return err
			}
			if err := o.validate(); err != nil {
				return err
			}

			return o.run()
		},
	}

	cmd.Flags().StringVarP(&o.sourcePath, "sourcePath", "f", "", "Path to source YAML file. Required. Example: source.yaml")
	cmd.Flags().StringVarP(&o.mappingPath, "mappingPath", "m", "", "Path to mapping YAML file.")
	cmd.Flags().StringVarP(&o.destPath, "destPath", "d", "", "Path to destination YAML file.")
	cmd.Flags().BoolVarP(&o.updateMap, "updateMap", "u", false, fmt.Sprintf("Update 'mappingPath' with provided 'sourcePath'. If set to 'true', default mappingPath is %s and default sourcePath is latest published Datadog chart values.yaml.", constants.DefaultDDAMappingPath))
	cmd.Flags().BoolVarP(&o.printOutput, "printOutput", "o", true, "print mapped DDA output to stdout")

	o.ConfigFlags.AddFlags(cmd.Flags())

	// Hide default k8s cli-runtime flags from usage
	toHide := []string{
		"as", "as-group", "as-uid", "cache-dir", "certificate-authority", "client-certificate",
		"client-key", "cluster", "context", "disable-compression", "insecure-skip-tls-verify",
		"kubeconfig", "request-timeout", "server", "tls-server-name", "token", "user",
	}
	for _, name := range toHide {
		_ = cmd.Flags().MarkHidden(name)
	}

	return cmd
}

// complete sets all information required for processing the map command.
func (o *options) complete(cmd *cobra.Command, args []string) error {
	o.args = args
	if len(args) == 1 {
		o.ddaName = args[0]
	}

	return o.Init(cmd)
}

// validate ensures that all required arguments and flag values are provided.
func (o *options) validate() error {
	if o.sourcePath == "" {
		return fmt.Errorf("`--sourcePath` flag is required")
	}

	if len(o.args) > 1 {
		return fmt.Errorf("received %v arguments. Only 1 argument allowed", len(o.args))
	}
	return nil
}

// Init initialize the command config
func (o *options) init(cmd *cobra.Command) error {
	var err error
	if o.mappingPath != "" {
		o.mappingPath, err = mapper.ResolveFilePath(o.mappingPath)
		if err != nil {
			return fmt.Errorf("could not resolve mapping path: %v: %w", o.mappingPath, err)
		}
	}
	if o.sourcePath != "" {
		o.sourcePath, err = mapper.ResolveFilePath(o.sourcePath)
		if err != nil {
			return fmt.Errorf("could not resolve source path: %v: %w", o.sourcePath, err)
		}
	}

	if o.destPath != "" {
		// Ignore the err since we will create the file later if it doesn't exist
		destPath, err := mapper.ResolveFilePath(o.destPath)
		if err == nil {
			o.destPath = destPath
		}
	}

	if o.namespace == "" {
		o.namespace, _ = cmd.Flags().GetString("namespace")
	}

	return nil
}

// run runs the helm2dda command
func (o *options) run() error {
	mapperConfig := mapper.MapConfig{
		MappingPath: o.mappingPath,
		SourcePath:  o.sourcePath,
		DestPath:    o.destPath,
		DDAName:     o.ddaName,
		Namespace:   o.namespace,
		UpdateMap:   o.updateMap,
		PrintOutput: o.printOutput,
	}
	newMapper := mapper.NewMapper(mapperConfig)
	err := newMapper.Run()
	if err != nil {
		return err
	}

	return nil
}
