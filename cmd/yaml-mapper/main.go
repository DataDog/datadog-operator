// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package main

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
