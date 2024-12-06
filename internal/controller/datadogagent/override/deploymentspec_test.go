// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/crds/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/crds/utils"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestDeployment(t *testing.T) {
	tests := []struct {
		deployment v1.Deployment
		override   v2alpha1.DatadogAgentComponentOverride
		expected   v1.Deployment
	}{
		{
			deployment: makeDeployment(
				apiutils.NewStringPointer("RollingUpdate"),
				apiutils.NewStringPointer("50%"),
				apiutils.NewStringPointer("50%"),
			),
			override: makeOverride(
				apiutils.NewStringPointer("RollingUpdate"),
				apiutils.NewStringPointer("0%"),
				apiutils.NewStringPointer("0%"),
			),
			expected: makeDeployment(
				apiutils.NewStringPointer("RollingUpdate"),
				apiutils.NewStringPointer("0%"),
				apiutils.NewStringPointer("0%"),
			),
		},
		{
			deployment: makeDeployment(
				apiutils.NewStringPointer("RollingUpdate"),
				apiutils.NewStringPointer("50%"),
				apiutils.NewStringPointer("50%"),
			),
			override: makeOverride(
				apiutils.NewStringPointer("Recreate"),
				nil,
				nil,
			),
			expected: makeDeployment(
				apiutils.NewStringPointer("Recreate"),
				nil,
				nil,
			),
		},
		{
			deployment: makeDeployment(
				apiutils.NewStringPointer("RollingUpdate"),
				apiutils.NewStringPointer("50%"),
				apiutils.NewStringPointer("50%"),
			),
			override: makeOverride(
				apiutils.NewStringPointer("Recreate"),
				apiutils.NewStringPointer("50%"),
				nil,
			),
			expected: makeDeployment(
				apiutils.NewStringPointer("Recreate"),
				apiutils.NewStringPointer("50%"),
				nil,
			),
		},
		{
			deployment: makeDeployment(
				nil,
				nil,
				nil,
			),
			override: makeOverride(
				apiutils.NewStringPointer("Recreate"),
				apiutils.NewStringPointer("25%"),
				apiutils.NewStringPointer("25%"),
			),
			expected: makeDeployment(
				apiutils.NewStringPointer("Recreate"),
				apiutils.NewStringPointer("25%"),
				apiutils.NewStringPointer("25%"),
			),
		},
	}

	for _, test := range tests {
		Deployment(&test.deployment, &test.override)
		assert.Equal(t, test.deployment, test.expected)
	}

	deployment := v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "current-name",
		},
		Spec: v1.DeploymentSpec{
			Replicas: apiutils.NewInt32Pointer(1),
		},
	}

	override := v2alpha1.DatadogAgentComponentOverride{
		Name:     apiutils.NewStringPointer("new-name"),
		Replicas: apiutils.NewInt32Pointer(2),
	}

	Deployment(&deployment, &override)

	assert.Equal(t, "new-name", deployment.Name)
	assert.Equal(t, int32(2), *deployment.Spec.Replicas)
}

func makeDeployment(strategyType *string, strategyMaxUnavailable *string, strategyMaxSurge *string) v1.Deployment {
	deployment := v1.Deployment{
		Spec: v1.DeploymentSpec{
			Strategy: v1.DeploymentStrategy{
				Type: "",
				RollingUpdate: &v1.RollingUpdateDeployment{
					MaxUnavailable: &intstr.IntOrString{},
					MaxSurge:       &intstr.IntOrString{},
				},
			},
		},
	}

	if strategyType != nil {
		deployment.Spec.Strategy.Type = v1.DeploymentStrategyType(*strategyType)
	}

	if strategyMaxUnavailable != nil {
		deployment.Spec.Strategy.RollingUpdate.MaxUnavailable.StrVal = *strategyMaxUnavailable
	}

	if strategyMaxSurge != nil {
		deployment.Spec.Strategy.RollingUpdate.MaxSurge.StrVal = *strategyMaxSurge
	}

	return deployment
}
