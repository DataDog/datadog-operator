package admissioncontroller

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	v2alpha1test "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1/test"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
)

const (
	apmSocketHostPath  = apicommon.DogstatsdAPMSocketHostPath + "/" + apicommon.APMSocketName
	apmSocketLocalPath = apicommon.APMSocketVolumeLocalPath + "/" + apicommon.APMSocketName
	customPath         = "/custom/host/filepath.sock"
)

func Test_admissionControllerFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "v2alpha1 Admission Controller not enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "v2alpha1 Admission Controller enabled with agent communication mode",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithAgentCommunicationMode("testCommunicationMode").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(true, false, "testCommunicationMode", "", "datadog-webhook"),
			),
		},
		{
			Name: "v2alpha1 Admission Controller enabled with custom service and webhook name",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithAgentCommunicationMode("testCommunicationMode").
				WithServiceName("testServiceName").
				WithWebhookName("testWebhookName").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(true, false, "testCommunicationMode", "testServiceName", "testWebhookName"),
			),
		},
		{
			Name: "v2alpha1 Admission Controller enabled with APM uds mode",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithAPMEnabled(true).
				WithAPMUDSEnabled(true, apmSocketHostPath).
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(true, false, "socket", "", "datadog-webhook"),
			),
		},
		{
			Name: "v2alpha1 Admission Controller enabled with DSD uds mode",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithDogstatsdUnixDomainSocketConfigEnabled(true).
				WithDogstatsdUnixDomainSocketConfigPath(customPath).
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(true, false, "socket", "", "datadog-webhook"),
			),
		},
		{
			Name: "v2alpha1 Admission Controller enabled with sidecar injection enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithSidecarInjectionEnabled(true).
				WithAgentCommunicationMode("testCommunicationMode").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(true, true, "testCommunicationMode", "", "datadog-webhook"),
			),
		},
	}

	tests.Run(t, buildAdmissionControllerFeature)
}

func generateEnvVars(enabled, sidecar bool, configMode, serviceName, webhookName string) []*corev1.EnvVar {
	envVars := []*corev1.EnvVar{
		{
			Name:  apicommon.DDAdmissionControllerEnabled,
			Value: "true",
		},
		{
			Name:  apicommon.DDAdmissionControllerMutateUnlabelled,
			Value: "false",
		},
	}
	if sidecar {
		envVars = append(envVars, []*corev1.EnvVar{
			{
				Name:  apicommon.DDAdmissionControllerAgentSidecarEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDAdmissionControllerAgentSidecarClusterAgentEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDAdmissionControllerAgentSidecarImageName,
				Value: "agent",
			},
		}...)
	}
	if serviceName != "" {
		serviceEnvVars := &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerServiceName,
			Value: serviceName,
		}
		envVars = append(envVars, serviceEnvVars)
	}
	if configMode != "" {
		configModeEnvVars := &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerInjectConfigMode,
			Value: configMode,
		}
		envVars = append(envVars, configModeEnvVars)
	}

	envVars = append(envVars, &corev1.EnvVar{Name: apicommon.DDAdmissionControllerLocalServiceName, Value: "-agent"})

	if webhookName != "" {
		webhookEnvVars := &corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerWebhookName,
			Value: webhookName,
		}
		envVars = append(envVars, webhookEnvVars)
	}

	return envVars
}

func admissionControllerWantFunc(enabled, sidecar bool, configMode, serviceName, webhookName string) func(testing.TB, feature.PodTemplateManagers) {
	return func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)
		dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]
		want := generateEnvVars(enabled, sidecar, configMode, serviceName, webhookName)
		assert.True(
			t,
			apiutils.IsEqualStruct(dcaEnvVars, want),
			"DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want),
		)
	}
}
