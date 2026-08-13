// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	common "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/stretchr/testify/assert"
)

func TestPreparedRolloutBudgetUsesExistingMaxUnavailable(t *testing.T) {
	fivePercent := intstr.FromString("5%")
	ddai := &datadoghqv1alpha1.DatadogAgentInternal{Spec: datadoghqv2alpha1.DatadogAgentSpec{
		Override: map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
			datadoghqv2alpha1.NodeAgentComponentName: {
				UpdateStrategy: &common.UpdateStrategy{RollingUpdate: &common.RollingUpdate{MaxUnavailable: ptr.To(fivePercent)}},
			},
		},
	}}

	assert.Equal(t, fivePercent, preparedRolloutBudget(ddai, &componentagent.ExtendedDaemonsetOptions{MaxPodUnavailable: "20%"}))
	assert.Equal(t, intstr.FromString("20%"), preparedRolloutBudget(&datadoghqv1alpha1.DatadogAgentInternal{}, &componentagent.ExtendedDaemonsetOptions{MaxPodUnavailable: "20%"}))
	assert.Equal(t, intstr.FromInt(1), preparedRolloutBudget(&datadoghqv1alpha1.DatadogAgentInternal{}, nil))
}
