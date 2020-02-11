// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package plugin

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// datadogOptions provides information required by Datadog command
type datadogOptions struct {
	configFlags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
}

// newDatadogOptions provides an instance of DatadogOptions with default values
func newDatadogOptions(streams genericclioptions.IOStreams) *datadogOptions {
	return &datadogOptions{
		configFlags: genericclioptions.NewConfigFlags(false),
		IOStreams:   streams,
	}
}

// NewDatadogCmd provides a cobra command wrapping DatadogOptions
func NewDatadogCmd(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use: "datadog [subcommand] [flags]",
	}

	cmd.AddCommand(newCmdGet(streams))
	cmd.AddCommand(newCmdFlare(streams))

	o := newDatadogOptions(streams)
	o.configFlags.AddFlags(cmd.Flags())

	return cmd
}
