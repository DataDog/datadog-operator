// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package datadog

import (
	"github.com/DataDog/datadog-operator/pkg/plugin/cmd/agent/agent"
	"github.com/DataDog/datadog-operator/pkg/plugin/cmd/clusteragent/clusteragent"
	"github.com/DataDog/datadog-operator/pkg/plugin/cmd/flare"
	"github.com/DataDog/datadog-operator/pkg/plugin/cmd/get"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// options provides information required by datadog command
type options struct {
	genericclioptions.IOStreams
	configFlags *genericclioptions.ConfigFlags
}

// newOptions provides an instance of options with default values
func newOptions(streams genericclioptions.IOStreams) *options {
	return &options{
		configFlags: genericclioptions.NewConfigFlags(false),
		IOStreams:   streams,
	}
}

// NewCmd provides a cobra command wrapping options
func NewCmd(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use: "datadog [subcommand] [flags]",
	}

	// Operator commands
	cmd.AddCommand(get.New(streams))
	cmd.AddCommand(flare.New(streams))

	// Agent commands
	cmd.AddCommand(agent.New(streams))

	// Cluster Agent commands
	cmd.AddCommand(clusteragent.New(streams))

	o := newOptions(streams)
	o.configFlags.AddFlags(cmd.Flags())

	return cmd
}
