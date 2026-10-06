// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package forcefallback provides the "autoscaling dpa force-fallback" commands.
package forcefallback

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/dpa/common"
)

var forceFallbackExample = `
  # use the local recommender for every DatadogPodAutoscaler matching a label
  %[1]s force-fallback enable -l team=payments -A

  # stop forcing it (back to the default staleness-driven fallback)
  %[1]s force-fallback disable -l team=payments -A
`

const forceFallbackLong = `Force the in-cluster local recommender as the active horizontal source, through
the ` + common.ForceFallbackAnnotationKey + ` annotation, as if recommendations from
Datadog were stale, even when they are fresh.

"disable" removes the annotation and only stops FORCING the fallback: the default
behaviour, where the local fallback engages once recommendations become stale,
applies again. It does NOT disable the local fallback; that is
spec.fallback.horizontal.enabled.

It works whatever the owner of the DatadogPodAutoscaler, local or remote.

Caveats:
  - pause and force-replicas take precedence over it.
  - until the local recommender has fresh values, no new horizontal
    recommendation is applied: the workload keeps its current replicas.
  - forcing the fallback has no effect when the applyPolicy disables both
    scaling directions.
  - forcing the fallback never overrides the spec: it has no effect when
    spec.fallback.horizontal.enabled is false.
  - the Cluster Agent reports no condition for it: "enable" waits for
    .status.horizontal.target.source to be Local, the local recommender being
    active, and only warns, without failing, when it is not: the annotation is
    applied, the local recommender may have no fresh values yet. "disable" does
    not wait: the source stays Local while recommendations from Datadog are stale.

Requires Cluster Agent ` + common.MinClusterAgentVersion + ` or later; an older one ignores the annotation.`

// New provides the "force-fallback" command.
func New(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "force-fallback enable|disable [NAME...] [flags]",
		Short:   "Force or stop forcing the local fallback of DatadogPodAutoscalers",
		Long:    forceFallbackLong,
		Example: fmt.Sprintf(forceFallbackExample, common.CommandPath),
	}

	cmd.AddCommand(common.NewAnnotationCommand(streams, common.AnnotationSpec{
		Key:            common.ForceFallbackAnnotationKey,
		Value:          common.SetValue("true"),
		PastTense:      "forced to use the local fallback",
		UntouchedState: "forcing the local fallback",
		Confirmation:   common.ForceFallbackConfirmation,
	}, &cobra.Command{
		Use:   "enable [NAME...] [flags]",
		Short: "Force the local recommender as the active horizontal source",
		Long:  forceFallbackLong,
	}))

	cmd.AddCommand(common.NewAnnotationCommand(streams, common.AnnotationSpec{
		Key:            common.ForceFallbackAnnotationKey,
		Value:          common.RemoveKey,
		PastTense:      "switched back to the default fallback",
		UntouchedState: "not forcing the local fallback",
	}, &cobra.Command{
		Use:   "disable [NAME...] [flags]",
		Short: "Stop forcing the local fallback (does NOT disable the fallback)",
		Long:  forceFallbackLong,
	}))

	return cmd
}
