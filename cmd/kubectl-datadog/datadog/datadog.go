// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadog

import (
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/helm2dda"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/agent/agent"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/clusteragent/clusteragent"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/flare"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/get"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/metrics"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/validate/validate"
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

// NewCmd provides a cobra command wrapping options for "datadog" command
func NewCmd(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use: "datadog [subcommand] [flags]",
	}

	// Operator commands
	cmd.AddCommand(get.New(streams))
	cmd.AddCommand(flare.New(streams))
	cmd.AddCommand(validate.New(streams))

	// Agent commands
	cmd.AddCommand(agent.New(streams))

	// Cluster Agent commands
	cmd.AddCommand(clusteragent.New(streams))

	// DatadogMetric commands
	cmd.AddCommand(metrics.New(streams))

	// HelmDDAConvert commands
	cmd.AddCommand(helm2dda.New(streams))

	o := newOptions(streams)
	o.configFlags.AddFlags(cmd.Flags())

	return cmd
}
