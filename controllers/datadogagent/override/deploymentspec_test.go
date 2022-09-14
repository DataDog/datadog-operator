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
)

func TestDeployment(t *testing.T) {
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
