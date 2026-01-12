// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package mapper

import (
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/DataDog/datadog-operator/cmd/yaml-mapper/constants"
)

func init() {
	// initialize logging handler
	slog.SetLogLoggerLevel(slog.LevelInfo)
}

// Options provides information required to manage the map command.
type Options struct {
	genericiooptions.IOStreams

	ConfigFlags *genericclioptions.ConfigFlags
	Args        []string
	MappingPath string
	SourcePath  string
	DestPath    string
	HeaderPath  string
	DdaName     string
	Namespace   string
	UpdateMap   bool
	PrintOutput bool
}

// NewOptions provides an instance of Options with default values.
func NewOptions(streams genericiooptions.IOStreams) *Options {
	return &Options{
		ConfigFlags: genericclioptions.NewConfigFlags(false),
		IOStreams:   streams,
		UpdateMap:   false,
		PrintOutput: false,
	}
}

// NewCmdMap provides a cobra command wrapping Options.
func NewCmdMap(streams genericiooptions.IOStreams) *cobra.Command {
	o := NewOptions(streams)

	usageExample := `
./yaml-mapper map datadog --sourcePath=example_source.yaml
`

	cmd := &cobra.Command{
		Use:          "map [DatadogAgent name] --sourcePath <path> [flags]",
		Short:        "Map Datadog Helm values to DatadogAgent CRD",
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

	cmd.Flags().StringVarP(&o.SourcePath, "sourcePath", "f", "", "Path to source YAML file. Required. Example: source.yaml")
	cmd.Flags().StringVarP(&o.MappingPath, "mappingPath", "m", "", "Path to mapping YAML file.")
	cmd.Flags().StringVarP(&o.DestPath, "destPath", "d", "", "Path to destination YAML file.")
	cmd.Flags().StringVarP(&o.DdaName, "ddaName", "", "", "DatadogAgent custom resource name.")
	cmd.Flags().StringVarP(&o.HeaderPath, "headerPath", "p", "", "Path to header YAML file. The content in this file will be prepended to the output.")
	cmd.Flags().BoolVarP(&o.UpdateMap, "updateMap", "u", false, fmt.Sprintf("Update 'mappingPath' with provided 'sourcePath'. If set to 'true', default mappingPath is %s and default sourcePath is latest published Datadog chart values.yaml.", constants.DefaultDDAMappingPath))
	cmd.Flags().BoolVarP(&o.PrintOutput, "printOutput", "o", true, "print mapped DDA output to stdout")
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

// Complete sets all information required for processing the map command.
func (o *Options) Complete(cmd *cobra.Command, args []string) error {
	o.Args = args
	if len(args) == 1 {
		o.DdaName = args[0]
	}

	return o.Init(cmd)
}

// Validate ensures that all required arguments and flag values are provided.
func (o *Options) Validate() error {
	if o.SourcePath == "" && !o.UpdateMap {
		return fmt.Errorf("`--sourcePath` flag is required")
	}

	if len(o.Args) > 1 {
		return fmt.Errorf("received %v arguments. Only 1 argument allowed", len(o.Args))
	}
	return nil
}

// Init initialize the command config
func (o *Options) Init(cmd *cobra.Command) error {
	var err error
	if o.MappingPath != "" {
		o.MappingPath, err = ResolveFilePath(o.MappingPath)
		if err != nil {
			return fmt.Errorf("could not resolve mapping path: %v: %w", o.MappingPath, err)
		}
	}
	if o.SourcePath != "" {
		o.SourcePath, err = ResolveFilePath(o.SourcePath)
		if err != nil {
			return fmt.Errorf("could not resolve source path: %v: %w", o.SourcePath, err)
		}
	}

	if o.DestPath != "" {
		// Ignore the err since we will create the file later if it doesn't exist
		destPath, err := ResolveFilePath(o.DestPath)
		if err == nil {
			o.DestPath = destPath
		}
	}

	if o.Namespace == "" {
		o.Namespace, _ = cmd.Flags().GetString("namespace")
	}

	return nil
}

// Run is used to run the map command.
func (o *Options) Run() {
	mapperConfig := MapConfig{
		MappingPath: o.MappingPath,
		SourcePath:  o.SourcePath,
		DestPath:    o.DestPath,
		DDAName:     o.DdaName,
		Namespace:   o.Namespace,
		UpdateMap:   o.UpdateMap,
		PrintOutput: o.PrintOutput,
		HeaderPath:  o.HeaderPath,
	}
	newMapper := NewMapper(mapperConfig)
	err := newMapper.Run()
	if err != nil {
		slog.Error("mapper run failed", "error", err)
		os.Exit(1)
	}
	os.Exit(0)
}

// ResolveFilePath validates and returns absolute filepath.
func ResolveFilePath(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	// Expand tilde
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot resolve home dir: %w", err)
		}
		p = path.Join(home, strings.TrimPrefix(p, "~"))
	}

	// Make absolute
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("cannot resolve absolute path for %q: %w", p, err)
	}

	// Verify existence
	if _, err := os.Stat(abs); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("path does not exist: %s", abs)
		}
		return "", err
	}

	return filepath.Clean(abs), nil
}
