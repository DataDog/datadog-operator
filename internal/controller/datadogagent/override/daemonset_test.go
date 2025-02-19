// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/stretchr/testify/assert"

	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestDaemonSet(t *testing.T) {
	tests := []struct {
		daemonSet v1.DaemonSet
		override  v2alpha1.DatadogAgentComponentOverride
		expected  v1.DaemonSet
	}{
		{
			daemonSet: makeDaemonSet(
				apiutils.NewPointer("RollingUpdate"),
				apiutils.NewPointer("50%"),
				apiutils.NewPointer("50%"),
			),
			override: makeOverride(
				apiutils.NewPointer("RollingUpdate"),
				apiutils.NewPointer("0%"),
				apiutils.NewPointer("0%"),
			),
			expected: makeDaemonSet(
				apiutils.NewPointer("RollingUpdate"),
				apiutils.NewPointer("0%"),
				apiutils.NewPointer("0%"),
			),
		},
		{
			daemonSet: makeDaemonSet(
				apiutils.NewPointer("RollingUpdate"),
				apiutils.NewPointer("50%"),
				apiutils.NewPointer("50%"),
			),
			override: makeOverride(
				apiutils.NewPointer("OnDelete"),
				nil,
				nil,
			),
			expected: makeDaemonSet(
				apiutils.NewPointer("OnDelete"),
				nil,
				nil,
			),
		},
		{
			daemonSet: makeDaemonSet(
				apiutils.NewPointer("RollingUpdate"),
				apiutils.NewPointer("50%"),
				apiutils.NewPointer("50%"),
			),
			override: makeOverride(
				apiutils.NewPointer("OnDelete"),
				apiutils.NewPointer("50%"),
				nil,
			),
			expected: makeDaemonSet(
				apiutils.NewPointer("OnDelete"),
				apiutils.NewPointer("50%"),
				nil,
			),
		},
		{
			daemonSet: makeDaemonSet(
				nil,
				nil,
				nil,
			),
			override: makeOverride(
				apiutils.NewPointer("OnDelete"),
				apiutils.NewPointer("25%"),
				apiutils.NewPointer("25%"),
			),
			expected: makeDaemonSet(
				apiutils.NewPointer("OnDelete"),
				apiutils.NewPointer("25%"),
				apiutils.NewPointer("25%"),
			),
		},
	}

	for _, test := range tests {
		DaemonSet(&test.daemonSet, &test.override)

		assert.Equal(t, test.daemonSet, test.expected)
	}

	daemonSet := v1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "current-name",
		},
	}

	override := v2alpha1.DatadogAgentComponentOverride{
		Name: apiutils.NewPointer("new-name"),
	}

	DaemonSet(&daemonSet, &override)

	assert.Equal(t, "new-name", daemonSet.Name)
}

func makeDaemonSet(strategyType *string, strategyMaxUnavailable *string, strategyMaxSurge *string) v1.DaemonSet {
	daemonSet := v1.DaemonSet{
		Spec: v1.DaemonSetSpec{
			UpdateStrategy: v1.DaemonSetUpdateStrategy{
				Type: "",
				RollingUpdate: &v1.RollingUpdateDaemonSet{
					MaxUnavailable: &intstr.IntOrString{},
					MaxSurge:       &intstr.IntOrString{},
				},
			},
		},
	}

	if strategyType != nil {
		daemonSet.Spec.UpdateStrategy.Type = v1.DaemonSetUpdateStrategyType(*strategyType)
	}

	if strategyMaxUnavailable != nil {
		daemonSet.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable.StrVal = *strategyMaxUnavailable
	}

	if strategyMaxSurge != nil {
		daemonSet.Spec.UpdateStrategy.RollingUpdate.MaxSurge.StrVal = *strategyMaxSurge
	}

	return daemonSet
}

func makeOverride(strategyType *string, strategyMaxUnavailable *string, strategyMaxSurge *string) v2alpha1.DatadogAgentComponentOverride {
	override := v2alpha1.DatadogAgentComponentOverride{
		UpdateStrategy: &apicommon.UpdateStrategy{
			Type: "",
			RollingUpdate: &apicommon.RollingUpdate{
				MaxUnavailable: &intstr.IntOrString{},
				MaxSurge:       &intstr.IntOrString{},
			},
		},
	}

	if strategyType != nil {
		override.UpdateStrategy.Type = *strategyType
	}

	if strategyMaxUnavailable != nil {
		override.UpdateStrategy.RollingUpdate.MaxUnavailable.StrVal = *strategyMaxUnavailable
	}

	if strategyMaxSurge != nil {
		override.UpdateStrategy.RollingUpdate.MaxSurge.StrVal = *strategyMaxSurge
	}

	return override
}
