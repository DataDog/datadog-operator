// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pause provides the "autoscaling dpa pause" commands.
package pause

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/dpa/common"
)

var pauseExample = `
  # pause a single DatadogPodAutoscaler
  %[1]s pause enable web-dpa -n prod

  # pause every DatadogPodAutoscaler matching a label, in every namespace
  %[1]s pause enable -l team=payments -A

  # resume it
  %[1]s pause disable web-dpa -n prod
`

const pauseLong = `Pause DatadogPodAutoscalers through the ` + common.PauseAnnotationKey + ` annotation.
The object spec is never modified.

While paused, the Cluster Agent keeps computing and reporting recommendations but
applies nothing: no horizontal scaling, no vertical rollout or in-place resize, and
no POD patching by the admission controller. Manual changes made to the target
workload are left untouched. Pause takes precedence over every force-* override.
The Cluster Agent reports it with the Active condition set to False, reason
` + common.LocallyPausedReason + `.

"disable" resumes every selected object, including those that were paused on
purpose before an incident: check the confirmation prompt, or use --dry-run first.

Requires Cluster Agent ` + common.MinClusterAgentVersion + ` or later; an older one ignores the annotation.`

// New provides the "pause" command.
func New(streams genericclioptions.IOStreams) *cobra.Command {
	example := fmt.Sprintf(pauseExample, common.CommandPath)
	cmd := &cobra.Command{
		Use:     "pause enable|disable [NAME...] [flags]",
		Short:   "Pause or resume DatadogPodAutoscalers",
		Long:    pauseLong,
		Example: example,
	}

	cmd.AddCommand(common.NewAnnotationCommand(streams, common.AnnotationSpec{
		Key:            common.PauseAnnotationKey,
		Value:          common.SetValue("true"),
		PastTense:      "paused",
		UntouchedState: "paused",
		Confirmation:   common.PauseConfirmation,
	}, &cobra.Command{
		Use:     "enable [NAME...] [flags]",
		Short:   "Stop DatadogPodAutoscalers from applying any change to their workloads",
		Long:    pauseLong,
		Example: example,
	}))

	cmd.AddCommand(common.NewAnnotationCommand(streams, common.AnnotationSpec{
		Key:            common.PauseAnnotationKey,
		Value:          common.RemoveKey,
		PastTense:      "unpaused",
		UntouchedState: "not paused",
		Confirmation:   common.PauseConfirmation,
	}, &cobra.Command{
		Use:     "disable [NAME...] [flags]",
		Short:   "Resume paused DatadogPodAutoscalers",
		Long:    pauseLong,
		Example: example,
	}))

	return cmd
}
