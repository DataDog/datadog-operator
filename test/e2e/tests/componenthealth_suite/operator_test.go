// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package componenthealthsuite

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/utils/ptr"
)

func TestOperatorIntakePatchPreservesDeployment(t *testing.T) {
	original := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(2)),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "operator"}},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{
					{Name: "sidecar", Image: "sidecar:latest"},
					{
						Name: operatorContainerName, Image: "operator:latest",
						Env: []corev1.EnvVar{
							{Name: "DD_COMPONENT_HEALTH_ENABLED", Value: "true"},
							{Name: envIntakeEnabled, Value: "false"},
							{Name: envIntakeURL, Value: "http://previous-intake/api/v2/agenthealth"},
						},
					},
				}},
			},
		},
	}
	url := "http://fakeintake/api/v2/agenthealth"
	patch, err := operatorIntakePatch(url)
	require.NoError(t, err)
	data, err := json.Marshal(original)
	require.NoError(t, err)
	data, err = strategicpatch.StrategicMergePatch(data, patch, appsv1.Deployment{})
	require.NoError(t, err)
	var updated appsv1.Deployment
	require.NoError(t, json.Unmarshal(data, &updated))

	expected := original.DeepCopy()
	expected.Spec.Template.Spec.Containers[1].Env[1].Value = "true"
	expected.Spec.Template.Spec.Containers[1].Env[2].Value = url
	require.Equal(t, expected, &updated, "patch must only replace the two intake env values")
}

func TestOperatorRolloutAvailability(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    appsv1.DeploymentStatus
		available bool
	}{
		{
			name:   "previous generation still observed",
			status: appsv1.DeploymentStatus{ObservedGeneration: 1, Replicas: 1, UpdatedReplicas: 1, AvailableReplicas: 1},
		},
		{
			name:   "old pod is available but replacement is not",
			status: appsv1.DeploymentStatus{ObservedGeneration: 2, Replicas: 2, UpdatedReplicas: 1, AvailableReplicas: 1},
		},
		{
			name:   "updated pod is running but unavailable",
			status: appsv1.DeploymentStatus{ObservedGeneration: 2, Replicas: 1, UpdatedReplicas: 1},
		},
		{
			name:      "updated generation is available",
			status:    appsv1.DeploymentStatus{ObservedGeneration: 2, Replicas: 1, UpdatedReplicas: 1, AvailableReplicas: 1},
			available: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec:       appsv1.DeploymentSpec{Replicas: ptr.To(int32(1))},
				Status:     tc.status,
			}
			require.Equal(t, tc.available, operatorRolloutAvailable(dep, 2))
		})
	}
}
