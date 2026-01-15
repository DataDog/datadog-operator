package otelagentgateway

import (
	"strings"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/otelagentgateway/defaultconfig"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	defaultLocalObjectReferenceName = "-otel-agent-gateway-config"
	defaultVolumeMounts             = []corev1.VolumeMount{
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

var defaultAnnotations = map[string]string{"checksum/otel_agent_gateway-custom-config": "271b7a21b7215c549ce1d617f2064a3f"}

func Test_otelAgentGatewayFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		// disabled
		{
			Name: "otel agent gateway disabled without config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelAgentGatewayEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "otel agent gateway disabled with config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelAgentGatewayEnabled(false).
				WithOTelAgentGatewayConfig().
				Build(),
			WantConfigure: false,
		},
		// enabled
		{
			Name: "otel agent gateway enabled with config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelAgentGatewayEnabled(true).
				WithOTelAgentGatewayConfig().
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			OtelAgentGateway:     testExpectedOtelAgentGateway(apicommon.OtelAgent, defaultExpectedPorts, defaultAnnotations, defaultVolumeMounts, defaultVolumes(defaultLocalObjectReferenceName)),
		},
		{
			Name: "otel agent gateway enabled with configMap",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelAgentGatewayEnabled(true).
				WithOTelAgentGatewayConfigMap().
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			OtelAgentGateway:     testExpectedOtelAgentGateway(apicommon.OtelAgent, defaultExpectedPorts, map[string]string{}, defaultVolumeMounts, defaultVolumes("user-provided-config-map")),
		},
		{
			Name: "otel agent gateway enabled with configMap multi items",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelAgentGatewayEnabled(true).
				WithOTelAgentGatewayConfigMapMultipleItems().
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			OtelAgentGateway: testExpectedOtelAgentGateway(apicommon.OtelAgent, defaultExpectedPorts, map[string]string{}, []corev1.VolumeMount{
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
			Name: "otel agent gateway enabled without config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelAgentGatewayEnabled(true).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			OtelAgentGateway:     testExpectedOtelAgentGateway(apicommon.OtelAgent, defaultExpectedPorts, defaultAnnotations, defaultVolumeMounts, defaultVolumes(defaultLocalObjectReferenceName)),
		},
		{
			Name: "otel agent gateway enabled without config non default ports",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelAgentGatewayEnabled(true).
				WithOTelAgentGatewayPorts(4444, 5555).
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			OtelAgentGateway: testExpectedOtelAgentGateway(apicommon.OtelAgent, expectedPorts{
				grpcPort: 4444,
				httpPort: 5555,
			},
				map[string]string{"checksum/otel_agent_gateway-custom-config": "69dcc5c01755641076ba5748c90ba409"},
				defaultVolumeMounts,
				defaultVolumes(defaultLocalObjectReferenceName),
			),
		},
		{
			Name: "otel agent gateway enabled with service ports default",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelAgentGatewayEnabled(true).
				WithOTelAgentGatewayConfig().
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			OtelAgentGateway: testExpectedOtelAgentGateway(
				apicommon.OtelAgent,
				defaultExpectedPorts,
				defaultAnnotations,
				defaultVolumeMounts,
				defaultVolumes(defaultLocalObjectReferenceName),
			),
		},
		{
			Name: "otel agent gateway enabled with service ports override",
			DDA: testutils.NewDatadogAgentBuilder().
				WithOTelAgentGatewayEnabled(true).
				WithOTelAgentGatewayPorts(4444, 5555).
				WithOTelAgentGatewayConfig().
				Build(),
			WantConfigure:        true,
			WantDependenciesFunc: testExpectedDepsCreatedCM,
			OtelAgentGateway: testExpectedOtelAgentGateway(
				apicommon.OtelAgent,
				expectedPorts{
					grpcPort: 4444,
					httpPort: 5555,
				},
				defaultAnnotations,
				defaultVolumeMounts,
				defaultVolumes(defaultLocalObjectReferenceName),
			),
		},
	}
	tests.Run(t, buildOtelAgentGatewayFeature)
}

func testExpectedOtelAgentGateway(
	agentContainerName apicommon.AgentContainerName,
	expectedPorts expectedPorts,
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
					Name:          "otel-grpc",
					ContainerPort: expectedPorts.grpcPort,
					Protocol:      corev1.ProtocolTCP,
				},
				{
					Name:          "otel-http",
					ContainerPort: expectedPorts.httpPort,
					Protocol:      corev1.ProtocolTCP,
				},
			}

			ports := mgr.PortMgr.PortsByC[agentContainerName]
			assert.Equal(t, wantPorts, ports)

			// annotations
			agentAnnotations := mgr.AnnotationMgr.Annotations
			assert.Equal(t, expectedAnnotations, agentAnnotations)
		},
	)
}

func testExpectedDepsCreatedCM(t testing.TB, store store.StoreClient) {
	// hacky to need to hardcode test name but unaware of a better approach that doesn't require
	// modifying WantDependenciesFunc definition.
	if t.Name() == "Test_otelAgentGatewayFeature_Configure/otel_agent_gateway_enabled_with_configMap" {
		// configMap is provided by user, no need to create it.
		_, found := store.Get(kubernetes.ConfigMapKind, "", "-otel-agent-gateway-config")
		assert.False(t, found)
		return
	}
	if t.Name() == "Test_otelAgentGatewayFeature_Configure/otel_agent_gateway_enabled_with_configMap_multi_items" {
		// configMap is provided by user, no need to create it.
		_, found := store.Get(kubernetes.ConfigMapKind, "", "-otel-agent-gateway-config")
		assert.False(t, found)
		return
	}
	configMapObject, found := store.Get(kubernetes.ConfigMapKind, "", "-otel-agent-gateway-config")
	assert.True(t, found)

	configMap := configMapObject.(*corev1.ConfigMap)
	expectedCM := map[string]string{
		"otel-config.yaml": defaultconfig.DefaultOtelAgentGatewayConfig}

	// validate that default ports were overriden by user provided ports in default config. hacky to need to
	// hardcode test name but unaware of a better approach that doesn't require modifying WantDependenciesFunc definition.
	if t.Name() == "Test_otelAgentGatewayFeature_Configure/otel_agent_gateway_enabled_without_config_non_default_ports" {
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

	serviceObject, found := store.Get(kubernetes.ServicesKind, "", "-otel-agent-gateway")
	switch t.Name() {
	case "Test_otelAgentGatewayFeature_Configure/otel_agent_gateway_enabled_with_service_ports_default":
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
	case "Test_otelAgentGatewayFeature_Configure/otel_agent_gateway_enabled_with_service_ports_override":
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
		assert.True(t, found)
	}
}

func Test_otelAgentGatewayFeature_ID(t *testing.T) {
	feat := buildOtelAgentGatewayFeature(nil)
	assert.Equal(t, string(feature.OtelAgentGatewayIDType), string(feat.ID()))
}

func Test_otelAgentGatewayFeature_ManageClusterAgent(t *testing.T) {
	feat := &otelAgentGatewayFeature{}
	err := feat.ManageClusterAgent(nil, "")
	assert.NoError(t, err)
}

func Test_otelAgentGatewayFeature_ManageSingleContainerNodeAgent(t *testing.T) {
	feat := &otelAgentGatewayFeature{}
	err := feat.ManageSingleContainerNodeAgent(nil, "")
	assert.NoError(t, err)
}

func Test_otelAgentGatewayFeature_ManageNodeAgent(t *testing.T) {
	feat := &otelAgentGatewayFeature{}
	err := feat.ManageNodeAgent(nil, "")
	assert.NoError(t, err)
}

func Test_otelAgentGatewayFeature_ManageClusterChecksRunner(t *testing.T) {
	feat := &otelAgentGatewayFeature{}
	err := feat.ManageClusterChecksRunner(nil, "")
	assert.NoError(t, err)
}
