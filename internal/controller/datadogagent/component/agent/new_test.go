// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func TestNewDaemonset(t *testing.T) {
	dda := &metav1.ObjectMeta{Name: "datadog", Namespace: "agents"}

	daemonset := NewDaemonset(
		dda,
		&ExtendedDaemonsetOptions{MaxPodUnavailable: "25%"},
		constants.DefaultAgentResourceSuffix,
		"datadog-agent",
		"7.78.0",
		nil,
		"datadog-agent",
	)

	require.Equal(t, "datadog-agent", daemonset.Name)
	require.Equal(t, "agents", daemonset.Namespace)
	require.Equal(t, "datadog-agent", daemonset.Labels[kubernetes.AppKubernetesInstanceLabelKey])
	require.Equal(t, constants.DefaultAgentResourceSuffix, daemonset.Labels[apicommon.AgentDeploymentComponentLabelKey])
	require.Equal(t, intstr.FromString("25%"), *daemonset.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable)
	require.Equal(t, map[string]string{
		kubernetes.AppKubernetesInstanceLabelKey:   "datadog-agent",
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
	}, daemonset.Spec.Selector.MatchLabels)
}

func TestNewExtendedDaemonset(t *testing.T) {
	dda := &metav1.ObjectMeta{Name: "datadog", Namespace: "agents"}

	extendedDaemonSet := NewExtendedDaemonset(
		dda,
		&ExtendedDaemonsetOptions{
			MaxPodUnavailable:                   "25%",
			MaxPodSchedulerFailure:              "10%",
			SlowStartAdditiveIncrease:           "2",
			CanaryDuration:                      5 * time.Minute,
			CanaryReplicas:                      "2",
			CanaryAutoPauseEnabled:              true,
			CanaryAutoPauseMaxRestarts:          3,
			CanaryAutoFailEnabled:               true,
			CanaryAutoFailMaxRestarts:           4,
			CanaryAutoPauseMaxSlowStartDuration: time.Minute,
		},
		constants.DefaultAgentResourceSuffix,
		"datadog-agent",
		"7.78.0",
		nil,
	)

	require.Equal(t, "datadog-agent", extendedDaemonSet.Name)
	require.Equal(t, "agents", extendedDaemonSet.Namespace)
	require.Equal(t, "25%", extendedDaemonSet.Spec.Strategy.RollingUpdate.MaxUnavailable.StrVal)
	require.Equal(t, "10%", extendedDaemonSet.Spec.Strategy.RollingUpdate.MaxPodSchedulerFailure.StrVal)
	require.Equal(t, intstr.FromInt(2), *extendedDaemonSet.Spec.Strategy.RollingUpdate.SlowStartAdditiveIncrease)
	require.Equal(t, 5*time.Minute, extendedDaemonSet.Spec.Strategy.Canary.Duration.Duration)
	require.Equal(t, intstr.FromInt(2), *extendedDaemonSet.Spec.Strategy.Canary.Replicas)
	require.True(t, *extendedDaemonSet.Spec.Strategy.Canary.AutoPause.Enabled)
	require.Equal(t, int32(3), *extendedDaemonSet.Spec.Strategy.Canary.AutoPause.MaxRestarts)
	require.True(t, *extendedDaemonSet.Spec.Strategy.Canary.AutoFail.Enabled)
	require.Equal(t, int32(4), *extendedDaemonSet.Spec.Strategy.Canary.AutoFail.MaxRestarts)
	require.Equal(t, time.Minute, extendedDaemonSet.Spec.Strategy.Canary.AutoPause.MaxSlowStartDuration.Duration)
}
