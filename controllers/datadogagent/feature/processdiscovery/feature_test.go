package processdiscovery

import (
	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"testing"
)

func Test_processDiscoveryFeature_Configure(t *testing.T) {
	processDiscoveryWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		// check volume mounts
		wantVolumeMounts := []corev1.VolumeMount{
			{
				Name:      apicommon.PasswdVolumeName,
				MountPath: apicommon.PasswdMountPath,
				ReadOnly:  true,
			},
		}

		processAgentMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.ProcessAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(processAgentMounts, wantVolumeMounts), "Process Agent volume mounts \ndiff = %s", cmp.Diff(processAgentMounts, wantVolumeMounts))

		// check volumes
		wantVolumes := []corev1.Volume{
			{
				Name: apicommon.PasswdVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: apicommon.PasswdHostPath,
					},
				},
			},
		}

		volumes := mgr.VolumeMgr.Volumes
		assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))

		// check env vars
		wantEnvVars := []*corev1.EnvVar{
			{
				Name:  apicommon.DDProcessDiscoveryEnabled,
				Value: "true",
			},
		}

		processAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ProcessAgentContainerName]
		assert.True(t, apiutils.IsEqualStruct(processAgentEnvVars, wantEnvVars), "Process Agent envvars \ndiff = %s", cmp.Diff(processAgentEnvVars, wantEnvVars))
	}
	tests := test.FeatureTestSuite{
		///////////////////////////
		// v2alpha1.DatadogAgent //
		///////////////////////////
		{
			Name: "v2alpha1 process discovery enabled",
			DDAv2: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Features: &v2alpha1.DatadogFeatures{
						ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
							Enabled: apiutils.NewBoolPointer(true),
						},
					},
				},
			},
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(processDiscoveryWantFunc),
		},
		{
			Name: "v2alpha1 process discovery disabled",
			DDAv2: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Features: &v2alpha1.DatadogFeatures{
						ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
							Enabled: apiutils.NewBoolPointer(false),
						},
					},
				},
			},
			WantConfigure: false,
		},
		{
			Name: "v2alpha1 process discovery config missing",
			DDAv2: &v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Features: &v2alpha1.DatadogFeatures{
						ProcessDiscovery: nil,
					},
				},
			},
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(processDiscoveryWantFunc),
		},
	}
	tests.Run(t, buildProcessDiscoveryFeature)
}
