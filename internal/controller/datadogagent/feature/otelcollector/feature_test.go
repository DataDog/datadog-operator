package otelcollector

import (
	"strings"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/otelcollector/defaultconfig"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/version"
)

type expectedPorts struct {
	httpPort int32
	grpcPort int32
}

type expectedEnvVars struct {
	agent_ipc_port     expectedEnvVar
	agent_ipc_refresh  expectedEnvVar
	enabled            expectedEnvVar
	extension_timeout  expectedEnvVar
	extension_url      expectedEnvVar
	converter_features expectedEnvVar
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
		agent_ipc_port: expectedEnvVar{
			present: true,
			value:   "5009",
		},
		agent_ipc_refresh: expectedEnvVar{
			present: true,
			value:   "60",
		},
		enabled: expectedEnvVar{
			present: true,
			value:   "true",
		},
		extension_timeout:  expectedEnvVar{},
		extension_url:      expectedEnvVar{},
		converter_features: expectedEnvVar{},
	}

	onlyIpcEnvVars = expectedEnvVars{
		agent_ipc_port: expectedEnvVar{
			present: true,
			value:   "5009",
		},
		agent_ipc_refresh: expectedEnvVar{
			present: true,
			value:   "60",
		},
	}
	defaultVolumeMounts = []corev1.VolumeMount{
		{
			Name:      otelAgentVolumeName,
			MountPath: common.ConfigVolumePath + "/" + otelConfigFileName,
			SubPath:   otelConfigFileName,
			ReadOnly:  true,
		},
	}
	defaultVolumes = func(objectName string) []corev1.Volume {
		return []corev1.Volume{
			{
				Name: otelAgentVolumeName,
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

var defaultAnnotations = map[string]string{"checksum/otel_agent-custom-config": "8e715f9526c27c6cd06ba9a9d8913451"}

func Test_otelCollectorFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		// disabled
		{
			Name: "otel agent disabled without config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "otel agent disabled with config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(false).
				WithOTelCollectorConfig().
				Build(),
			WantConfigure: false,
		},
		// enabled
		{
			Name: "otel agent enabled with config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorConfig().
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent:                testExpectedAgent(apicommon.OtelAgent, defaultExpectedPorts, defaultExpectedEnvVars, defaultAnnotations, defaultVolumeMounts, defaultVolumes(defaultLocalObjectReferenceName)),
		},
		{
			Name: "otel agent enabled with configMap",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorConfigMap().
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent:                testExpectedAgent(apicommon.OtelAgent, defaultExpectedPorts, defaultExpectedEnvVars, map[string]string{}, defaultVolumeMounts, defaultVolumes("user-provided-config-map")),
		},
		{
			Name: "otel agent enabled with configMap multi items",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorConfigMapMultipleItems().
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent: testExpectedAgent(apicommon.OtelAgent, defaultExpectedPorts, defaultExpectedEnvVars, map[string]string{}, []corev1.VolumeMount{
				{
					Name:      otelAgentVolumeName,
					MountPath: common.ConfigVolumePath + "/otel/",
				},
			},
				[]corev1.Volume{
					{
						Name: otelAgentVolumeName,
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
			Name: "otel agent enabled without config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent:                testExpectedAgent(apicommon.OtelAgent, defaultExpectedPorts, defaultExpectedEnvVars, defaultAnnotations, defaultVolumeMounts, defaultVolumes(defaultLocalObjectReferenceName)),
		},
		{
			Name: "otel agent enabled without config non default ports",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorPorts(4444, 5555).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent: testExpectedAgent(apicommon.OtelAgent, expectedPorts{
				grpcPort: 4444,
				httpPort: 5555,
			},
				defaultExpectedEnvVars,
				map[string]string{"checksum/otel_agent-custom-config": "1b4f73fd3576db6a939bbfe788cc1f80"},
				defaultVolumeMounts,
				defaultVolumes(defaultLocalObjectReferenceName),
			),
		},
		// gateway enabled
		{
			Name: "otel agent with gateway enabled default ports",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelAgentGatewayEnabled(true).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent: testExpectedAgent(apicommon.OtelAgent, defaultExpectedPorts,
				expectedEnvVars{
					agent_ipc_port: expectedEnvVar{
						present: true,
						value:   "5009",
					},
					agent_ipc_refresh: expectedEnvVar{
						present: true,
						value:   "60",
					},
					enabled: expectedEnvVar{
						present: true,
						value:   "true",
					},
					extension_timeout: expectedEnvVar{},
					extension_url:     expectedEnvVar{},
					converter_features: expectedEnvVar{
						present: true,
						value:   "health_check,zpages,pprof,ddflare",
					},
				},
				map[string]string{"checksum/otel_agent-custom-config": "b4ea5ecc5c7901d3b48c58622379ecfb"},
				defaultVolumeMounts,
				defaultVolumes(defaultLocalObjectReferenceName),
			),
		},
		{
			Name: "otel agent with gateway enabled non default ports",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelAgentGatewayEnabled(true).
				WithOTelCollectorPorts(4444, 5555).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent: testExpectedAgent(apicommon.OtelAgent, expectedPorts{
				grpcPort: 4444,
				httpPort: 5555,
			},
				expectedEnvVars{
					agent_ipc_port: expectedEnvVar{
						present: true,
						value:   "5009",
					},
					agent_ipc_refresh: expectedEnvVar{
						present: true,
						value:   "60",
					},
					enabled: expectedEnvVar{
						present: true,
						value:   "true",
					},
					extension_timeout: expectedEnvVar{},
					extension_url:     expectedEnvVar{},
					converter_features: expectedEnvVar{
						present: true,
						value:   "health_check,zpages,pprof,ddflare",
					},
				},
				map[string]string{"checksum/otel_agent-custom-config": "d9c73c9017a4fcb811da0e51f5044b3c"},
				defaultVolumeMounts,
				defaultVolumes(defaultLocalObjectReferenceName),
			),
		},
		// coreconfig
		{
			Name: "otel agent coreconfig enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorCoreConfigEnabled(true).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent:                testExpectedAgent(apicommon.OtelAgent, defaultExpectedPorts, defaultExpectedEnvVars, defaultAnnotations, defaultVolumeMounts, defaultVolumes(defaultLocalObjectReferenceName)),
		},
		{
			Name: "otel agent coreconfig disabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorCoreConfigEnabled(false).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent:                testExpectedAgent(apicommon.OtelAgent, defaultExpectedPorts, onlyIpcEnvVars, defaultAnnotations, defaultVolumeMounts, defaultVolumes(defaultLocalObjectReferenceName)),
		},
		{
			Name: "otel agent coreconfig extensionTimeout",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorCoreConfigEnabled(false).
				WithOTelCollectorCoreConfigExtensionTimeout(13).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent: testExpectedAgent(apicommon.OtelAgent, defaultExpectedPorts, expectedEnvVars{
				agent_ipc_port: expectedEnvVar{
					present: true,
					value:   "5009",
				},
				agent_ipc_refresh: expectedEnvVar{
					present: true,
					value:   "60",
				},
				extension_timeout: expectedEnvVar{
					present: true,
					value:   "13",
				},
			},
				defaultAnnotations,
				defaultVolumeMounts,
				defaultVolumes(defaultLocalObjectReferenceName)),
		},
		{
			Name: "otel agent coreconfig extensionURL",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorCoreConfigEnabled(false).
				WithOTelCollectorCoreConfigExtensionURL("https://localhost:1234").
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent: testExpectedAgent(apicommon.OtelAgent, defaultExpectedPorts, expectedEnvVars{
				agent_ipc_port: expectedEnvVar{
					present: true,
					value:   "5009",
				},
				agent_ipc_refresh: expectedEnvVar{
					present: true,
					value:   "60",
				},
				extension_url: expectedEnvVar{
					present: true,
					value:   "https://localhost:1234",
				},
			},
				defaultAnnotations,
				defaultVolumeMounts,
				defaultVolumes(defaultLocalObjectReferenceName)),
		},
		{
			Name: "otel agent coreconfig all env vars",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorCoreConfigEnabled(true).
				WithOTelCollectorCoreConfigExtensionTimeout(13).
				WithOTelCollectorCoreConfigExtensionURL("https://localhost:1234").
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent: testExpectedAgent(apicommon.OtelAgent, defaultExpectedPorts, expectedEnvVars{
				agent_ipc_port: expectedEnvVar{
					present: true,
					value:   "5009",
				},
				agent_ipc_refresh: expectedEnvVar{
					present: true,
					value:   "60",
				},
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
			},
				defaultAnnotations,
				defaultVolumeMounts,
				defaultVolumes(defaultLocalObjectReferenceName)),
		},
		{
			Name: "otel agent enabled with service ports default",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorConfig().
				Build(),
			WantConfigure: true,
			StoreOption: &store.StoreOptions{
				PlatformInfo: kubernetes.NewPlatformInfo(
					&version.Info{
						Major:      "1",
						Minor:      "32",
						GitVersion: "1.32.0",
					},
					nil,
					nil,
				),
			},
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent: testExpectedAgent(
				apicommon.OtelAgent,
				defaultExpectedPorts,
				defaultExpectedEnvVars,
				defaultAnnotations,
				defaultVolumeMounts,
				defaultVolumes(defaultLocalObjectReferenceName),
			),
		},
		{
			Name: "otel agent enabled with service ports override",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorPorts(4444, 5555).
				WithOTelCollectorConfig().
				Build(),
			WantConfigure: true,
			StoreOption: &store.StoreOptions{
				PlatformInfo: kubernetes.NewPlatformInfo(
					&version.Info{
						Major:      "1",
						Minor:      "32",
						GitVersion: "1.32.0",
					},
					nil,
					nil,
				),
			},
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent: testExpectedAgent(
				apicommon.OtelAgent,
				expectedPorts{
					grpcPort: 4444,
					httpPort: 5555,
				},
				defaultExpectedEnvVars,
				defaultAnnotations,
				defaultVolumeMounts,
				defaultVolumes(defaultLocalObjectReferenceName),
			),
		},
	}
	tests.Run(t, buildOtelCollectorFeature)
}

func testExpectedAgent(
	agentContainerName apicommon.AgentContainerName,
	expectedPorts expectedPorts,
	expectedEnvVars expectedEnvVars,
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
			wantEnvVarsOTel := []*corev1.EnvVar{}

			if expectedEnvVars.agent_ipc_port.present {
				wantEnvVars = append(wantEnvVars, &corev1.EnvVar{
					Name:  DDAgentIpcPort,
					Value: expectedEnvVars.agent_ipc_port.value,
				})
				wantEnvVarsOTel = append(wantEnvVarsOTel, &corev1.EnvVar{
					Name:  DDAgentIpcPort,
					Value: expectedEnvVars.agent_ipc_port.value,
				})
			}

			if expectedEnvVars.agent_ipc_refresh.present {
				wantEnvVars = append(wantEnvVars, &corev1.EnvVar{
					Name:  DDAgentIpcConfigRefreshInterval,
					Value: expectedEnvVars.agent_ipc_refresh.value,
				})
				wantEnvVarsOTel = append(wantEnvVarsOTel, &corev1.EnvVar{
					Name:  DDAgentIpcConfigRefreshInterval,
					Value: expectedEnvVars.agent_ipc_refresh.value,
				})
			}

			if expectedEnvVars.enabled.present {
				wantEnvVars = append(wantEnvVars, &corev1.EnvVar{
					Name:  DDOtelCollectorCoreConfigEnabled,
					Value: expectedEnvVars.enabled.value,
				})
				wantEnvVarsOTel = append(wantEnvVarsOTel, &corev1.EnvVar{
					Name:  DDOtelCollectorCoreConfigEnabled,
					Value: expectedEnvVars.enabled.value,
				})
			}

			if expectedEnvVars.extension_timeout.present {
				wantEnvVars = append(wantEnvVars, &corev1.EnvVar{
					Name:  DDOtelCollectorCoreConfigExtensionTimeout,
					Value: expectedEnvVars.extension_timeout.value,
				})
			}

			if expectedEnvVars.extension_url.present {
				wantEnvVars = append(wantEnvVars, &corev1.EnvVar{
					Name:  DDOtelCollectorCoreConfigExtensionURL,
					Value: expectedEnvVars.extension_url.value,
				})
			}

			if expectedEnvVars.converter_features.present {
				wantEnvVarsOTel = append(wantEnvVarsOTel, &corev1.EnvVar{
					Name:  DDOtelCollectorConverterFeatures,
					Value: expectedEnvVars.converter_features.value,
				})
			}

			if len(wantEnvVars) == 0 {
				wantEnvVars = nil
			}

			agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
			otelAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.OtelAgent]
			image := mgr.PodTemplateSpec().Spec.Containers[0].Image
			assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantEnvVars))
			assert.True(t, apiutils.IsEqualStruct(otelAgentEnvVars, wantEnvVarsOTel), "OTel Agent envvars \ndiff = %s", cmp.Diff(otelAgentEnvVars, wantEnvVarsOTel))
			assert.Equal(t, images.GetLatestAgentImage(), image)

			// annotations
			agentAnnotations := mgr.AnnotationMgr.Annotations
			assert.Equal(t, expectedAnnotations, agentAnnotations)
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
	if t.Name() == "Test_otelCollectorFeature_Configure/otel_agent_enabled_with_configMap_multi_items" {
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

	// validate gateway-enabled tests use the gateway config
	if t.Name() == "Test_otelCollectorFeature_Configure/otel_agent_with_gateway_enabled_default_ports" {
		expectedCM["otel-config.yaml"] = defaultconfig.DefaultOtelCollectorConfigInGateway("")
		assert.True(
			t,
			apiutils.IsEqualStruct(configMap.Data, expectedCM),
			"ConfigMap \ndiff = %s", cmp.Diff(configMap.Data, expectedCM),
		)
		return
	}

	if t.Name() == "Test_otelCollectorFeature_Configure/otel_agent_with_gateway_enabled_non_default_ports" {
		expectedCM["otel-config.yaml"] = defaultconfig.DefaultOtelCollectorConfigInGateway("")
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

	serviceObject, found := store.Get(kubernetes.ServicesKind, "", "-agent")
	switch t.Name() {
	case "Test_otelCollectorFeature_Configure/otel_agent_enabled_with_service_ports_default":
		assert.True(t, found)
		service := serviceObject.(*corev1.Service)
		assert.Equal(t, []corev1.ServicePort{
			{
				Name:       "otlpgrpcport",
				Port:       4317,
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.FromInt(4317),
			},
			{
				Name:       "otlphttpport",
				Port:       4318,
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.FromInt(4318),
			},
		}, service.Spec.Ports)
	case "Test_otelCollectorFeature_Configure/otel_agent_enabled_with_service_ports_override":
		assert.True(t, found)
		service := serviceObject.(*corev1.Service)
		assert.Equal(t, []corev1.ServicePort{
			{
				Name:       "otlpgrpcport",
				Port:       4444,
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.FromInt(4444),
			},
			{
				Name:       "otlphttpport",
				Port:       5555,
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.FromInt(5555),
			},
		}, service.Spec.Ports)
	default:
		assert.False(t, found)
	}
}
