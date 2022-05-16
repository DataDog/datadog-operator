// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package root

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/cmd/check-operator/upgrade"
)

// Options provides information required to manage Root.
type Options struct {
	configFlags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
}

// NewRootOptions provides an instance of Options with default values.
func NewRootOptions(streams genericclioptions.IOStreams) *Options {
	return &Options{
		configFlags: genericclioptions.NewConfigFlags(false),
		IOStreams:   streams,
	}
}

// NewCmdRoot provides a cobra command wrapping Options.
func NewCmdRoot(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewRootOptions(streams)
	cmd := &cobra.Command{
		Use: "check-operator [subcommand] [flags]",
	}

	o.configFlags.AddFlags(cmd.Flags())
	cmd.AddCommand(upgrade.NewCmdUpgrade(streams))

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
