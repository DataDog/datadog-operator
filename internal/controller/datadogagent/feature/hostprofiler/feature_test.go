package hostprofiler

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

var (
	tracingfsVolumeMount = corev1.VolumeMount{
		Name:      "tracingfs",
		MountPath: "/sys/kernel/tracing",
		ReadOnly:  true,
	}
	defaultVolumeMounts = []corev1.VolumeMount{tracingfsVolumeMount}
	tracingfsVolume     = corev1.Volume{
		Name: "tracingfs",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/sys/kernel/tracing",
			},
		},
	}
	defaultVolumes = []corev1.Volume{tracingfsVolume}
	wantIpcEnvVars = []*corev1.EnvVar{
		{Name: common.DDAgentIpcPort, Value: "5009"},
		{Name: common.DDAgentIpcConfigRefreshInterval, Value: "60"},
	}
)

func Test_hostProfilerFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		// disabled
		{
			Name: "host profiler disabled without config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAnnotations(map[string]string{"agent.datadoghq.com/host-profiler-enabled": "false"}).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "host profiler enabled via annotation",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAnnotations(map[string]string{"agent.datadoghq.com/host-profiler-enabled": "true"}).
				Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.HostProfiler, defaultVolumeMounts, defaultVolumes),
		},
		{
			Name:          "host profiler enabled via CRD",
			DDA:           testutils.NewDatadogAgentBuilder().WithHostProfilerEnabled(true).Build(),
			WantConfigure: true,
			Agent:         testExpectedAgent(apicommon.HostProfiler, defaultVolumeMounts, defaultVolumes),
		},
		{
			Name:          "host profiler disabled via CRD",
			DDA:           testutils.NewDatadogAgentBuilder().WithHostProfilerEnabled(false).Build(),
			WantConfigure: false,
		},
		{
			Name: "host profiler CRD takes precedence over annotation",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAnnotations(map[string]string{"agent.datadoghq.com/host-profiler-enabled": "true"}).
				WithHostProfilerEnabled(false).
				Build(),
			WantConfigure: false,
		},
	}
	tests.Run(t, buildHostProfilerFeature)
}

func testExpectedAgent(
	agentContainerName apicommon.AgentContainerName,
	expectedVolumeMount []corev1.VolumeMount,
	expectedVolume []corev1.Volume) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentMounts := mgr.VolumeMountMgr.VolumeMountsByC[agentContainerName]
			assert.True(t, apiutils.IsEqualStruct(agentMounts, expectedVolumeMount), "%s volume mounts \ndiff = %s", agentContainerName, cmp.Diff(agentMounts, expectedVolumeMount))

			volumes := mgr.VolumeMgr.Volumes
			assert.True(t, apiutils.IsEqualStruct(volumes, expectedVolume), "Volumes \ndiff = %s", cmp.Diff(volumes, expectedVolume))

			assert.Equal(t, true, mgr.Tpl.Spec.HostPID)

			// IPC env vars
			coreEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
			assert.True(t, apiutils.IsEqualStruct(coreEnvVars, wantIpcEnvVars), "Core agent IPC env vars \ndiff = %s", cmp.Diff(coreEnvVars, wantIpcEnvVars))

			hostProfilerEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.HostProfiler]
			assert.True(t, apiutils.IsEqualStruct(hostProfilerEnvVars, wantIpcEnvVars), "HostProfiler IPC env vars \ndiff = %s", cmp.Diff(hostProfilerEnvVars, wantIpcEnvVars))
		},
	)
}
