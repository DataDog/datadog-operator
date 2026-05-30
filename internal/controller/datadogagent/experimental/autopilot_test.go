// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experimental

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	mergerfake "github.com/DataDog/datadog-operator/internal/controller/datadogagent/merger/fake"
)

func findEnvVar(envs []*v1.EnvVar, name string) *v1.EnvVar {
	for _, e := range envs {
		if e.Name == name {
			return e
		}
	}
	return nil
}

func TestApplyExperimentalAutopilotOverrides_KubeletUseAPIServerEnvVar(t *testing.T) {
	tests := []struct {
		name              string
		autopilotEnabled  bool
		expectEnvVarValue string // empty means env var should NOT be present
	}{
		{
			name:              "autopilot enabled adds DD_KUBELET_USE_API_SERVER=true",
			autopilotEnabled:  true,
			expectEnvVarValue: "true",
		},
		{
			name:              "autopilot disabled does not add the env var",
			autopilotEnabled:  false,
			expectEnvVarValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{})

			dda := &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
			}
			if tt.autopilotEnabled {
				dda.Annotations[getExperimentalAnnotationKey(ExperimentalAutopilotSubkey)] = "true"
			}

			applyExperimentalAutopilotOverrides(dda, manager)

			got := findEnvVar(manager.EnvVarMgr.EnvVarsByC[mergerfake.AllContainers], DDKubeletUseAPIServer)
			if tt.expectEnvVarValue == "" {
				assert.Nil(t, got, "DD_KUBELET_USE_API_SERVER should not be set when autopilot is disabled")
				return
			}
			if assert.NotNil(t, got, "DD_KUBELET_USE_API_SERVER should be set when autopilot is enabled") {
				assert.Equal(t, tt.expectEnvVarValue, got.Value)
			}
		})
	}
}

func TestApplyExperimentalAutopilotOverrides_CloudProviderMetadataEnvVar(t *testing.T) {
	tests := []struct {
		name              string
		autopilotEnabled  bool
		expectEnvVarValue string // empty means env var should NOT be present
	}{
		{
			name:              "autopilot enabled restricts cloud provider metadata to GCP",
			autopilotEnabled:  true,
			expectEnvVarValue: `["gcp"]`,
		},
		{
			name:              "autopilot disabled does not set cloud provider metadata",
			autopilotEnabled:  false,
			expectEnvVarValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{})

			dda := &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
			}
			if tt.autopilotEnabled {
				dda.Annotations[getExperimentalAnnotationKey(ExperimentalAutopilotSubkey)] = "true"
			}

			applyExperimentalAutopilotOverrides(dda, manager)

			got := findEnvVar(manager.EnvVarMgr.EnvVarsByC[mergerfake.AllContainers], DDCloudProviderMetadata)
			if tt.expectEnvVarValue == "" {
				assert.Nil(t, got, "DD_CLOUD_PROVIDER_METADATA should not be set when autopilot is disabled")
				return
			}
			if assert.NotNil(t, got, "DD_CLOUD_PROVIDER_METADATA should be set when autopilot is enabled") {
				assert.Equal(t, tt.expectEnvVarValue, got.Value)
			}
		})
	}
}

// TestApplyExperimentalAutopilotOverrides_NPMSurvives asserts that the volumes,
// mounts, and HostPID required by the NPM feature on the system-probe container
// are NOT stripped by the Autopilot overrides. NPM on Autopilot relies on the
// WorkloadAllowlist to grant the required exemptions; if the operator strips
// the mounts client-side, the system-probe container will fail to start even
// when the allowlist would have permitted it.
func TestApplyExperimentalAutopilotOverrides_NPMSurvives(t *testing.T) {
	npmVolumes := []v1.Volume{
		{Name: common.ProcdirVolumeName},
		{Name: common.CgroupsVolumeName},
		{Name: common.DebugfsVolumeName},
		{Name: common.SystemProbeSocketVolumeName},
	}
	npmMounts := []v1.VolumeMount{
		{Name: common.ProcdirVolumeName, MountPath: "/host/proc"},
		{Name: common.CgroupsVolumeName, MountPath: "/host/sys/fs/cgroup"},
		{Name: common.DebugfsVolumeName, MountPath: "/sys/kernel/debug"},
		{Name: common.SystemProbeSocketVolumeName, MountPath: "/var/run/sysprobe"},
	}

	manager := fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{
		Spec: v1.PodSpec{
			HostPID: true,
			Volumes: npmVolumes,
			Containers: []v1.Container{
				{Name: string(apicommon.SystemProbeContainerName), VolumeMounts: npmMounts},
				{Name: string(apicommon.CoreAgentContainerName)},
			},
		},
	})

	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				getExperimentalAnnotationKey(ExperimentalAutopilotSubkey): "true",
			},
		},
	}

	applyExperimentalAutopilotOverrides(dda, manager)

	tpl := manager.PodTemplateSpec()
	assert.True(t, tpl.Spec.HostPID, "HostPID should be preserved on autopilot for NPM")

	gotVolumes := map[string]bool{}
	for _, v := range tpl.Spec.Volumes {
		gotVolumes[v.Name] = true
	}
	for _, want := range npmVolumes {
		assert.True(t, gotVolumes[want.Name], "NPM volume %q should survive autopilot overrides", want.Name)
	}

	var sysProbeMounts []v1.VolumeMount
	for _, c := range tpl.Spec.Containers {
		if c.Name == string(apicommon.SystemProbeContainerName) {
			sysProbeMounts = c.VolumeMounts
			break
		}
	}
	gotMounts := map[string]bool{}
	for _, m := range sysProbeMounts {
		gotMounts[m.Name] = true
	}
	for _, want := range npmMounts {
		assert.True(t, gotMounts[want.Name], "NPM mount %q should survive autopilot overrides on system-probe", want.Name)
	}
}

func TestApplyExperimentalAutopilotOverrides_LogCollectionStoragePath(t *testing.T) {
	tests := []struct {
		name             string
		autopilotEnabled bool
		inputPath        string
		wantPath         string
	}{
		{
			name:             "autopilot enabled rewrites log collection storage hostPath",
			autopilotEnabled: true,
			inputPath:        common.DefaultLogTempStoragePath,
			wantPath:         autopilotLogCollectionStoragePath,
		},
		{
			name:             "autopilot disabled preserves log collection storage hostPath",
			autopilotEnabled: false,
			inputPath:        common.DefaultLogTempStoragePath,
			wantPath:         common.DefaultLogTempStoragePath,
		},
		{
			name:             "autopilot enabled rewrites custom log collection storage hostPath",
			autopilotEnabled: true,
			inputPath:        "/custom/log/storage",
			wantPath:         autopilotLogCollectionStoragePath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Volumes: []v1.Volume{
						{
							Name: common.RunPathVolumeName,
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{
									Path: tt.inputPath,
								},
							},
						},
					},
				},
			})

			dda := &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
			}
			if tt.autopilotEnabled {
				dda.Annotations[getExperimentalAnnotationKey(ExperimentalAutopilotSubkey)] = "true"
			}

			applyExperimentalAutopilotOverrides(dda, manager)

			volumes := manager.PodTemplateSpec().Spec.Volumes
			if assert.Len(t, volumes, 1) && assert.NotNil(t, volumes[0].HostPath) {
				assert.Equal(t, tt.wantPath, volumes[0].HostPath.Path)
			}
		})
	}
}

func TestApplyExperimentalAutopilotOverrides_RunPathEmptyDirIsPreserved(t *testing.T) {
	manager := fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				common.GetVolumeForRunPath(),
			},
		},
	})
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				getExperimentalAnnotationKey(ExperimentalAutopilotSubkey): "true",
			},
		},
	}

	applyExperimentalAutopilotOverrides(dda, manager)

	volumes := manager.PodTemplateSpec().Spec.Volumes
	if assert.Len(t, volumes, 1) {
		assert.Nil(t, volumes[0].HostPath)
		assert.NotNil(t, volumes[0].EmptyDir)
	}
}

func TestApplyExperimentalAutopilotOverrides_RemovesAuthTokenFilePathAndAuthMounts(t *testing.T) {
	authEnv := []v1.EnvVar{
		{Name: common.DDAuthTokenFilePath, Value: "/etc/datadog-agent/auth/token"},
		{Name: common.DDClusterAgentEnabled, Value: "true"},
	}

	manager := fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{
		Spec: v1.PodSpec{
			InitContainers: []v1.Container{
				{
					Name: string(apicommon.InitConfigContainerName),
					Env:  authEnv,
					VolumeMounts: []v1.VolumeMount{
						{Name: common.AuthVolumeName},
						{Name: common.CriSocketVolumeName},
						{Name: common.LogDatadogVolumeName},
					},
				},
				{
					Name: string(apicommon.SeccompSetupContainerName),
					VolumeMounts: []v1.VolumeMount{
						{Name: common.SeccompSecurityVolumeName, ReadOnly: false},
						{Name: common.SeccompRootVolumeName, ReadOnly: false},
					},
				},
			},
			Volumes: []v1.Volume{
				{Name: common.LogDatadogVolumeName},
				{Name: common.AuthVolumeName},
				{Name: common.DogstatsdSocketVolumeName},
				{Name: common.SeccompSecurityVolumeName},
				{Name: common.SeccompRootVolumeName},
				{Name: common.ProcdirVolumeName},
				{Name: common.CgroupsVolumeName},
			},
			Containers: []v1.Container{
				{
					Name: string(apicommon.SystemProbeContainerName),
					Env:  authEnv,
					VolumeMounts: []v1.VolumeMount{
						{Name: common.LogDatadogVolumeName},
						{Name: common.AuthVolumeName},
						{Name: common.DogstatsdSocketVolumeName},
						{Name: common.ProcdirVolumeName},
						{Name: common.CgroupsVolumeName},
					},
				},
				{
					Name: string(apicommon.OtelAgent),
					Env:  authEnv,
					VolumeMounts: []v1.VolumeMount{
						{Name: common.AuthVolumeName},
						{Name: common.LogDatadogVolumeName},
					},
				},
				{
					Name: string(apicommon.AgentDataPlaneContainerName),
					Env:  authEnv,
					VolumeMounts: []v1.VolumeMount{
						{Name: common.AuthVolumeName},
						{Name: common.LogDatadogVolumeName},
					},
				},
			},
		},
	})

	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				getExperimentalAnnotationKey(ExperimentalAutopilotSubkey): "true",
			},
		},
	}

	applyExperimentalAutopilotOverrides(dda, manager)

	tpl := manager.PodTemplateSpec()
	remainingVolumes := map[string]bool{}
	for _, v := range tpl.Spec.Volumes {
		remainingVolumes[v.Name] = true
	}

	assert.False(t, remainingVolumes[common.AuthVolumeName], "auth volume should be stripped on autopilot")
	assert.False(t, remainingVolumes[common.DogstatsdSocketVolumeName], "DogStatsD socket volume should be stripped on autopilot")

	for _, c := range tpl.Spec.InitContainers {
		for _, e := range c.Env {
			assert.NotEqual(t, common.DDAuthTokenFilePath, e.Name, "init container %s should not keep DD_AUTH_TOKEN_FILE_PATH on autopilot", c.Name)
		}
		for _, m := range c.VolumeMounts {
			assert.NotEqual(t, common.AuthVolumeName, m.Name, "init container %s should not keep auth mount on autopilot", c.Name)
			if c.Name == string(apicommon.SeccompSetupContainerName) && m.Name == common.SeccompSecurityVolumeName {
				assert.True(t, m.ReadOnly, "seccomp-setup datadog-agent-security mount should be read-only on autopilot")
			}
		}
	}

	for _, c := range tpl.Spec.Containers {
		mounts := map[string]bool{}
		for _, e := range c.Env {
			assert.NotEqual(t, common.DDAuthTokenFilePath, e.Name, "container %s should not keep DD_AUTH_TOKEN_FILE_PATH on autopilot", c.Name)
		}
		for _, m := range c.VolumeMounts {
			mounts[m.Name] = true
			assert.NotEqual(t, common.AuthVolumeName, m.Name, "container %s should not keep auth mount on autopilot", c.Name)
			assert.True(t, remainingVolumes[m.Name], "mount %q should refer to an existing volume", m.Name)
		}

		if c.Name == string(apicommon.SystemProbeContainerName) {
			assert.False(t, mounts[common.DogstatsdSocketVolumeName], "system-probe DogStatsD socket mount should be stripped with its volume")
			assert.True(t, mounts[common.ProcdirVolumeName], "system-probe proc mount should survive for NPM/service discovery")
			assert.True(t, mounts[common.CgroupsVolumeName], "system-probe cgroups mount should survive for NPM/service discovery")
		}
	}
}

func TestGetAutopilotAllowlistVersionAnnotation(t *testing.T) {
	tests := []struct {
		name       string
		annotation string
		want       string
	}{
		{name: "no annotation", annotation: "", want: ""},
		{name: "explicit override", annotation: "v1.2.3", want: "v1.2.3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
			}
			if tt.annotation != "" {
				dda.Annotations[getExperimentalAnnotationKey(ExperimentalAutopilotAllowlistVersionSubkey)] = tt.annotation
			}
			got := getExperimentalAnnotation(dda, ExperimentalAutopilotAllowlistVersionSubkey)
			assert.Equal(t, tt.want, got)
		})
	}
}
