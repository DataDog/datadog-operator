// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"testing"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestDeployment(t *testing.T) {
	deployment := v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "current-name",
		},
		Spec: v1.DeploymentSpec{
			Replicas: apiutils.NewInt32Pointer(1),
			Strategy: v1.DeploymentStrategy{
				Type: "RollingUpdate",
				RollingUpdate: &v1.RollingUpdateDeployment{
					MaxUnavailable: &intstr.IntOrString{
						StrVal: "25%",
					},
					MaxSurge: &intstr.IntOrString{
						StrVal: "25%",
					},
				},
			},
		},
	}

	override := v2alpha1.DatadogAgentComponentOverride{
		Name:     apiutils.NewStringPointer("new-name"),
		Replicas: apiutils.NewInt32Pointer(2),
		Strategy: &v1.DeploymentStrategy{
			Type: "RollingUpdate",
			RollingUpdate: &v1.RollingUpdateDeployment{
				MaxUnavailable: &intstr.IntOrString{
					StrVal: "50%",
				},
				MaxSurge: &intstr.IntOrString{
					StrVal: "50%",
				},
			},
		},
	}

	Deployment(&deployment, &override)

	assert.Equal(t, "new-name", deployment.Name)
	assert.Equal(t, int32(2), *deployment.Spec.Replicas)
	assert.Equal(t, v1.RollingUpdateDeploymentStrategyType, deployment.Spec.Strategy.Type)
	assert.Equal(t, "50%", deployment.Spec.Strategy.RollingUpdate.MaxUnavailable.StrVal)
	assert.Equal(t, "50%", deployment.Spec.Strategy.RollingUpdate.MaxSurge.StrVal)

}
