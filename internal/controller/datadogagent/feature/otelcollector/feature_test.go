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
)

type expectedPorts struct {
	httpPort int32
	grpcPort int32
}

type expectedEnvVars struct {
	agent_ipc_port    expectedEnvVar
	agent_ipc_refresh expectedEnvVar
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
)

var defaultAnnotations = map[string]string{"checksum/otel_agent-custom-config": "c609e2fb7352676a67f0423b58970d43"}

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
			Agent:                testExpectedAgent(apicommon.OtelAgent, false, defaultExpectedPorts, defaultLocalObjectReferenceName, defaultExpectedEnvVars, defaultAnnotations),
		},
		{
			Name: "otel agent enabled with configMap",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorConfigMap().
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent:                testExpectedAgent(apicommon.OtelAgent, false, defaultExpectedPorts, "user-provided-config-map", defaultExpectedEnvVars, map[string]string{}),
		},
		{
			Name: "otel agent enabled without config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent:                testExpectedAgent(apicommon.OtelAgent, false, defaultExpectedPorts, defaultLocalObjectReferenceName, defaultExpectedEnvVars, defaultAnnotations),
		},
		{
			Name: "otel agent enabled without config non default ports",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorPorts(4444, 5555).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent: testExpectedAgent(apicommon.OtelAgent, false, expectedPorts{
				grpcPort: 4444,
				httpPort: 5555,
			},
				defaultLocalObjectReferenceName,
				defaultExpectedEnvVars,
				map[string]string{"checksum/otel_agent-custom-config": "8fd9e6854714be53bd838063a4111c96"},
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
			Agent:                testExpectedAgent(apicommon.OtelAgent, false, defaultExpectedPorts, defaultLocalObjectReferenceName, defaultExpectedEnvVars, defaultAnnotations),
		},
		{
			Name: "otel agent coreconfig disabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorCoreConfigEnabled(false).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent:                testExpectedAgent(apicommon.OtelAgent, false, defaultExpectedPorts, defaultLocalObjectReferenceName, onlyIpcEnvVars, defaultAnnotations),
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
			Agent: testExpectedAgent(apicommon.OtelAgent, false, defaultExpectedPorts, defaultLocalObjectReferenceName, expectedEnvVars{
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
				defaultAnnotations),
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
			Agent: testExpectedAgent(apicommon.OtelAgent, false, defaultExpectedPorts, defaultLocalObjectReferenceName, expectedEnvVars{
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
				defaultAnnotations),
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
			Agent: testExpectedAgent(apicommon.OtelAgent, false, defaultExpectedPorts, defaultLocalObjectReferenceName, expectedEnvVars{
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
				defaultAnnotations),
		},
		{
			Name: "otel agent with UseStandaloneImage enabled (default)",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorUseStandaloneImage(true).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent:                testExpectedAgent(apicommon.OtelAgent, true, defaultExpectedPorts, defaultLocalObjectReferenceName, defaultExpectedEnvVars, defaultAnnotations),
		},
		test.FeatureTest{
			Name: "otel agent with UseStandaloneImage disabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorUseStandaloneImage(false).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			Agent:                testExpectedAgent(apicommon.OtelAgent, false, defaultExpectedPorts, defaultLocalObjectReferenceName, defaultExpectedEnvVars, defaultAnnotations),
		},
	}
	tests.Run(t, buildOtelCollectorFeature)
}

func testExpectedAgent(agentContainerName apicommon.AgentContainerName, useStandaloneImage bool, expectedPorts expectedPorts, localObjectReferenceName string, expectedEnvVars expectedEnvVars, expectedAnnotations map[string]string) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)
			// check volume mounts
			wantVolumeMounts := []corev1.VolumeMount{
				{
					Name:      otelAgentVolumeName,
					MountPath: common.ConfigVolumePath + "/" + otelConfigFileName,
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

			if len(wantEnvVars) == 0 {
				wantEnvVars = nil
			}

			agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
			otelAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.OtelAgent]
			assert.True(t, apiutils.IsEqualStruct(agentEnvVars, wantEnvVars), "Agent envvars \ndiff = %s", cmp.Diff(agentEnvVars, wantEnvVars))
			assert.True(t, apiutils.IsEqualStruct(otelAgentEnvVars, wantEnvVarsOTel), "OTel Agent envvars \ndiff = %s", cmp.Diff(otelAgentEnvVars, wantEnvVarsOTel))

			if useStandaloneImage {
				for _, container := range mgr.PodTemplateSpec().Spec.Containers {
					if container.Name == string(apicommon.OtelAgent) {
						assert.Equal(t, images.GetLatestDdotCollectorImage(), container.Image)
					} else {
						assert.Equal(t, images.GetLatestAgentImage(), container.Image)
					}
				}
			} else {
				for _, container := range mgr.PodTemplateSpec().Spec.Containers {
					assert.Equal(t, images.GetLatestAgentImageWithSuffix(false, false, true), container.Image)
				}
			}

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

func Test_otelCollectorFeature_ManageNodeAgent(t *testing.T) {
	tests := test.FeatureTestSuite{
		test.FeatureTest{
			Name: "otel agent with UseStandaloneImage enabled (default)",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorUseStandaloneImage(true).
				Build(),
			WantConfigure: true,
			Agent: &test.ComponentTest{
				CreateFunc: func(t testing.TB) (feature.PodTemplateManagers, string) {
					// Create pod template with both CoreAgent and OtelAgent containers
					newPTS := corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:    string(apicommon.CoreAgentContainerName),
									Image:   images.GetLatestAgentImage(),
									Command: []string{"agent", "run"},
								},
								{
									Name:    string(apicommon.OtelAgent),
									Image:   images.GetLatestDdotCollectorImage(),
									Command: []string{"otel-agent"},
								},
							},
						},
					}
					return fake.NewPodTemplateManagers(t, newPTS), kubernetes.DefaultProvider
				},
				WantFunc: func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)

					// When UseStandaloneImage is true, only otel-agent container should use ddot-collector image
					otelContainerFound := false
					for _, container := range mgr.PodTemplateSpec().Spec.Containers {
						if container.Name == string(apicommon.OtelAgent) {
							otelContainerFound = true
							expectedOtelImage := images.GetLatestDdotCollectorImage()
							assert.Equal(t, expectedOtelImage, container.Image, "OTel Agent container should use ddot-collector image when UseStandaloneImage is true")
						} else {
							// Other containers should not use -full image when UseStandaloneImage is true
							assert.NotContains(t, container.Image, "-full", "Non-OTel containers should not use -full image when UseStandaloneImage is true")
						}
					}
					assert.True(t, otelContainerFound, "OTel Agent container should be present")
				},
			},
		},
		{
			Name: "otel agent with UseStandaloneImage disabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				WithOTelCollectorUseStandaloneImage(false).
				Build(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)

					// When UseStandaloneImage is false, all containers should use -full image
					for _, container := range mgr.PodTemplateSpec().Spec.Containers {
						assert.Contains(t, container.Image, "-full", "All containers should use -full image when UseStandaloneImage is false")
					}
				},
			),
		},
	}

	tests.Run(t, buildOtelCollectorFeature)
}
