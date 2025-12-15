package hostprofiler

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/hostprofiler/defaultconfig"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

var (
	defaultLocalObjectReferenceName = "-host-profiler-config"
	tracingfsVolumeMount            = corev1.VolumeMount{
		Name:      "tracingfs",
		MountPath: "/sys/kernel/tracing",
		ReadOnly:  true,
	}
	defaultVolumeMounts = []corev1.VolumeMount{
		tracingfsVolumeMount,
		{
			Name:      hostProfilerVolumeName,
			MountPath: common.ConfigVolumePath + "/" + hostProfilerConfigFileName,
			SubPath:   hostProfilerConfigFileName,
			ReadOnly:  true,
		},
	}
	tracingfsVolume = corev1.Volume{
		Name: "tracingfs",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/sys/kernel/tracing",
			},
		},
	}
	defaultVolumes = func(objectName string) []corev1.Volume {
		return []corev1.Volume{
			tracingfsVolume,
			{
				Name: hostProfilerVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: objectName,
						},
					},
				},
			},
		}
	}
)

var defaultAnnotations = map[string]string{"checksum/host_profiler-custom-config": "7b48d4d7ca198be0a6d7d8c7a5ad5535"}

func Test_hostProfilerFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		// disabled
		{
			Name: "host profiler disabled without config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithHostProfilerEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "host profiler disabled with config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithHostProfilerEnabled(false).
				WithHostProfilerConfig().
				Build(),
			WantConfigure: false,
		},
		// enabled
		{
			Name: "host profiler enabled with config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithHostProfilerEnabled(true).
				WithHostProfilerConfig().
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent:                testExpectedAgent(apicommon.HostProfiler, defaultAnnotations, defaultVolumeMounts, defaultVolumes(defaultLocalObjectReferenceName)),
		},
		{
			Name: "host profiler enabled with configMap",
			DDA: testutils.NewDatadogAgentBuilder().
				WithHostProfilerEnabled(true).
				WithHostProfilerConfigMap().
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent:                testExpectedAgent(apicommon.HostProfiler, map[string]string{}, defaultVolumeMounts, defaultVolumes("user-provided-config-map")),
		},
		{
			Name: "host profiler enabled with configMap multi items",
			DDA: testutils.NewDatadogAgentBuilder().
				WithHostProfilerEnabled(true).
				WithHostProfilerConfigMapMultipleItems().
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent: testExpectedAgent(apicommon.HostProfiler, map[string]string{}, []corev1.VolumeMount{
				tracingfsVolumeMount,
				{
					Name:      hostProfilerVolumeName,
					MountPath: common.ConfigVolumePath + "/otel/",
				},
			},
				[]corev1.Volume{
					tracingfsVolume,
					{
						Name: hostProfilerVolumeName,
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "user-provided-config-map",
								},
								Items: []corev1.KeyToPath{
									{
										Key:  "otel-config.yaml",
										Path: "otel-config.yaml",
									},
									{
										Key:  "otel-config-two.yaml",
										Path: "otel-config-two.yaml",
									},
								},
							},
						},
					},
				}),
		},
		{
			Name: "host profiler enabled without config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithHostProfilerEnabled(true).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent:                testExpectedAgent(apicommon.HostProfiler, defaultAnnotations, defaultVolumeMounts, defaultVolumes(defaultLocalObjectReferenceName)),
		},
	}
	tests.Run(t, buildHostProfilerFeature)
}

func testExpectedAgent(
	agentContainerName apicommon.AgentContainerName,
	expectedAnnotations map[string]string,
	expectedVolumeMount []corev1.VolumeMount,
	expectedVolume []corev1.Volume) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentMounts := mgr.VolumeMountMgr.VolumeMountsByC[agentContainerName]
			assert.True(t, apiutils.IsEqualStruct(agentMounts, expectedVolumeMount), "%s volume mounts \ndiff = %s", agentContainerName, cmp.Diff(agentMounts, expectedVolumeMount))

			volumes := mgr.VolumeMgr.Volumes
			assert.True(t, apiutils.IsEqualStruct(volumes, expectedVolume), "Volumes \ndiff = %s", cmp.Diff(volumes, expectedVolume))

			// annotations
			agentAnnotations := mgr.AnnotationMgr.Annotations
			assert.Equal(t, expectedAnnotations, agentAnnotations)
		},
	)
}

func testExpectedDepsCreatedCM(t testing.TB, store store.StoreClient) {
	// hacky to need to hardcode test name but unaware of a better approach that doesn't require
	// modifying WantDependenciesFunc definition.
	if t.Name() == "Test_hostProfilerFeature_Configure/host_profiler_enabled_with_configMap" {
		// configMap is provided by user, no need to create it.
		_, found := store.Get(kubernetes.ConfigMapKind, "", "-host-profiler-config")
		assert.False(t, found)
		return
	}
	if t.Name() == "Test_hostProfilerFeature_Configure/host_profiler_enabled_with_configMap_multi_items" {
		// configMap is provided by user, no need to create it.
		_, found := store.Get(kubernetes.ConfigMapKind, "", "-host-profiler-config")
		assert.False(t, found)
		return
	}
	configMapObject, found := store.Get(kubernetes.ConfigMapKind, "", "-host-profiler-config")
	assert.True(t, found)

	configMap := configMapObject.(*corev1.ConfigMap)
	expectedCM := map[string]string{
		"host-profiler-config.yaml": defaultconfig.DefaultHostProfilerConfig}

	assert.True(
		t,
		apiutils.IsEqualStruct(configMap.Data, expectedCM),
		"ConfigMap \ndiff = %s", cmp.Diff(configMap.Data, expectedCM),
	)
}
