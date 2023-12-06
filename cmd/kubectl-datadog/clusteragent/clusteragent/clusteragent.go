// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/clusteragent/leader"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/clusteragent/upgrade"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// options provides information required by clusteragent command
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

// New provides a cobra command wrapping options for "clusteragent" sub command
func New(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use: "clusteragent [subcommand] [flags]",
	}

	cmd.AddCommand(leader.New(streams))
	cmd.AddCommand(upgrade.New(streams))

	o := newOptions(streams)
	o.configFlags.AddFlags(cmd.Flags())

	return cmd
}
