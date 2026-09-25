// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogsyntheticsprivatelocation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func Test_buildDeploymentDefaults(t *testing.T) {
	instance := newTestInstance()
	d := buildDeployment(instance, instance.Name)

	assert.Equal(t, "my-pl", d.Name)
	assert.Equal(t, "default", d.Namespace)
	assert.Equal(t, ptr.To[int32](1), d.Spec.Replicas)
	assert.Equal(t, "gcr.io/datadoghq/synthetics-private-location-worker:1.73.0", d.Spec.Template.Spec.Containers[0].Image)
	assert.Equal(t, corev1.PullIfNotPresent, d.Spec.Template.Spec.Containers[0].ImagePullPolicy)
	assert.Equal(t, "my-pl", d.Spec.Template.Spec.ServiceAccountName)

	// Selector contains the instance name so multiple private locations can
	// coexist in the same namespace.
	assert.Equal(t, map[string]string{
		appLabelKey:       workerAppLabelValue,
		instanceLabelKey:  "my-pl",
		managedByLabelKey: managedByLabelValue,
	}, d.Spec.Selector.MatchLabels)

	// Config Secret volume mounted at /etc/datadog.
	require.Len(t, d.Spec.Template.Spec.Containers[0].VolumeMounts, 2)
	assert.Equal(t, configVolumeName, d.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name)
	assert.Equal(t, "/etc/datadog", d.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath)
	require.NotNil(t, d.Spec.Template.Spec.Volumes[0].Secret)
	assert.Equal(t, "my-pl-config", d.Spec.Template.Spec.Volumes[0].Secret.SecretName)

	// /run is always an emptyDir, even without extra volumes.
	assert.Equal(t, runVolumeName, d.Spec.Template.Spec.Containers[0].VolumeMounts[1].Name)
	assert.Equal(t, "/run", d.Spec.Template.Spec.Containers[0].VolumeMounts[1].MountPath)
	require.NotNil(t, d.Spec.Template.Spec.Volumes[1].EmptyDir)

	// Probes disabled by default, and the env var is in sync.
	assert.Nil(t, d.Spec.Template.Spec.Containers[0].LivenessProbe)
	assert.Nil(t, d.Spec.Template.Spec.Containers[0].ReadinessProbe)
	assert.NotContains(t, d.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{Name: enableStatusProbesEnvVar})
}

func Test_buildDeploymentStatusProbesSync(t *testing.T) {
	instance := newTestInstance()
	instance.Annotations = map[string]string{datadoghqv1alpha1.DatadogSPLStatusProbesEnabledAnnotation: "true"}
	d := buildDeployment(instance, instance.Name)

	c := d.Spec.Template.Spec.Containers[0]
	require.NotNil(t, c.LivenessProbe)
	require.NotNil(t, c.LivenessProbe.HTTPGet)
	assert.Equal(t, "/liveness", c.LivenessProbe.HTTPGet.Path)
	assert.Equal(t, int32(8080), c.LivenessProbe.HTTPGet.Port.IntVal)
	require.NotNil(t, c.ReadinessProbe)
	require.NotNil(t, c.ReadinessProbe.HTTPGet)
	assert.Equal(t, "/readiness", c.ReadinessProbe.HTTPGet.Path)
	assert.Equal(t, int32(8080), c.ReadinessProbe.HTTPGet.Port.IntVal)

	assert.Contains(t, c.Env, corev1.EnvVar{Name: enableStatusProbesEnvVar, Value: "true"})
	assert.Contains(t, c.Env, corev1.EnvVar{Name: statusProbesPortEnvVar, Value: "8080"})
}

func Test_buildDeploymentStatusProbesDisabled(t *testing.T) {
	tests := []struct {
		name       string
		annotation string
		tag        string
	}{
		{name: "annotation is not true", annotation: "false"},
		{name: "worker version is too old", annotation: "true", tag: "1.11.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := newTestInstance()
			instance.Annotations = map[string]string{datadoghqv1alpha1.DatadogSPLStatusProbesEnabledAnnotation: tt.annotation}
			if tt.tag != "" {
				instance.Spec.Worker = &datadoghqv1alpha1.DatadogSPLWorker{
					Image: &datadoghqv1alpha1.DatadogSPLImage{Tag: tt.tag},
				}
			}
			d := buildDeployment(instance, instance.Name)

			c := d.Spec.Template.Spec.Containers[0]
			assert.Nil(t, c.LivenessProbe)
			assert.Nil(t, c.ReadinessProbe)
			for _, env := range c.Env {
				assert.NotEqual(t, enableStatusProbesEnvVar, env.Name)
			}
		})
	}
}

func Test_statusProbesSupported(t *testing.T) {
	tests := []struct {
		tag  string
		want bool
	}{
		{tag: "", want: true},
		{tag: "1.11.9", want: false},
		{tag: "1.12.0", want: true},
		{tag: "1.12.0-rc.1", want: true},
		{tag: "1.73.0", want: true},
		{tag: "latest", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			instance := newTestInstance()
			instance.Spec.Worker = &datadoghqv1alpha1.DatadogSPLWorker{
				Image: &datadoghqv1alpha1.DatadogSPLImage{Tag: tt.tag},
			}
			assert.Equal(t, tt.want, statusProbesSupported(instance))
		})
	}
}

func Test_buildDeploymentCustomization(t *testing.T) {
	instance := newTestInstance()
	instance.Spec.Worker = &datadoghqv1alpha1.DatadogSPLWorker{
		Replicas: ptr.To[int32](3),
		Image: &datadoghqv1alpha1.DatadogSPLImage{
			Repository:  "example.com/worker",
			Tag:         "2.0.0",
			PullPolicy:  corev1.PullAlways,
			PullSecrets: []corev1.LocalObjectReference{{Name: "regcred"}},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
		},
	}
	d := buildDeployment(instance, "my-sa")

	assert.Equal(t, ptr.To[int32](3), d.Spec.Replicas)
	assert.Equal(t, "example.com/worker:2.0.0", d.Spec.Template.Spec.Containers[0].Image)
	assert.Equal(t, corev1.PullAlways, d.Spec.Template.Spec.Containers[0].ImagePullPolicy)
	assert.Equal(t, "my-sa", d.Spec.Template.Spec.ServiceAccountName)
	assert.Equal(t, []corev1.LocalObjectReference{{Name: "regcred"}}, d.Spec.Template.Spec.ImagePullSecrets)
	assert.Equal(t, resource.MustParse("100m"), d.Spec.Template.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU])
}

func Test_buildDeploymentCommonLabels(t *testing.T) {
	instance := newTestInstance()
	instance.Spec.Worker = &datadoghqv1alpha1.DatadogSPLWorker{
		CommonLabels: map[string]string{
			"team":           "synthetics",
			appLabelKey:      "override-attempt",
			instanceLabelKey: "override-attempt",
		},
		PodLabels: map[string]string{"pod-only": "yes", "team": "override"},
	}
	d := buildDeployment(instance, instance.Name)

	// Operator-owned keys cannot be overridden.
	assert.Equal(t, workerAppLabelValue, d.Labels[appLabelKey])
	assert.Equal(t, "my-pl", d.Labels[instanceLabelKey])
	// Extra labels are added to both resource and pod labels.
	assert.Equal(t, "synthetics", d.Labels["team"])
	assert.Equal(t, "synthetics", d.Spec.Template.Labels["team"])
	// Pod labels appear only on the pod template; conflicts resolve in favor
	// of the operator-set value.
	assert.Equal(t, "yes", d.Spec.Template.Labels["pod-only"])
	assert.NotContains(t, d.Labels, "pod-only")
}

func Test_reconcileDeploymentCreateAndUpdate(t *testing.T) {
	s := newSchemeWithSPL(t)
	c := fake.NewClientBuilder().WithScheme(s).Build()
	instance := newTestInstance()

	// Create.
	status, err := reconcileDeployment(context.Background(), c, s, instance, instance.Name)
	require.NoError(t, err)
	assert.Nil(t, status)

	created := &appsv1.Deployment{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "my-pl", Namespace: "default"}, created))
	assertOwnedByInstance(t, created, instance)

	// Update after spec change.
	instance.Spec.Worker = &datadoghqv1alpha1.DatadogSPLWorker{
		Replicas: ptr.To[int32](4),
	}
	status, err = reconcileDeployment(context.Background(), c, s, instance, instance.Name)
	require.NoError(t, err)
	// No observed status on a fresh Deployment object from the fake client.
	assert.Equal(t, int32(0), status.Replicas)

	updated := &appsv1.Deployment{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "my-pl", Namespace: "default"}, updated))
	assert.Equal(t, ptr.To[int32](4), updated.Spec.Replicas)

	// Idempotence: re-reconciling does not bump the generation.
	gen := updated.Generation
	_, err = reconcileDeployment(context.Background(), c, s, instance, instance.Name)
	require.NoError(t, err)
	same := &appsv1.Deployment{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "my-pl", Namespace: "default"}, same))
	assert.Equal(t, gen, same.Generation)

	// External drift is reverted by the full-replacement update.
	same.Spec.Replicas = ptr.To[int32](99)
	require.NoError(t, c.Update(context.Background(), same))
	_, err = reconcileDeployment(context.Background(), c, s, instance, instance.Name)
	require.NoError(t, err)
	drifted := &appsv1.Deployment{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "my-pl", Namespace: "default"}, drifted))
	assert.Equal(t, ptr.To[int32](4), drifted.Spec.Replicas)
}
