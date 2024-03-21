package processdiscovery

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	v2alpha1test "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1/test"
	"github.com/DataDog/datadog-operator/apis/utils"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_processDiscoveryFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		///////////////////////////
		// v2alpha1.DatadogAgent //
		///////////////////////////
		{
			Name: "v2alpha1 process discovery enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithProcessDiscoveryEnabled(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.ProcessAgentContainerName, false),
		},
		{
			Name: "v2alpha1 process discovery disabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithProcessDiscoveryEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "v2alpha1 process discovery config missing",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.ProcessAgentContainerName, false),
		},
		{
			Name: "v2alpha1 process discovery enabled in core agent",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithProcessDiscoveryEnabled(true).
				WithProcessDiscoveryRunInCoreAgent(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.CoreAgentContainerName, true),
		},
		{
			Name: "v2alpha1 process discovery enabled in core agent -- single container",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithProcessDiscoveryEnabled(true).
				WithProcessDiscoveryRunInCoreAgent(true).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.UnprivilegedSingleAgentContainerName, true),
		},
		{
			Name: "v2alpha1 process discovery disabled in core agent -- single container",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithProcessDiscoveryEnabled(true).
				WithProcessDiscoveryRunInCoreAgent(false).
				WithSingleContainerStrategy(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.UnprivilegedSingleAgentContainerName, false),
		},
		{
			Name: "v2alpha1 process discovery disabled in core agent + container collection in core agent",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveContainerCollectionEnabled(true).
				WithLiveContainerRunInCoreAgent(true).
				WithProcessDiscoveryEnabled(true).
				WithProcessDiscoveryRunInCoreAgent(false).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.CoreAgentContainerName, true),
		},
		{
			Name: "v2alpha1 process discovery enabled in core agent + container collection false in core agent",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithProcessDiscoveryEnabled(true).
				WithLiveContainerCollectionEnabled(true).
				WithLiveContainerRunInCoreAgent(false).
				WithProcessDiscoveryRunInCoreAgent(true).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommonv1.ProcessAgentContainerName, false),
		},
		{
			Name: "v2alpha1 process discovery disabled in core agent + process collection in core agent",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithLiveProcessEnabled(true).
				WithLiveProcessRunInCoreAgent(true).
				WithProcessDiscoveryEnabled(true).
				WithProcessDiscoveryRunInCoreAgent(false).
				Build(),
			WantConfigure: false,
			Agent:         testExpectedAgent(apicommonv1.CoreAgentContainerName, true),
		},
	}
	tests.Run(t, buildProcessDiscoveryFeature)
}

func testExpectedAgent(agentContainerName apicommonv1.AgentContainerName, runInCoreAgent bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			// check volume mounts
			wantVolumeMounts := []corev1.VolumeMount{
				{
					Name:      apicommon.PasswdVolumeName,
					MountPath: apicommon.PasswdMountPath,
					ReadOnly:  true,
				},
				{
					Name:      apicommon.CgroupsVolumeName,
					MountPath: apicommon.CgroupsMountPath,
					ReadOnly:  true,
				},
				{
					Name:      apicommon.ProcdirVolumeName,
					MountPath: apicommon.ProcdirMountPath,
					ReadOnly:  true,
				},
			}

			agentMounts := mgr.VolumeMountMgr.VolumeMountsByC[agentContainerName]
			assert.True(t, apiutils.IsEqualStruct(agentMounts, wantVolumeMounts), "%s volume mounts \ndiff = %s", agentContainerName, cmp.Diff(agentMounts, wantVolumeMounts))

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
				{
					Name: apicommon.CgroupsVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: apicommon.CgroupsHostPath,
						},
					},
				},
				{
					Name: apicommon.ProcdirVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: apicommon.ProcdirHostPath,
						},
					},
				},
			}

			volumes := mgr.VolumeMgr.Volumes
			assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))

			// check env vars
			wantEnvVars := []*corev1.EnvVar{
				{
					Name:  apicommon.DDProcessConfigRunInCoreAgent,
					Value: utils.BoolToString(&runInCoreAgent),
				},
				{
					Name:  apicommon.DDProcessDiscoveryEnabled,
					Value: "true",
				},
			}

			agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[agentContainerName]
			assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "%s envvars \ndiff = %s", agentContainerName, cmp.Diff(agentEnvVars, wantEnvVars))
		},
	)
}
