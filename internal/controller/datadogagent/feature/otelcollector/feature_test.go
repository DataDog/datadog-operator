package otelcollector

import (
	"strings"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	v2alpha1test "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1/test"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/otelcollector/defaultconfig"
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

type expectedEnvVars struct {
	enabled           expectedEnvVar
	extension_timeout expectedEnvVar
	extension_url     expectedEnvVar
}

type expectedEnvVar struct {
	present bool
	value   string
}

var (
	defaultExpectedPorts = expectedPorts{
		httpPort: 4318,
		grpcPort: 4317,
	}
	defaultLocalObjectReferenceName = "-otel-agent-config"
	defaultExpectedEnvVars          = expectedEnvVars{
		enabled: expectedEnvVar{
			present: true,
			value:   "true",
		},
		extension_timeout: expectedEnvVar{},
		extension_url:     expectedEnvVar{},
	}
)

func Test_otelCollectorFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		// disabled
		{
			Name: "otel agent disabled without config",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "otel agent disabled with config",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(false).
				WithOTelCollectorConfig().
				Build(),
			WantConfigure: false,
		},
		// enabled
		{
			Name: "otel agent enabled with config",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorConfig().
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent:                testExpectedAgent(apicommon.OtelAgent, defaultExpectedPorts, defaultLocalObjectReferenceName, defaultExpectedEnvVars),
		},
		{
			Name: "otel agent enabled with configMap",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorConfigMap().
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent:                testExpectedAgent(apicommon.OtelAgent, defaultExpectedPorts, "user-provided-config-map", defaultExpectedEnvVars),
		},
		{
			Name: "otel agent enabled without config",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent:                testExpectedAgent(apicommon.OtelAgent, defaultExpectedPorts, defaultLocalObjectReferenceName, defaultExpectedEnvVars),
		},
		{
			Name: "otel agent enabled without config non default ports",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorPorts(4444, 5555).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent: testExpectedAgent(apicommon.OtelAgent, expectedPorts{
				grpcPort: 4444,
				httpPort: 5555,
			},
				defaultLocalObjectReferenceName,
				defaultExpectedEnvVars,
			),
		},
		// coreconfig
		{
			Name: "otel agent coreconfig enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorCoreConfigEnabled(true).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent:                testExpectedAgent(apicommon.OtelAgent, defaultExpectedPorts, defaultLocalObjectReferenceName, defaultExpectedEnvVars),
		},
		{
			Name: "otel agent coreconfig disabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorCoreConfigEnabled(false).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent:                testExpectedAgent(apicommon.OtelAgent, defaultExpectedPorts, defaultLocalObjectReferenceName, expectedEnvVars{}),
		},
		{
			Name: "otel agent coreconfig extensionTimeout",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorCoreConfigEnabled(false).
				WithOTelCollectorCoreConfigExtensionTimeout(13).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent: testExpectedAgent(apicommon.OtelAgent, defaultExpectedPorts, defaultLocalObjectReferenceName, expectedEnvVars{
				extension_timeout: expectedEnvVar{
					present: true,
					value:   "13",
				},
			}),
		},
		{
			Name: "otel agent coreconfig extensionURL",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorCoreConfigEnabled(false).
				WithOTelCollectorCoreConfigExtensionURL("https://localhost:1234").
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent: testExpectedAgent(apicommon.OtelAgent, defaultExpectedPorts, defaultLocalObjectReferenceName, expectedEnvVars{
				extension_url: expectedEnvVar{
					present: true,
					value:   "https://localhost:1234",
				},
			}),
		},
		{
			Name: "otel agent coreconfig all env vars",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorCoreConfigEnabled(true).
				WithOTelCollectorCoreConfigExtensionTimeout(13).
				WithOTelCollectorCoreConfigExtensionURL("https://localhost:1234").
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent: testExpectedAgent(apicommon.OtelAgent, defaultExpectedPorts, defaultLocalObjectReferenceName, expectedEnvVars{
				extension_url: expectedEnvVar{
					present: true,
					value:   "https://localhost:1234",
				},
				extension_timeout: expectedEnvVar{
					present: true,
					value:   "13",
				},
				enabled: expectedEnvVar{
					present: true,
					value:   "true",
				},
			}),
		},
	}
	tests.Run(t, buildOtelCollectorFeature)
}

func testExpectedAgent(agentContainerName apicommon.AgentContainerName, expectedPorts expectedPorts, localObjectReferenceName string, expectedEnvVars expectedEnvVars) *test.ComponentTest {
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

			// check env vars
			wantEnvVars := []*corev1.EnvVar{}

			if expectedEnvVars.enabled.present {
				wantEnvVars = append(wantEnvVars, &corev1.EnvVar{
					Name:  v2alpha1.DDOtelCollectorCoreConfigEnabled,
					Value: expectedEnvVars.enabled.value,
				})
			}

			if expectedEnvVars.extension_timeout.present {
				wantEnvVars = append(wantEnvVars, &corev1.EnvVar{
					Name:  v2alpha1.DDOtelCollectorCoreConfigExtensionTimeout,
					Value: expectedEnvVars.extension_timeout.value,
				})
			}

			if expectedEnvVars.extension_url.present {
				wantEnvVars = append(wantEnvVars, &corev1.EnvVar{
					Name:  v2alpha1.DDOtelCollectorCoreConfigExtensionURL,
					Value: expectedEnvVars.extension_url.value,
				})
			}

			if len(wantEnvVars) == 0 {
				wantEnvVars = nil
			}

			agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
			assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantEnvVars))

		},
	)
}

func testExpectedDepsCreatedCM(t testing.TB, store store.StoreClient) {
	// hacky to need to hardcode test name but unaware of a better approach that doesn't require
	// modifying WantDependenciesFunc definition.
	if t.Name() == "Test_otelCollectorFeature_Configure/otel_agent_enabled_with_configMap" {
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
	if t.Name() == "Test_otelCollectorFeature_Configure/otel_agent_enabled_without_config_non_default_ports" {
		expectedCM["otel-config.yaml"] = strings.Replace(expectedCM["otel-config.yaml"], "4317", "4444", 1)
		expectedCM["otel-config.yaml"] = strings.Replace(expectedCM["otel-config.yaml"], "4318", "5555", 1)
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