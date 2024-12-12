package otelagent

import (
	"strings"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	v2alpha1test "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1/test"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/otelagent/defaultconfig"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

type expectedPorts struct {
	httpPort int32
	grpcPort int32
}

var (
	defaultExpectedPorts = expectedPorts{
		httpPort: 4318,
		grpcPort: 4317,
	}
	defaultLocalObjectReferenceName = "-otel-agent-config"
)

func Test_otelAgentFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		// disabled
		{
			Name: "otel agent disabled without config",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithOTelAgentEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "otel agent disabled with config",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithOTelAgentEnabled(false).
				WithOTelAgentConfig().
				Build(),
			WantConfigure: false,
		},
		// enabled
		{
			Name: "otel agent enabled with config",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithOTelAgentEnabled(true).
				WithOTelAgentConfig().
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent:                testExpectedAgent(apicommon.OtelAgent, defaultExpectedPorts, defaultLocalObjectReferenceName),
		},
		{
			Name: "otel agent enabled with configMap",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithOTelAgentEnabled(true).
				WithOTelAgentConfigMap().
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent:                testExpectedAgent(apicommon.OtelAgent, defaultExpectedPorts, "user-provided-config-map"),
		},
		{
			Name: "otel agent enabled without config",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithOTelAgentEnabled(true).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent:                testExpectedAgent(apicommon.OtelAgent, defaultExpectedPorts, defaultLocalObjectReferenceName),
		},
		{
			Name: "otel agent enabled without config non default ports",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithOTelAgentEnabled(true).
				WithOTelAgentPorts(4444, 5555).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent: testExpectedAgent(apicommon.OtelAgent, expectedPorts{
				grpcPort: 4444,
				httpPort: 5555,
			},
				defaultLocalObjectReferenceName,
			),
		},
	}

	tests.Run(t, buildOtelAgentFeature)
}

func testExpectedAgent(agentContainerName apicommon.AgentContainerName, expectedPorts expectedPorts, localObjectReferenceName string) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)
			// check volume mounts
			wantVolumeMounts := []corev1.VolumeMount{
				{
					Name:      otelAgentVolumeName,
					MountPath: v2alpha1.ConfigVolumePath + "/" + otelConfigFileName,
					SubPath:   otelConfigFileName,
					ReadOnly:  true,
				},
			}

			agentMounts := mgr.VolumeMountMgr.VolumeMountsByC[agentContainerName]
			assert.True(t, apiutils.IsEqualStruct(agentMounts, wantVolumeMounts), "%s volume mounts \ndiff = %s", agentContainerName, cmp.Diff(agentMounts, wantVolumeMounts))

			// check volumes "otel-agent-config"
			wantVolumes := []corev1.Volume{
				{
					Name: otelAgentVolumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: localObjectReferenceName,
							},
						},
					},
				},
			}

			volumes := mgr.VolumeMgr.Volumes
			assert.True(t, apiutils.IsEqualStruct(volumes, wantVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, wantVolumes))

			// check ports
			wantPorts := []*corev1.ContainerPort{
				{
					Name:          "otel-http",
					ContainerPort: expectedPorts.httpPort,
					HostPort:      expectedPorts.httpPort,
					Protocol:      corev1.ProtocolTCP,
				},
				{
					Name:          "otel-grpc",
					ContainerPort: expectedPorts.grpcPort,
					HostPort:      expectedPorts.grpcPort,
					Protocol:      corev1.ProtocolTCP,
				},
			}

			ports := mgr.PortMgr.PortsByC[agentContainerName]
			assert.Equal(t, wantPorts, ports)
		},
	)
}

func testExpectedDepsCreatedCM(t testing.TB, store store.StoreClient) {
	// hacky to need to hardcode test name but unaware of a better approach that doesn't require
	// modifying WantDependenciesFunc definition.
	if t.Name() == "Test_otelAgentFeature_Configure/otel_agent_enabled_with_configMap" {
		// configMap is provided by user, no need to create it.
		_, found := store.Get(kubernetes.ConfigMapKind, "", "-otel-agent-config")
		assert.False(t, found)
		return
	}
	configMapObject, found := store.Get(kubernetes.ConfigMapKind, "", "-otel-agent-config")
	assert.True(t, found)

	configMap := configMapObject.(*corev1.ConfigMap)
	expectedCM := map[string]string{
		"otel-config.yaml": defaultconfig.DefaultOtelCollectorConfig}

	// validate that default ports were overriden by user provided ports in default config. hacky to need to
	// hardcode test name but unaware of a better approach that doesn't require modifying WantDependenciesFunc definition.
	if t.Name() == "Test_otelAgentFeature_Configure/otel_agent_enabled_without_config_non_default_ports" {
		expectedCM["otel-config.yaml"] = strings.ReplaceAll(expectedCM["otel-config.yaml"], "4317", "4444")
		expectedCM["otel-config.yaml"] = strings.ReplaceAll(expectedCM["otel-config.yaml"], "4318", "5555")
		assert.True(
			t,
			apiutils.IsEqualStruct(configMap.Data, expectedCM),
			"ConfigMap \ndiff = %s", cmp.Diff(configMap.Data, expectedCM),
		)
		return
	}
	assert.True(
		t,
		apiutils.IsEqualStruct(configMap.Data, expectedCM),
		"ConfigMap \ndiff = %s", cmp.Diff(configMap.Data, expectedCM),
	)
}
