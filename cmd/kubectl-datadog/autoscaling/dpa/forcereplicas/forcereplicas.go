// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package forcereplicas provides the "autoscaling dpa force-replicas" commands.
package forcereplicas

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/dpa/common"
)

var forceReplicasExample = `
  # force a DatadogPodAutoscaler to 10 replicas
  %[1]s force-replicas set --replicas 10 web-dpa -n prod

  # stop forcing the replica count
  %[1]s force-replicas unset web-dpa -n prod
`

const forceReplicasLong = `Force the replica count of DatadogPodAutoscalers through the
` + common.ForceReplicasAnnotationKey + ` annotation, ignoring recommendations from every
source. The object spec is never modified.

This is a break-glass override: the count is NOT clamped by spec.constraints and is
reached in one step, without the scaling rules and stabilization windows, even when
the workload is scaled to zero or one scaling direction is disabled. It is still
suppressed by pause, by applyPolicy.mode: Preview, and when both scaling directions
are disabled. While applied, the Cluster Agent reports it in the
HorizontalScalingLimited condition.

Requires Cluster Agent ` + common.MinClusterAgentVersion + ` or later; an older one ignores the annotation.`

// New provides the "force-replicas" command.
func New(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "force-replicas set|unset [NAME...] [flags]",
		Short:   "Force or stop forcing the replica count of DatadogPodAutoscalers",
		Long:    forceReplicasLong,
		Example: fmt.Sprintf(forceReplicasExample, common.CommandPath),
	}

	var replicas int32
	set := common.NewAnnotationCommand(streams, common.AnnotationSpec{
		Key: common.ForceReplicasAnnotationKey,
		Validate: func() error {
			if replicas < 1 {
				return errors.New("--replicas must be at least 1")
			}
			return nil
		},
		Value: func(*v1alpha2.DatadogPodAutoscaler) (common.AnnotationValue, error) {
			return common.Set(strconv.Itoa(int(replicas))), nil
		},
		PastTense:      "forced to the requested replicas",
		UntouchedState: "forced to the requested replicas",
		Confirmation:   common.ForceReplicasConfirmation,
	}, &cobra.Command{
		Use:   "set --replicas N [NAME...] [flags]",
		Short: "Force the replica count",
		Long:  forceReplicasLong,
	})
	set.Flags().Int32Var(&replicas, "replicas", 0, "Replica count to force (at least 1)")
	_ = set.MarkFlagRequired("replicas")
	cmd.AddCommand(set)

	cmd.AddCommand(common.NewAnnotationCommand(streams, common.AnnotationSpec{
		Key:            common.ForceReplicasAnnotationKey,
		Value:          common.RemoveKey,
		PastTense:      "released from the forced replica count",
		UntouchedState: "not forced to a replica count",
		Confirmation:   common.ForceReplicasConfirmation,
	}, &cobra.Command{
		Use:   "unset [NAME...] [flags]",
		Short: "Stop forcing the replica count",
		Long:  forceReplicasLong,
	}))

	return cmd
}
