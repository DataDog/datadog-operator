// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package main

//var (
//	mappingFile string
//	sourceFile  string
//	destFile    string
//	prefixFile  string
//	ddaName     string
//	namespace   string
//	updateMap   bool
//	printPtr    bool
//)

//const defaultDDAMappingPath = "mapping_datadog_helm_to_datadogagent_crd_v2.yaml"
//
//func main() {
//	flags := pflag.NewFlagSet("yaml-mapper", pflag.ExitOnError)
//	pflag.CommandLine = flags
//
//	yamlMapper := mapper.MapYaml
//	flag.Usage = func() {
//		fmt.Fprintf(os.Stderr, `yaml-mapper: migrate Datadog Helm values to the DatadogAgent CRD
//Usage:
//	helm-mapper -sourceFile=<FILE> -destFile=<DEST_FILE> -mappingFile=<MAPPING_FILE>
//
//Options:
//`)
//		flag.PrintDefaults()
//	}
//
//	flag.StringVar(&mappingFile, "mappingFile", defaultDDAMappingPath, "Path to mapping YAML file. Example: mapping.yaml")
//	flag.StringVar(&sourceFile, "sourceFile", "", "Path to source YAML file. Example: source.yaml")
//	flag.StringVar(&destFile, "destFile", "destination.yaml", "Path to destination YAML file.")
//	flag.StringVar(&prefixFile, "prefixFile", "", "Path to prefix YAML file. The content in this file will be prepended to the output.")
//	flag.StringVar(&ddaName, "ddaName", "", "Name to use for the destination DDA manifest.")
//	flag.StringVar(&namespace, "namespace", "", "Namespace to use in destination DDA manifest.")
//	flag.BoolVar(&updateMap, "updateMap", false, fmt.Sprintf("Update 'mappingFile' with provided 'sourceFile'. (default false) If set to 'true', default mappingFile is %s and default sourceFile is latest published Datadog chart values.yaml.", defaultDDAMappingPath))
//	flag.BoolVar(&printPtr, "printOutput", true, "print output to stdout")
//
//	flag.Parse()
//
//	mapper.MapYaml(mappingFile, sourceFile, destFile, prefixFile, ddaName, namespace, updateMap, printPtr)
//}

import (
	"os"

	"github.com/DataDog/datadog-operator/cmd/yaml-mapper/mapper"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

type Options struct {
	configFlags *genericclioptions.ConfigFlags
	genericiooptions.IOStreams
}

func NewMapperOptions(streams genericiooptions.IOStreams) *Options {
	return &Options{
		configFlags: genericclioptions.NewConfigFlags(false),
		IOStreams:   streams,
	}
}

func NewMapper(streams genericiooptions.IOStreams) *cobra.Command {
	o := NewMapperOptions(streams)
	cmd := &cobra.Command{
		Use: "yaml-mapper [command] [flags]",
	}

	o.configFlags.AddFlags(cmd.Flags())
	cmd.AddCommand(mapper.NewCmdMap(streams))

	// Hide default k8s cli-runtime flags from usage
	toHide := []string{
		"as", "as-group", "as-uid", "cache-dir", "certificate-authority", "client-certificate",
		"client-key", "cluster", "context", "disable-compression", "insecure-skip-tls-verify",
		"kubeconfig", "namespace", "request-timeout", "server", "tls-server-name", "token", "user",
	}
	for _, name := range toHide {
		_ = cmd.Flags().MarkHidden(name)
	}

	return cmd
}

// Complete sets all information required for processing the command.
func (o *Options) Complete(cmd *cobra.Command, args []string) error {
	return nil
}

// Validate ensures that all required arguments and flag values are provided.
func (o *Options) Validate() error {
	return nil
}

// Run runs the command.
func (o *Options) Run() error {
	return nil
}

func main() {
	flags := pflag.NewFlagSet("yaml-mapper", pflag.ExitOnError)
	pflag.CommandLine = flags

	newMapper := NewMapper(genericiooptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
	if err := newMapper.Execute(); err != nil {
		os.Exit(1)
	}
}
