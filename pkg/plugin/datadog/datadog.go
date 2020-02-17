// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package datadog

import (
	"github.com/DataDog/datadog-operator/pkg/plugin/flare"
	"github.com/DataDog/datadog-operator/pkg/plugin/get"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// datadogOptions provides information required by Datadog command
type datadogOptions struct {
	genericclioptions.IOStreams
	configFlags *genericclioptions.ConfigFlags
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

	cmd.AddCommand(get.NewCmdGet(streams))
	cmd.AddCommand(flare.NewCmdFlare(streams))

	o := newDatadogOptions(streams)
	o.configFlags.AddFlags(cmd.Flags())

	return cmd
}
