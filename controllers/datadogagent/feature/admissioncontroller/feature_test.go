// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package admissioncontroller

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	v2alpha1test "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1/test"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
	defaulting "github.com/DataDog/datadog-operator/pkg/defaulting"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_admissionControllerFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		// //////////////////////////
		// // v1Alpha1.DatadogAgent
		// //////////////////////////
		{
			Name:          "v1alpha1 admission controller not enabled",
			DDAv1:         newV1Agent(false),
			WantConfigure: false,
		},
		{
			Name:          "v1alpha1 admission controller enabled, cwsInstrumentation not enabled",
			DDAv1:         newV1Agent(true),
			WantConfigure: true,
			ClusterAgent:  testDCAResources("hostip", "", false),
		},
		// //////////////////////////
		// // v2alpha1.DatadogAgent
		// //////////////////////////
		{
			Name: "v2alpha1 Admission Controller not enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				Build(),
			WantConfigure: false,
		},
		{
			Name: "v2alpha1 Admission Controller enabled with basic setup",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc("", "")),
		},
		{
			Name: "v2alpha1 Admission Controller enabled with overriding registry",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithRegistry("testRegistry").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc("", "testRegistry")),
		},
		{
			Name: "v2alpha1 Admission Controller enabled with custom registry in global config, override with feature config",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithAdmissionControllerRegistry("featureRegistry").
				WithRegistry("globalRegistry").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc("", "featureRegistry")),
		},
		{
			Name: "v2alpha1 Admission Controller enabled with apm uds",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithAPMEnabled(true).
				WithAPMUDSEnabled(true, "testHostPath").
				WithAPMUDSEnabled(true, "testHostPath").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc("socket", "")),
		},
		{
			Name: "v2alpha1 Admission Controller enabled with DSD uds",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithDogstatsdUnixDomainSocketConfigEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc("socket", "")),
		},
		{
			Name: "v2alpha1 Admission Controller enabled with sidecar basic setup",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithSidecarInjectionEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				sidecarInjectionWantFunc("", "", "", "agent", defaulting.AgentLatestVersion)),
		},
		{
			Name: "v2alpha1 Admission Controller enabled with sidecar injection adding global registry",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithSidecarInjectionEnabled(true).
				WithRegistry("globalRegistry").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				sidecarInjectionWantFunc("", "globalRegistry", "globalRegistry", "agent", defaulting.AgentLatestVersion)),
		},
		{
			Name: "v2alpha1 Admission Controller enabled with sidecar injection adding both sidecar and global registry",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithSidecarInjectionEnabled(true).
				WithRegistry("globalRegistry").
				WithSidecarInjectionRegistry("sidecarRegistry").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				sidecarInjectionWantFunc("", "globalRegistry", "sidecarRegistry", "agent", defaulting.AgentLatestVersion)),
		},
		{
			Name: "v2alpha1 Admission Controller enabled with sidecar injection adding test sidecar image and tag",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithSidecarInjectionEnabled(true).
				WithSidecarInjectionImageName("testAgentImage").
				WithSidecarInjectionImageTag("testAgentTag").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				sidecarInjectionWantFunc("", "", "", "testAgentImage", "testAgentTag")),
		},
		{
			Name: "v2alpha1 Admission Controller enabled with sidecar injection adding global image and tag",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithSidecarInjectionEnabled(true).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					Image: &apicommonv1.AgentImageConfig{
						Name: "overrideName",
						Tag:  "overrideTag",
					},
				}).
				WithSidecarInjectionImageName("").
				WithSidecarInjectionImageTag("").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				sidecarInjectionWantFunc("", "", "", "overrideName", "overrideTag")),
		},
		{
			Name: "v2alpha1 Admission Controller enabled with sidecar injection adding both global and sidecar image and tag",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithSidecarInjectionEnabled(true).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					Image: &apicommonv1.AgentImageConfig{
						Name: "overrideName",
						Tag:  "overrideTag",
					},
				}).
				WithSidecarInjectionImageName("sidecarAgent").
				WithSidecarInjectionImageTag("sidecarTag").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				sidecarInjectionWantFunc("", "", "", "sidecarAgent", "sidecarTag")),
		},
		{
			Name:          "v2alpha1 admission controller enabled, cwsInstrumentation enabled",
			DDAv2:         newV2Agent(true, "", "", true, &v2alpha1.APMFeatureConfig{}, &v2alpha1.DogstatsdFeatureConfig{}, nil),
			WantConfigure: true,
			ClusterAgent:  testDCAResources("", "", true),
		},
	}

	tests.Run(t, buildAdmissionControllerFeature)
}

func newV1Agent(enabled bool) *v1alpha1.DatadogAgent {
	return &v1alpha1.DatadogAgent{
		Spec: v1alpha1.DatadogAgentSpec{
			ClusterAgent: v1alpha1.DatadogAgentSpecClusterAgentSpec{
				Config: &v1alpha1.ClusterAgentConfig{
					AdmissionController: &v1alpha1.AdmissionControllerConfig{
						Enabled:                apiutils.NewBoolPointer(enabled),
						MutateUnlabelled:       apiutils.NewBoolPointer(true),
						ServiceName:            apiutils.NewStringPointer("testServiceName"),
						AgentCommunicationMode: apiutils.NewStringPointer("hostip"),
					},
				},
			},
		},
	}
}

func newV2Agent(enabled bool, acm, registry string, cwsInstrumentationEnabled bool, apm *v2alpha1.APMFeatureConfig, dsd *v2alpha1.DogstatsdFeatureConfig, global *v2alpha1.GlobalConfig) *v2alpha1.DatadogAgent {
	dda := &v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Global: &v2alpha1.GlobalConfig{},
			Features: &v2alpha1.DatadogFeatures{
				AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
					Enabled:          apiutils.NewBoolPointer(enabled),
					MutateUnlabelled: apiutils.NewBoolPointer(true),
					ServiceName:      apiutils.NewStringPointer("testServiceName"),
					CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
						Enabled: apiutils.NewBoolPointer(cwsInstrumentationEnabled),
						Mode:    apiutils.NewStringPointer("test-mode"),
					},
				},
			},
		},
	}
	if acm != "" {
		dda.Spec.Features.AdmissionController.AgentCommunicationMode = apiutils.NewStringPointer(acm)
	}
	if apm != nil {
		dda.Spec.Features.APM = apm
	}
	if dsd != nil {
		dda.Spec.Features.Dogstatsd = dsd
	}
	if registry != "" {
		dda.Spec.Features.AdmissionController.Registry = apiutils.NewStringPointer(registry)

	}
	if global != nil {
		dda.Spec.Global = global
	}
	return dda
}

func testDCAResources(acm string, registry string, cwsInstrumentationEnabled bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  apicommon.DDAdmissionControllerEnabled,
					Value: "true",
				},
				{
					Name:  apicommon.DDAdmissionControllerMutateUnlabelled,
					Value: "true",
				},
				{
					Name:  apicommon.DDAdmissionControllerServiceName,
					Value: "testServiceName",
				},
				{
					Name:  apicommon.DDAdmissionControllerLocalServiceName,
					Value: "-agent",
				},
				{
					Name:  apicommon.DDAdmissionControllerWebhookName,
					Value: "datadog-webhook",
				},
			}
			if cwsInstrumentationEnabled {
				expectedAgentEnvs = append(expectedAgentEnvs, []*corev1.EnvVar{
					{
						Name:  apicommon.DDAdmissionControllerCWSInstrumentationEnabled,
						Value: apiutils.BoolToString(&cwsInstrumentationEnabled),
					},
					{
						Name:  apicommon.DDAdmissionControllerCWSInstrumentationMode,
						Value: "test-mode",
					},
				}...)
			}
			if acm != "" {
				acmEnv := corev1.EnvVar{
					Name:  apicommon.DDAdmissionControllerInjectConfigMode,
					Value: acm,
				}
				expectedAgentEnvs = append(expectedAgentEnvs, &acmEnv)
			}
			if registry != "" {
				registryEnv := corev1.EnvVar{
					Name:  apicommon.DDAdmissionControllerRegistryName,
					Value: registry,
				}
				expectedAgentEnvs = append(expectedAgentEnvs, &registryEnv)
			}

			assert.ElementsMatch(t,
				agentEnvs,
				expectedAgentEnvs,
				"Cluster Agent ENVs (-want +got):\n%s",
				cmp.Diff(agentEnvs, expectedAgentEnvs),
			)
		},
	)
}

func getACEnvVars(acm, registry string) []*corev1.EnvVar {
	envVars := []*corev1.EnvVar{
		{
			Name:  apicommon.DDAdmissionControllerEnabled,
			Value: "true",
		},
		{
			Name:  apicommon.DDAdmissionControllerMutateUnlabelled,
			Value: "false",
		},
		{
			Name:  apicommon.DDAdmissionControllerLocalServiceName,
			Value: "-agent",
		},
		{
			Name:  apicommon.DDAdmissionControllerWebhookName,
			Value: "datadog-webhook",
		},
	}

	if acm != "" {
		acmEnv := corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerInjectConfigMode,
			Value: acm,
		}
		envVars = append(envVars, &acmEnv)
	}
	if registry != "" {
		registryEnv := corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerRegistryName,
			Value: registry,
		}
		envVars = append(envVars, &registryEnv)
	}
	return envVars
}

func admissionControllerWantFunc(acm, registry string) func(testing.TB, feature.PodTemplateManagers) {
	return func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)
		dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]
		want := getACEnvVars(acm, registry)
		assert.ElementsMatch(
			t,
			dcaEnvVars,
			want,
			"DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want),
		)
	}
}

func sidecarHelperFunc(admissionControllerConfig, sidecarConfig []*corev1.EnvVar) []*corev1.EnvVar {
	envVars := []*corev1.EnvVar{}

	// Append elements of admissionControllerConfig to envVars
	envVars = append(envVars, admissionControllerConfig...)

	// Append elements of sidecarConfig to envVars
	envVars = append(envVars, sidecarConfig...)

	return envVars
}

func getSidecarEnvVars(imageName, imageTag, registry string) []*corev1.EnvVar {
	envVars := []*corev1.EnvVar{
		{
			Name:  apicommon.DDAdmissionControllerAgentSidecarEnabled,
			Value: "true",
		},
		{
			Name:  apicommon.DDAdmissionControllerAgentSidecarClusterAgentEnabled,
			Value: "true",
		},
		{
			Name:  apicommon.DDAdmissionControllerAgentSidecarProvider,
			Value: "fargate",
		},
		{
			Name:  apicommon.DDAdmissionControllerAgentSidecarImageName,
			Value: imageName,
		},
		{
			Name:  apicommon.DDAdmissionControllerAgentSidecarImageTag,
			Value: imageTag,
		},
	}
	if registry != "" {
		registryEnv := corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerAgentSidecarRegistry,
			Value: registry,
		}
		envVars = append(envVars, &registryEnv)
	}

	return envVars
}

func sidecarInjectionWantFunc(acm, acRegistry, sidecarRegstry, imageName, imageTag string) func(testing.TB, feature.PodTemplateManagers) {
	return func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)
		dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]
		want := sidecarHelperFunc(getACEnvVars(acm, acRegistry), getSidecarEnvVars(imageName, imageTag, sidecarRegstry))
		assert.ElementsMatch(
			t,
			dcaEnvVars,
			want,
			"DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want),
		)
	}
}
