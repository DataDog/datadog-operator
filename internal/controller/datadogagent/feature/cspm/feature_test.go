// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cspm

import (
	"testing"
	"time"

	"k8s.io/utils/ptr"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/defaults"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/providercaps"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_cspmFeature_Configure(t *testing.T) {
	ddaCSPMDisabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				CSPM: &v2alpha1.CSPMFeatureConfig{
					Enabled: ptr.To(false),
				},
			},
		},
	}
	ddaCSPMEnabled := ddaCSPMDisabled.DeepCopy()
	{
		ddaCSPMEnabled.Spec.Features.CSPM.Enabled = ptr.To(true)
		ddaCSPMEnabled.Spec.Features.CSPM.CustomBenchmarks = &v2alpha1.CustomConfig{
			ConfigMap: &v2alpha1.ConfigMapConfig{
				Name: "custom_test",
				Items: []corev1.KeyToPath{
					{
						Key:  "key1",
						Path: "some/path",
					},
				},
			},
		}
		ddaCSPMEnabled.Spec.Features.CSPM.CheckInterval = &metav1.Duration{Duration: 20 * time.Minute}
		ddaCSPMEnabled.Spec.Features.CSPM.HostBenchmarks = &v2alpha1.CSPMHostBenchmarksConfig{Enabled: ptr.To(true)}
		ddaCSPMEnabled.Spec.Features.CSPM.RunInSystemProbe = ptr.To(false)
	}

	ddaCSPMDirectSendEnabled := ddaCSPMDisabled.DeepCopy()
	{
		ddaCSPMDirectSendEnabled.Spec.Features.CSPM.Enabled = ptr.To(true)
		ddaCSPMDirectSendEnabled.Spec.Features.CSPM.CustomBenchmarks = &v2alpha1.CustomConfig{
			ConfigMap: &v2alpha1.ConfigMapConfig{
				Name: "custom_test",
				Items: []corev1.KeyToPath{
					{
						Key:  "key1",
						Path: "some/path",
					},
				},
			},
		}
		ddaCSPMDirectSendEnabled.Spec.Features.CSPM.CheckInterval = &metav1.Duration{Duration: 20 * time.Minute}
		ddaCSPMDirectSendEnabled.Spec.Features.CSPM.HostBenchmarks = &v2alpha1.CSPMHostBenchmarksConfig{Enabled: ptr.To(true)}
		ddaCSPMDirectSendEnabled.Spec.Features.CSPM.RunInSystemProbe = ptr.To(true)
	}

	tests := test.FeatureTestSuite{

		{
			Name:          "CSPM not enabled",
			DDA:           ddaCSPMDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "CSPM enabled with runInSystemProbe disabled",
			DDA:           ddaCSPMEnabled,
			WantConfigure: true,
			ClusterAgent:  cspmClusterAgentWantFunc(),
			Agent:         cspmAgentNodeWantFunc(false),
		},
		{
			Name:          "CSPM enabled with runInSystemProbe",
			DDA:           ddaCSPMDirectSendEnabled,
			WantConfigure: true,
			ClusterAgent:  cspmClusterAgentWantFunc(),
			Agent:         cspmAgentNodeWantFunc(true),
		},
	}

	tests.Run(t, buildCSPMFeature)
}

func cspmClusterAgentWantFunc() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)
			dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]

			want := []*corev1.EnvVar{
				{
					Name:  DDComplianceConfigEnabled,
					Value: "true",
				},
				{
					Name:  DDComplianceConfigCheckInterval,
					Value: "1200000000000",
				},
			}
			assert.True(t, apiutils.IsEqualStruct(dcaEnvVars, want), "DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want))

			wantVolumeMounts := []corev1.VolumeMount{
				{
					Name:      cspmConfigVolumeName,
					MountPath: "/etc/datadog-agent/compliance.d/some/path",
					SubPath:   "some/path",
					ReadOnly:  true,
				},
			}

			volumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.ClusterAgentContainerName]
			assert.True(t, apiutils.IsEqualStruct(volumeMounts, wantVolumeMounts), "Cluster Agent volume mounts \ndiff = %s", cmp.Diff(volumeMounts, wantVolumeMounts))

			wantVolumes := []corev1.Volume{
				{
					Name: cspmConfigVolumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "custom_test",
							},
							Items: []corev1.KeyToPath{{Key: "key1", Path: "some/path"}},
						},
					},
				},
			}
			volumes := mgr.VolumeMgr.Volumes
			assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Cluster Agent volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))

			annotations := mgr.AnnotationMgr.Annotations
			assert.Empty(t, annotations)

		},
	)
}

func cspmAgentNodeWantFunc(runInSystemProbe bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			// Determine which container to check based on runInSystemProbe
			targetContainer := apicommon.SecurityAgentContainerName
			if runInSystemProbe {
				targetContainer = apicommon.SystemProbeContainerName
			}

			// The compliance settings are also added to the Core Agent container so that its
			// config snapshot carries them when config streaming is enabled.
			wantCoreAgent := []*corev1.EnvVar{
				{
					Name:  DDComplianceConfigEnabled,
					Value: "true",
				},
				{
					Name:  DDComplianceConfigCheckInterval,
					Value: "1200000000000",
				},
				{
					Name:  DDComplianceHostBenchmarksEnabled,
					Value: "true",
				},
				{
					Name:  DDComplianceConfigRunInSystemProbe,
					Value: apiutils.BoolToString(&runInSystemProbe),
				},
			}

			if runInSystemProbe {
				// config sync is served by the core agent, so it needs the same env vars
				wantCoreAgent = append(wantCoreAgent, configSyncEnvVars()...)
			}

			coreAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
			assert.True(t, apiutils.IsEqualStruct(coreAgentEnvVars, wantCoreAgent), "Core Agent envvars \ndiff = %s", cmp.Diff(coreAgentEnvVars, wantCoreAgent))

			// HOST_ROOT is only set on the container running the checks, since it is where the
			// host root volume is mounted.
			wantTargetContainer := []*corev1.EnvVar{
				{
					Name:  DDComplianceConfigEnabled,
					Value: "true",
				},
				{
					Name:  common.DDHostRootEnvVar,
					Value: common.HostRootMountPath,
				},
				{
					Name:  DDComplianceConfigCheckInterval,
					Value: "1200000000000",
				},
				{
					Name:  DDComplianceHostBenchmarksEnabled,
					Value: "true",
				},
				{
					Name:  DDComplianceConfigRunInSystemProbe,
					Value: apiutils.BoolToString(&runInSystemProbe),
				},
			}

			if runInSystemProbe {
				wantTargetContainer = append(wantTargetContainer, configSyncEnvVars()...)
			}

			targetContainerEnvVars := mgr.EnvVarMgr.EnvVarsByC[targetContainer]
			assert.True(t, apiutils.IsEqualStruct(targetContainerEnvVars, wantTargetContainer), "Agent envvars \ndiff = %s", cmp.Diff(targetContainerEnvVars, wantTargetContainer))

			// check volume mounts
			wantVolumeMounts := []corev1.VolumeMount{
				{
					Name:      securityAgentComplianceConfigDirVolumeName,
					MountPath: "/etc/datadog-agent/compliance.d",
					ReadOnly:  true,
				},
				{
					Name:      common.CgroupsVolumeName,
					MountPath: common.CgroupsMountPath,
					ReadOnly:  true,
				},
				{
					Name:      common.PasswdVolumeName,
					MountPath: common.PasswdMountPath,
					ReadOnly:  true,
				},
				{
					Name:      common.ProcdirVolumeName,
					MountPath: common.ProcdirMountPath,
					ReadOnly:  true,
				},
				{
					Name:      common.HostRootVolumeName,
					MountPath: common.HostRootMountPath,
					ReadOnly:  true,
				},
				{
					Name:      common.GroupVolumeName,
					MountPath: common.GroupMountPath,
					ReadOnly:  true,
				},
			}

			targetContainerVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[targetContainer]
			assert.True(t, apiutils.IsEqualStruct(targetContainerVolumeMounts, wantVolumeMounts), "Target container volume mounts \ndiff = %s", cmp.Diff(targetContainerVolumeMounts, wantVolumeMounts))

			// check volumes
			wantVolumes := []corev1.Volume{
				{
					Name: cspmConfigVolumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "custom_test",
							},
							Items: []corev1.KeyToPath{{Key: "key1", Path: "some/path"}},
						},
					},
				},
				{
					Name: securityAgentComplianceConfigDirVolumeName,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
				{
					Name: common.CgroupsVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: common.CgroupsHostPath,
						},
					},
				},
				{
					Name: common.PasswdVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: common.PasswdHostPath,
						},
					},
				},
				{
					Name: common.ProcdirVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: common.ProcdirHostPath,
						},
					},
				},
				{
					Name: common.HostRootVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: common.HostRootHostPath,
						},
					},
				},
				{
					Name: common.GroupVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: common.GroupHostPath,
						},
					},
				},
			}

			volumes := mgr.VolumeMgr.Volumes
			assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))

			annotations := mgr.AnnotationMgr.Annotations
			assert.Empty(t, annotations)
		},
	)
}

func Test_cspmFeature_NodeAgentProviderCapabilities(t *testing.T) {
	newPodTemplate := func() *corev1.PodTemplateSpec {
		return &corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: string(apicommon.SecurityAgentContainerName)},
					{Name: string(apicommon.SystemProbeContainerName)},
				},
			},
		}
	}

	volumeNames := func(tmpl *corev1.PodTemplateSpec) []string {
		names := make([]string, 0, len(tmpl.Spec.Volumes))
		for _, v := range tmpl.Spec.Volumes {
			names = append(names, v.Name)
		}
		return names
	}

	t.Run("talos strips passwd and group volumes", func(t *testing.T) {
		f := &cspmFeature{}
		tmpl := newPodTemplate()
		mgr := feature.NewPodTemplateManagers(tmpl)
		require.NoError(t, f.ManageNodeAgent(mgr))

		providercaps.ApplyProviderCapabilities(mgr, kubernetes.TalosProvider, f.NodeAgentProviderCapabilities())

		assert.NotContains(t, volumeNames(tmpl), common.PasswdVolumeName)
		assert.NotContains(t, volumeNames(tmpl), common.GroupVolumeName)
	})

	t.Run("default provider keeps passwd and group volumes", func(t *testing.T) {
		f := &cspmFeature{}
		tmpl := newPodTemplate()
		mgr := feature.NewPodTemplateManagers(tmpl)
		require.NoError(t, f.ManageNodeAgent(mgr))

		providercaps.ApplyProviderCapabilities(mgr, kubernetes.DefaultProvider, f.NodeAgentProviderCapabilities())

		assert.Contains(t, volumeNames(tmpl), common.PasswdVolumeName)
		assert.Contains(t, volumeNames(tmpl), common.GroupVolumeName)
	})
}

// configSyncEnvVars are the env vars the system-probe needs when it runs the compliance checks
// and submits the payloads itself: it cannot resolve secret handles, so the resolved api_key has
// to come from the core agent.
func configSyncEnvVars() []*corev1.EnvVar {
	return []*corev1.EnvVar{
		{Name: common.DDAgentIpcPort, Value: featureutils.DefaultAgentIpcPort},
		{Name: common.DDAgentIpcConfigRefreshInterval, Value: featureutils.DefaultAgentIpcConfigRefreshInterval},
	}
}

// Remote Configuration can enable CSPM after the defaulting pass has run. RunInSystemProbe is
// defaulted in Configure, after the merge, so a remotely enabled CSPM runs in the system-probe too.
func Test_cspmFeature_ConfigureFromRemoteConfig(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{
		Status: v2alpha1.DatadogAgentStatus{
			RemoteConfigConfiguration: &v2alpha1.RemoteConfigConfiguration{
				Features: &v2alpha1.DatadogFeatures{
					CSPM: &v2alpha1.CSPMFeatureConfig{Enabled: ptr.To(true)},
				},
			},
		},
	}
	defaults.DefaultDatadogAgentSpec(&dda.Spec)
	require.False(t, *dda.Spec.Features.CSPM.Enabled, "CSPM is off in the spec before the merge")

	f := &cspmFeature{}
	reqComp := f.Configure(dda, &dda.Spec, dda.Status.RemoteConfigConfiguration)

	assert.True(t, f.runInSystemProbe)
	assert.Equal(t, []apicommon.AgentContainerName{apicommon.SystemProbeContainerName}, reqComp.Agent.Containers)
}

func Test_cspmFeature_RunInSystemProbeDefault(t *testing.T) {
	tests := []struct {
		name             string
		agentTag         string
		runInSystemProbe *bool
		want             bool
	}{
		{name: "unset, no Agent image override", want: true},
		{name: "unset, Agent too old", agentTag: "7.76.0", want: false},
		{name: "unset, Agent recent enough", agentTag: "7.77.0", want: true},
		{name: "unset, unparsable Agent tag is assumed recent enough", agentTag: "latest", want: true},
		{name: "explicit false wins", runInSystemProbe: ptr.To(false), want: false},
		{name: "explicit true wins on an older Agent", agentTag: "7.76.0", runInSystemProbe: ptr.To(true), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Features: &v2alpha1.DatadogFeatures{
						CSPM: &v2alpha1.CSPMFeatureConfig{Enabled: ptr.To(true), RunInSystemProbe: tt.runInSystemProbe},
					},
				},
			}
			if tt.agentTag != "" {
				dda.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {Image: &v2alpha1.AgentImageConfig{Tag: tt.agentTag}},
				}
			}

			f := &cspmFeature{}
			f.Configure(dda, &dda.Spec, nil)

			assert.Equal(t, tt.want, f.runInSystemProbe)
			// the system-probe seccomp profile reads the value from the spec
			require.NotNil(t, dda.Spec.Features.CSPM.RunInSystemProbe)
			assert.Equal(t, tt.want, *dda.Spec.Features.CSPM.RunInSystemProbe)
		})
	}
}
