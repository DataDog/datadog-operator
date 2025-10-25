// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package mapper

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

type Options struct {
	genericiooptions.IOStreams

	configFlags *genericclioptions.ConfigFlags
	args        []string
	mappingFile string
	sourceFile  string
	destFile    string
	prefixFile  string
	ddaName     string
	namespace   string
	updateMap   bool
	printPtr    bool
}

func NewOptions(streams genericiooptions.IOStreams) *Options {
	return &Options{
		configFlags: genericclioptions.NewConfigFlags(false),
		IOStreams:   streams,
		updateMap:   false,
		printPtr:    false,
	}
}

func NewCmdMap(streams genericiooptions.IOStreams) *cobra.Command {
	o := NewOptions(streams)

	usageExample := `
./yaml-mapper map datadog --sourceFile=example_source.yaml
`

	cmd := &cobra.Command{
		Use:          "map [DatadogAgent name] [flags]",
		Short:        "Map Datadog Helm to DDA",
		Example:      usageExample,
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(c, args); err != nil {
				return err
			}
			if err := o.Validate(); err != nil {
				return err
			}

			o.Run()

			return nil
		},
	}

	cmd.Flags().StringVarP(&o.mappingFile, "mappingFile", "m", "", "Path to mapping YAML file.")
	cmd.Flags().StringVarP(&o.sourceFile, "sourceFile", "f", "", "Path to source YAML file. Example: source.yaml")
	cmd.Flags().StringVarP(&o.destFile, "destFile", "d", "destination.yaml", "Path to destination YAML file.")
	cmd.Flags().StringVarP(&o.prefixFile, "prefixFile", "p", "", "Path to prefix YAML file. The content in this file will be prepended to the output.")
	cmd.Flags().BoolVarP(&o.updateMap, "updateMap", "u", false, fmt.Sprintf("Update 'mappingFile' with provided 'sourceFile'. If set to 'true', default mappingFile is %s and default sourceFile is latest published Datadog chart values.yaml.", defaultDDAMappingPath))
	cmd.Flags().BoolVarP(&o.printPtr, "printOutput", "o", true, "print mapped DDA output to stdout")
	o.configFlags.AddFlags(cmd.Flags())

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

// Complete sets all information required for processing the command.
func (o *Options) Complete(cmd *cobra.Command, args []string) error {
	o.args = args
	if len(args) > 0 {
		o.ddaName = args[0]
	}

	return o.Init(cmd)
}

// Validate ensures that all required arguments and flag values are provided.
func (o *Options) Validate() error {
	if o.ddaName == "" {
		return fmt.Errorf("the DatadogAgent name is required")
	}

	return nil
}

// Init initialize the command config
func (o *Options) Init(cmd *cobra.Command) error {
	o.mappingFile, _ = resolveFilePath(o.mappingFile)
	o.sourceFile, _ = resolveFilePath(o.sourceFile)
	o.destFile, _ = resolveFilePath(o.destFile)

	if o.namespace == "" {
		o.namespace, _ = cmd.Flags().GetString("namespace")
	}

	return nil
}

func (o *Options) Run() {
	mapperConfig := Config{
		MappingPath: o.mappingFile,
		SourcePath:  o.sourceFile,
		DestPath:    o.destFile,
		DDAName:     o.ddaName,
		Namespace:   o.namespace,
		UpdateMap:   o.updateMap,
		PrintOutput: o.printPtr,
		PrefixPath:  o.prefixFile,
	}
	newMapper := NewMapper(mapperConfig)
	err := newMapper.Run()
	if err != nil {
		log.Print(err)
	}
}
