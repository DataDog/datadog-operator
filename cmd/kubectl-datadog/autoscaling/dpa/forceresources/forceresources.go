// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package forceresources provides the "autoscaling dpa force-resources" commands.
package forceresources

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/dpa/common"
)

var forceResourcesExample = `
  # force the cpu of the "app" container of the checkout-dpa DatadogPodAutoscaler
  %[1]s force-resources set -c app --requests cpu=2 --limits cpu=4 checkout-dpa

  # also force the memory of the "sidecar" container; "app" is kept
  %[1]s force-resources set -c sidecar --requests memory=512Mi checkout-dpa

  # stop forcing the "sidecar" container only
  %[1]s force-resources unset -c sidecar checkout-dpa

  # stop forcing every container
  %[1]s force-resources unset checkout-dpa
`

const forceResourcesLong = `Force container cpu and memory requests and limits of DatadogPodAutoscalers,
through the ` + common.ForceResourcesAnnotationKey + ` annotation. The object spec is never
modified.

Positional arguments are DatadogPodAutoscaler names; containers are named with -c.
"set" merges into the current annotation: containers, resources, requests and limits
that are not passed are kept. Everything not forced keeps following the recommendation.

Forced values are overlaid on the vertical recommendation: without a vertical
recommendation, nothing is forced. They are bounded by spec.constraints like
recommended values, and rolled out like a recommendation that raises limits: pods
may be evicted or rolled out, unless in-place resize is enabled, even during an
ongoing rollout. They are still suppressed by pause and by applyPolicy.mode: Preview.

Requires Cluster Agent ` + common.MinClusterAgentVersion + ` or later; an older one ignores the annotation.`

type options struct {
	containers []string
	requests   string
	limits     string

	requestList corev1.ResourceList
	limitList   corev1.ResourceList
}

// New provides the "force-resources" command.
func New(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "force-resources set|unset [NAME...] [flags]",
		Short:   "Force or stop forcing container resources of DatadogPodAutoscalers",
		Long:    forceResourcesLong,
		Example: fmt.Sprintf(forceResourcesExample, common.CommandPath),
	}
	cmd.AddCommand(newSetCommand(streams), newUnsetCommand(streams))
	return cmd
}

func newSetCommand(streams genericclioptions.IOStreams) *cobra.Command {
	o := &options{}
	cmd := common.NewAnnotationCommand(streams, common.AnnotationSpec{
		Key:            common.ForceResourcesAnnotationKey,
		Validate:       o.validateSet,
		Value:          o.setValue,
		PastTense:      "updated with the forced resources",
		UntouchedState: "forcing these resources",
		Confirmation:   common.ForceResourcesConfirmation,
	}, &cobra.Command{
		Use:     "set -c CONTAINER... [--requests LIST] [--limits LIST] [NAME...] [flags]",
		Example: fmt.Sprintf(forceResourcesExample, common.CommandPath),
		Short:   "Force container resources, merged into the current ones",
		Long:    forceResourcesLong,
	})
	cmd.Flags().StringSliceVarP(&o.containers, "container", "c", nil, "Container to force; repeatable, the same values apply to each")
	cmd.Flags().StringVar(&o.requests, "requests", "", "Requests to force, e.g. cpu=2,memory=1Gi")
	cmd.Flags().StringVar(&o.limits, "limits", "", "Limits to force, e.g. cpu=4,memory=2Gi")
	return cmd
}

func newUnsetCommand(streams genericclioptions.IOStreams) *cobra.Command {
	o := &options{}
	cmd := common.NewAnnotationCommand(streams, common.AnnotationSpec{
		Key:            common.ForceResourcesAnnotationKey,
		Value:          o.unsetValue,
		PastTense:      "released from the forced resources",
		UntouchedState: "without these forced resources",
		Confirmation:   common.ForceResourcesConfirmation,
	}, &cobra.Command{
		Use:     "unset [-c CONTAINER...] [NAME...] [flags]",
		Example: fmt.Sprintf(forceResourcesExample, common.CommandPath),
		Short:   "Stop forcing container resources; without -c, for every container",
		Long:    forceResourcesLong,
	})
	cmd.Flags().StringSliceVarP(&o.containers, "container", "c", nil, "Container to stop forcing; repeatable. Without -c, the whole annotation is removed")
	return cmd
}

func (o *options) validateSet() error {
	// Without a container, the merge would be empty and remove the annotation.
	if len(o.containers) == 0 {
		return errors.New("--container is required")
	}
	if o.requests == "" && o.limits == "" {
		return errors.New("at least one of --requests or --limits is required")
	}
	var err error
	if o.requestList, err = parseResourceList(o.requests); err != nil {
		return fmt.Errorf("--requests: %w", err)
	}
	if o.limitList, err = parseResourceList(o.limits); err != nil {
		return fmt.Errorf("--limits: %w", err)
	}
	return forcedResources{}.merge(o.containers, o.requestList, o.limitList).validate()
}

func (o *options) setValue(dpa *v1alpha2.DatadogPodAutoscaler) (common.AnnotationValue, error) {
	current, err := parseValue(dpa.Annotations[common.ForceResourcesAnnotationKey])
	if err != nil {
		return common.AnnotationValue{}, fmt.Errorf(`current value is invalid (%w): run "unset" first`, err)
	}
	merged := current.merge(o.containers, o.requestList, o.limitList)
	if err := merged.validate(); err != nil {
		return common.AnnotationValue{}, fmt.Errorf("merged with the current value: %w", err)
	}
	return merged.value()
}

func (o *options) unsetValue(dpa *v1alpha2.DatadogPodAutoscaler) (common.AnnotationValue, error) {
	if len(o.containers) == 0 {
		return common.Removed, nil
	}
	current, err := parseValue(dpa.Annotations[common.ForceResourcesAnnotationKey])
	if err != nil {
		return common.AnnotationValue{}, fmt.Errorf(`current value is invalid (%w): run "unset" without -c`, err)
	}
	return current.remove(o.containers).value()
}
