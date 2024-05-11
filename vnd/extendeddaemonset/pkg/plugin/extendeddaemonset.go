// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// Package plugin contains kubectl plugin logic.
package plugin

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/extendeddaemonset/pkg/plugin/canary"
	"github.com/DataDog/extendeddaemonset/pkg/plugin/diff"
	"github.com/DataDog/extendeddaemonset/pkg/plugin/freeze"
	"github.com/DataDog/extendeddaemonset/pkg/plugin/get"
	"github.com/DataDog/extendeddaemonset/pkg/plugin/pause"
	"github.com/DataDog/extendeddaemonset/pkg/plugin/pods"
)

// ExtendedDaemonsetOptions provides information required to manage ExtendedDaemonset.
type ExtendedDaemonsetOptions struct {
	configFlags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
}

// NewExtendedDaemonsetOptions provides an instance of ExtendedDaemonsetOptions with default values.
func NewExtendedDaemonsetOptions(streams genericclioptions.IOStreams) *ExtendedDaemonsetOptions {
	return &ExtendedDaemonsetOptions{
		configFlags: genericclioptions.NewConfigFlags(false),

		IOStreams: streams,
	}
}

// NewCmdExtendedDaemonset provides a cobra command wrapping ExtendedDaemonsetOptions.
func NewCmdExtendedDaemonset(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewExtendedDaemonsetOptions(streams)

	cmd := &cobra.Command{
		Use: "kubectl eds [subcommand] [flags]",
	}

	cmd.AddCommand(canary.NewCmdCanary(streams))
	cmd.AddCommand(get.NewCmdGet(streams))
	cmd.AddCommand(get.NewCmdGetERS(streams))
	cmd.AddCommand(pods.NewCmdPods(streams))
	cmd.AddCommand(pause.NewCmdPause(streams))
	cmd.AddCommand(pause.NewCmdUnpause(streams))
	cmd.AddCommand(freeze.NewCmdFreeze(streams))
	cmd.AddCommand(freeze.NewCmdUnfreeze(streams))
	cmd.AddCommand(diff.NewCmdDiff(streams))

	o.configFlags.AddFlags(cmd.Flags())

	return cmd
}

// Complete sets all information required for processing the command.
func (o *ExtendedDaemonsetOptions) Complete(cmd *cobra.Command, args []string) error {
	return nil
}

// Validate ensures that all required arguments and flag values are provided.
func (o *ExtendedDaemonsetOptions) Validate() error {
	return nil
}

// Run use to run the command.
func (o *ExtendedDaemonsetOptions) Run() error {
	return nil
}
