package override

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"

	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	v2alpha1test "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1/test"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	hostCAPath       = "/host/ca/path/ca.crt"
	agentCAPath      = "/agent/ca/path/ca.crt"
	dockerSocketPath = "/docker/socket/path/docker.sock"
)

func TestNodeAgentComponenGlobalSettings(t *testing.T) {
	logger := logf.Log.WithName("TestRequiredComponents")

	testScheme := runtime.NewScheme()
	testScheme.AddKnownTypes(v2alpha1.GroupVersion, &v2alpha1.DatadogAgent{})
	storeOptions := &dependencies.StoreOptions{
		Scheme: testScheme,
	}

	tests := []struct {
		name             string
		dda              *v2alpha1.DatadogAgent
		useMonoContainer bool
		wantVolumeMounts []*corev1.VolumeMount
		wantVolumes      []*corev1.Volume
		wantEnvVars      []*corev1.EnvVar
		want             func(t testing.TB, mgrInterface feature.PodTemplateManagers, expectedEnvVars []*corev1.EnvVar, expectedVolumes []*corev1.Volume, expectedVolumeMounts []*corev1.VolumeMount)
	}{
		{
			name:             "Kubelet volume configured",
			useMonoContainer: false,
			dda: v2alpha1test.NewDatadogAgentBuilder().
				WithGlobalKubeletConfig(hostCAPath, agentCAPath, true).
				WithGlobalDockerSocketPath(dockerSocketPath).
				BuildWithDefaults(),
			wantEnvVars:      getExpectedEnvVars(),
			wantVolumeMounts: getExpectedVolumeMounts(),
			wantVolumes:      getExpectedVolumes(),
			want:             assertAll,
		},
		{
			name:             "Kubelet volume configured",
			useMonoContainer: true,
			dda: v2alpha1test.NewDatadogAgentBuilder().
				WithGlobalKubeletConfig(hostCAPath, agentCAPath, true).
				WithGlobalDockerSocketPath(dockerSocketPath).
				BuildWithDefaults(),
			wantEnvVars:      getExpectedEnvVars(),
			wantVolumeMounts: getExpectedVolumeMounts(),
			wantVolumes:      getExpectedVolumes(),
			want:             assertAllMonoContainer,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			podTemplateManager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
			store := dependencies.NewStore(tt.dda, storeOptions)
			resourcesManager := feature.NewResourceManagers(store)

			ApplyGlobalSettingsMonoSupport(logger, podTemplateManager, tt.dda, resourcesManager, v2alpha1.NodeAgentComponentName, tt.useMonoContainer)

			tt.want(t, podTemplateManager, tt.wantEnvVars, tt.wantVolumes, tt.wantVolumeMounts)
		})
	}
}

func assertAll(t testing.TB, mgrInterface feature.PodTemplateManagers, expectedEnvVars []*corev1.EnvVar, expectedVolumes []*corev1.Volume, expectedVolumeMounts []*corev1.VolumeMount) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)

	coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.CoreAgentContainerName]
	traceAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.TraceAgentContainerName]
	processAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.ProcessAgentContainerName]

	assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, expectedVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, []*corev1.VolumeMount(nil)))
	assert.True(t, apiutils.IsEqualStruct(traceAgentVolumeMounts, expectedVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(traceAgentVolumeMounts, []*corev1.VolumeMount(nil)))
	assert.True(t, apiutils.IsEqualStruct(processAgentVolumeMounts, expectedVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(processAgentVolumeMounts, []*corev1.VolumeMount(nil)))

	volumes := mgr.VolumeMgr.Volumes
	assert.True(t, apiutils.IsEqualStruct(volumes, expectedVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, []*corev1.Volume{}))

	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedEnvVars))
}

func assertAllMonoContainer(t testing.TB, mgrInterface feature.PodTemplateManagers, expectedEnvVars []*corev1.EnvVar, expectedVolumes []*corev1.Volume, expectedVolumeMounts []*corev1.VolumeMount) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)

	monoAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommonv1.NonPrivilegedMultiProcessAgentContainerName]

	assert.True(t, apiutils.IsEqualStruct(monoAgentVolumeMounts, expectedVolumeMounts), "Volume mounts \ndiff = %s", cmp.Diff(monoAgentVolumeMounts, []*corev1.VolumeMount(nil)))

	volumes := mgr.VolumeMgr.Volumes
	assert.True(t, apiutils.IsEqualStruct(volumes, expectedVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, []*corev1.Volume{}))

	agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
	assert.True(t, apiutils.IsEqualStruct(agentEnvVars, expectedEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, expectedEnvVars))
}

func getExpectedEnvVars() []*corev1.EnvVar {
	return []*corev1.EnvVar{
		{
			Name:  apicommon.DDSite,
			Value: "datadoghq.com",
		},
		{
			Name:  apicommon.DDLogLevel,
			Value: "info",
		},
		{
			Name:  apicommon.DDKubeletTLSVerify,
			Value: "true",
		},
		{
			Name:  apicommon.DDKubeletCAPath,
			Value: agentCAPath,
		},
		{
			Name:  apicommon.DockerHost,
			Value: "unix:///host" + dockerSocketPath,
		},
	}
}
func getExpectedVolumes() []*corev1.Volume {
	return []*corev1.Volume{
		{
			Name: apicommon.KubeletCAVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: hostCAPath,
				},
			},
		},
		{
			Name: apicommon.CriSocketVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: dockerSocketPath,
				},
			},
		},
	}
}

func getExpectedVolumeMounts() []*corev1.VolumeMount {
	return []*corev1.VolumeMount{
		{
			Name:      apicommon.KubeletCAVolumeName,
			MountPath: agentCAPath,
			ReadOnly:  true,
		},
		{
			Name:      apicommon.CriSocketVolumeName,
			MountPath: "/host" + dockerSocketPath,
			ReadOnly:  true,
		},
	}
}
