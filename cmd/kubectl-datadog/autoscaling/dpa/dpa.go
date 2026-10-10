// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dpa provides the DatadogPodAutoscaler break-glass commands.
package dpa

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/dpa/common"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/dpa/forcefallback"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/dpa/forcereplicas"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/dpa/forceresources"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/dpa/pause"
)

// New provides the "dpa" command.
func New(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dpa [subcommand] [flags]",
		Short: "Manage DatadogPodAutoscaler objects",
		Long: `Break-glass overrides for DatadogPodAutoscaler objects, through operational
annotations. The object spec is never modified.

Requires Cluster Agent ` + common.MinClusterAgentVersion + ` or later; an older one ignores the annotations.`,
	}

	cmd.AddCommand(pause.New(streams))
	cmd.AddCommand(forcefallback.New(streams))
	cmd.AddCommand(forcereplicas.New(streams))
	cmd.AddCommand(forceresources.New(streams))

	return cmd
}
