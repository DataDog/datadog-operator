// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func TestShouldRunProcessChecksInCoreAgent(t *testing.T) {
	withNodeAgentImage := func(tag string) *v2alpha1.DatadogAgentSpec {
		return &v2alpha1.DatadogAgentSpec{
			Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.NodeAgentComponentName: {
					Image: &v2alpha1.AgentImageConfig{Name: "agent", Tag: tag},
				},
			},
		}
	}

	for _, tt := range []struct {
		name        string
		labels      map[string]string
		annotations map[string]string
		spec        *v2alpha1.DatadogAgentSpec
		want        bool
	}{
		{name: "supported Agent version", spec: withNodeAgentImage("7.60.0"), want: true},
		{name: "unsupported Agent version", spec: withNodeAgentImage("7.59.0")},
		{
			name:        "unprofiled Windows annotation",
			annotations: map[string]string{kubernetes.ProviderAnnotationKey: kubernetes.WindowsProvider},
			spec:        withNodeAgentImage("7.83.1"),
			want:        true,
		},
		{
			name:        "Windows profile",
			labels:      map[string]string{constants.ProfileLabelKey: "windows"},
			annotations: map[string]string{kubernetes.ProviderAnnotationKey: kubernetes.WindowsProvider},
			spec:        withNodeAgentImage("7.83.1"),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Labels: tt.labels, Annotations: tt.annotations}}
			assert.Equal(t, tt.want, ShouldRunProcessChecksInCoreAgent(dda, tt.spec))
		})
	}
}

func TestIsDataPlaneEnabled(t *testing.T) {
	withDataPlaneEnabled := func(enabled bool) *v2alpha1.DatadogAgentSpec {
		return &v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				DataPlane: &v2alpha1.DataPlaneFeatureConfig{Enabled: ptr.To(enabled)},
			},
		}
	}
	withNodeAgentImage := func(tag string) *v2alpha1.DatadogAgentSpec {
		return &v2alpha1.DatadogAgentSpec{
			Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.NodeAgentComponentName: {
					Image: &v2alpha1.AgentImageConfig{Name: "agent", Tag: tag},
				},
			},
		}
	}

	for _, tt := range []struct {
		name           string
		annotations    map[string]string
		spec           *v2alpha1.DatadogAgentSpec
		defaultEnabled bool
		want           bool
	}{
		{name: "runtime default disabled", spec: &v2alpha1.DatadogAgentSpec{}},
		{name: "runtime default enables current default Agent image at or above minimum version", spec: &v2alpha1.DatadogAgentSpec{}, defaultEnabled: true, want: true},
		{name: "runtime default does not enable older pinned Agent image", spec: withNodeAgentImage("7.83.0"), defaultEnabled: true},
		{name: "runtime default enables compatible pinned Agent image", spec: withNodeAgentImage("7.84.0"), defaultEnabled: true, want: true},
		{name: "runtime default enables compatible pinned Agent RC image", spec: withNodeAgentImage("7.84.0-rc.1"), defaultEnabled: true, want: true},
		{name: "explicit CRD enable overrides incompatible pinned Agent image", spec: func() *v2alpha1.DatadogAgentSpec {
			spec := withNodeAgentImage("7.81.0")
			spec.Features = &v2alpha1.DatadogFeatures{DataPlane: &v2alpha1.DataPlaneFeatureConfig{Enabled: ptr.To(true)}}
			return spec
		}(), defaultEnabled: true, want: true},
		{name: "legacy enable annotation", annotations: map[string]string{EnableADPAnnotation: "true"}, spec: &v2alpha1.DatadogAgentSpec{}, want: true},
		{name: "legacy disable annotation overrides runtime default", annotations: map[string]string{EnableADPAnnotation: "false"}, spec: &v2alpha1.DatadogAgentSpec{}, defaultEnabled: true},
		{name: "CRD enable overrides legacy disable", annotations: map[string]string{EnableADPAnnotation: "false"}, spec: withDataPlaneEnabled(true), defaultEnabled: true, want: true},
		{name: "CRD disable overrides legacy enable", annotations: map[string]string{EnableADPAnnotation: "true"}, spec: withDataPlaneEnabled(false), defaultEnabled: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Annotations: tt.annotations}}
			assert.Equal(t, tt.want, IsDataPlaneEnabled(dda, tt.spec, tt.defaultEnabled))
		})
	}
}

func TestEnableConfigSyncForDirectSend(t *testing.T) {
	containers := []apicommon.AgentContainerName{
		apicommon.CoreAgentContainerName,
		apicommon.SystemProbeContainerName,
	}

	t.Run("sets both env vars on every container", func(t *testing.T) {
		managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})

		EnableConfigSyncForDirectSend(managers, containers)

		want := []*corev1.EnvVar{
			{Name: common.DDAgentIpcPort, Value: DefaultAgentIpcPort},
			{Name: common.DDAgentIpcConfigRefreshInterval, Value: DefaultAgentIpcConfigRefreshInterval},
		}
		for _, container := range containers {
			assert.Equal(t, want, managers.EnvVarMgr.EnvVarsByC[container], "container %s", container)
		}
	})

	t.Run("keeps a user-provided value", func(t *testing.T) {
		managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
		userPort := &corev1.EnvVar{Name: common.DDAgentIpcPort, Value: "1234"}
		managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, userPort)

		EnableConfigSyncForDirectSend(managers, containers)

		want := []*corev1.EnvVar{
			userPort,
			{Name: common.DDAgentIpcConfigRefreshInterval, Value: DefaultAgentIpcConfigRefreshInterval},
		}
		assert.Equal(t, want, managers.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName])
	})
}

func kubernetesActionsSpec(enabled bool, communication *v2alpha1.KubernetesActionsCommunicationConfig, clusterAgentTag string) *v2alpha1.DatadogAgentSpec {
	spec := &v2alpha1.DatadogAgentSpec{
		Features: &v2alpha1.DatadogFeatures{
			KubernetesActions: &v2alpha1.KubernetesActionsFeatureConfig{
				Enabled:       ptr.To(enabled),
				Communication: communication,
			},
		},
	}
	if clusterAgentTag != "" {
		spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
			v2alpha1.ClusterAgentComponentName: {Image: &v2alpha1.AgentImageConfig{Tag: clusterAgentTag}},
		}
	}
	return spec
}

func TestClusterAgentVersion(t *testing.T) {
	assert.Equal(t, images.AgentLatestVersion, ClusterAgentVersion(nil))
	assert.Equal(t, images.AgentLatestVersion, ClusterAgentVersion(&v2alpha1.DatadogAgentSpec{}))
	assert.Equal(t, "7.82.0", ClusterAgentVersion(kubernetesActionsSpec(true, nil, "7.82.0")))
}

func TestIsKubernetesActionsRCEnabled(t *testing.T) {
	tests := []struct {
		name string
		spec *v2alpha1.DatadogAgentSpec
		want bool
	}{
		{name: "nil spec", spec: nil, want: false},
		{name: "feature not configured", spec: &v2alpha1.DatadogAgentSpec{Features: &v2alpha1.DatadogFeatures{}}, want: false},
		{name: "feature disabled", spec: kubernetesActionsSpec(false, nil, ""), want: false},
		{name: "communication unset defaults to true", spec: kubernetesActionsSpec(true, nil, ""), want: true},
		{name: "rc unset defaults to true", spec: kubernetesActionsSpec(true, &v2alpha1.KubernetesActionsCommunicationConfig{PAR: ptr.To(true)}, ""), want: true},
		{name: "rc false", spec: kubernetesActionsSpec(true, &v2alpha1.KubernetesActionsCommunicationConfig{RC: ptr.To(false)}, ""), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsKubernetesActionsRCEnabled(tt.spec))
		})
	}
}

func TestIsKubernetesActionsPAREnabled(t *testing.T) {
	par := &v2alpha1.KubernetesActionsCommunicationConfig{PAR: ptr.To(true)}
	tests := []struct {
		name          string
		spec          *v2alpha1.DatadogAgentSpec
		wantRequested bool
		wantEnabled   bool
	}{
		{name: "nil spec", spec: nil},
		{name: "communication unset defaults to false", spec: kubernetesActionsSpec(true, nil, "")},
		{name: "par false", spec: kubernetesActionsSpec(true, &v2alpha1.KubernetesActionsCommunicationConfig{PAR: ptr.To(false)}, "")},
		{name: "feature disabled", spec: kubernetesActionsSpec(false, par, "")},
		{name: "par true on the default cluster agent", spec: kubernetesActionsSpec(true, par, ""), wantRequested: true, wantEnabled: true},
		{name: "par true at the minimum cluster agent version", spec: kubernetesActionsSpec(true, par, "7.83.0"), wantRequested: true, wantEnabled: true},
		{name: "par true on a cluster agent without the bundle", spec: kubernetesActionsSpec(true, par, "7.82.0"), wantRequested: true, wantEnabled: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantRequested, IsKubernetesActionsPARRequested(tt.spec))
			assert.Equal(t, tt.wantEnabled, IsKubernetesActionsPAREnabled(tt.spec))
		})
	}
}
