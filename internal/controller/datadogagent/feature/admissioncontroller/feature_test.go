// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package admissioncontroller

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	v2alpha1test "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1/test"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	defaulting "github.com/DataDog/datadog-operator/pkg/defaulting"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_admissionControllerFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "Admission Controller not enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				Build(),
			WantConfigure: false,
		},
		{
			Name: "Admission Controller enabled with basic setup",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(false, false, "", "", false)),
		},
		{
			Name: "Admission Controller enabled with validation and mutation enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithAdmissionControllerValidationEnabled(true).
				WithAdmissionControllerMutationEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(true, true, "", "", false)),
		},
		{
			Name: "Admission controller enabled, cwsInstrumentation enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithCWSInstrumentationEnabled(true).
				WithCWSInstrumentationMode("test-mode").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(false, false, "", "", true)),
		},
		{
			Name: "Admission Controller enabled with overriding registry",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithRegistry("testRegistry").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(false, false, "", "testRegistry", false)),
		},
		{
			Name: "Admission Controller enabled with custom registry in global config, override with feature config",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithAdmissionControllerRegistry("featureRegistry").
				WithRegistry("globalRegistry").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(false, false, "", "featureRegistry", false)),
		},
		{
			Name: "Admission Controller enabled with apm uds",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithAPMEnabled(true).
				WithAPMUDSEnabled(true, "testHostPath").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(false, false, "socket", "", false)),
		},
		{
			Name: "Admission Controller enabled with DSD uds",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithDogstatsdUnixDomainSocketConfigEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(false, false, "socket", "", false)),
		},
		{
			Name: "Admission Controller enabled with sidecar basic setup",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithSidecarInjectionEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				sidecarInjectionWantFunc("", "", "", "agent", defaulting.AgentLatestVersion, false, false)),
		},
		{
			Name: "Admission Controller enabled with sidecar injection adding global registry",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithSidecarInjectionEnabled(true).
				WithRegistry("globalRegistry").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				sidecarInjectionWantFunc("", "globalRegistry", "globalRegistry", "agent", defaulting.AgentLatestVersion, false, false)),
		},
		{
			Name: "Admission Controller enabled with sidecar injection adding both sidecar and global registry",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithSidecarInjectionEnabled(true).
				WithRegistry("globalRegistry").
				WithSidecarInjectionRegistry("sidecarRegistry").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				sidecarInjectionWantFunc("", "globalRegistry", "sidecarRegistry", "agent", defaulting.AgentLatestVersion, false, false)),
		},
		{
			Name: "Admission Controller enabled with sidecar injection adding test sidecar image and tag",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithSidecarInjectionEnabled(true).
				WithSidecarInjectionImageName("testAgentImage").
				WithSidecarInjectionImageTag("testAgentTag").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				sidecarInjectionWantFunc("", "", "", "testAgentImage", "testAgentTag", false, false)),
		},
		{
			Name: "Admission Controller enabled with sidecar injection adding global image and tag",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithSidecarInjectionEnabled(true).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					Image: &v2alpha1.AgentImageConfig{
						Name: "overrideName",
						Tag:  "overrideTag",
					},
				}).
				WithSidecarInjectionImageName("").
				WithSidecarInjectionImageTag("").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				sidecarInjectionWantFunc("", "", "", "overrideName", "overrideTag", false, false)),
		},
		{
			Name: "Admission Controller enabled with sidecar injection adding both global and sidecar image and tag",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithSidecarInjectionEnabled(true).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					Image: &v2alpha1.AgentImageConfig{
						Name: "overrideName",
						Tag:  "overrideTag",
					},
				}).
				WithSidecarInjectionImageName("sidecarAgent").
				WithSidecarInjectionImageTag("sidecarTag").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				sidecarInjectionWantFunc("", "", "", "sidecarAgent", "sidecarTag", false, false)),
		},
		{
			Name: "Admission Controller enabled with sidecar injection with selector and profile",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithSidecarInjectionEnabled(true).
				WithSidecarInjectionSelectors("testKey", "testValue").
				WithSidecarInjectionProfiles("testName", "testValue", "500m", "1Gi").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				sidecarInjectionWantFunc("", "", "", "agent", defaulting.AgentLatestVersion, true, true)),
		},
	}

	tests.Run(t, buildAdmissionControllerFeature)
}

func testDCAResources(acm string, registry string, cwsInstrumentationEnabled bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
			expectedAgentEnvs := []*corev1.EnvVar{
				{
					Name:  apicommon.DDAdmissionControllerEnabled,
					Value: "true",
				},
				{
					Name:  apicommon.DDAdmissionControllerValidationEnabled,
					Value: "true",
				},
				{
					Name:  apicommon.DDAdmissionControllerMutationEnabled,
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

func getACEnvVars(validation, mutation bool, acm, registry string, cws bool) []*corev1.EnvVar {
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

	if validation {
		validationEnv := corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerValidationEnabled,
			Value: apiutils.BoolToString(&validation),
		}
		envVars = append(envVars, &validationEnv)
	}

	if mutation {
		mutationEnv := corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerMutationEnabled,
			Value: apiutils.BoolToString(&mutation),
		}
		envVars = append(envVars, &mutationEnv)
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

	if cws {
		cwsEnv := []corev1.EnvVar{
			{
				Name:  apicommon.DDAdmissionControllerCWSInstrumentationEnabled,
				Value: apiutils.BoolToString(&cws),
			},
			{
				Name:  apicommon.DDAdmissionControllerCWSInstrumentationMode,
				Value: "test-mode",
			},
		}
		envVars = append(envVars, &cwsEnv[0], &cwsEnv[1])
	}
	return envVars
}

func admissionControllerWantFunc(validation, mutation bool, acm, registry string, cws bool) func(testing.TB, feature.PodTemplateManagers) {
	return func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)
		dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
		want := getACEnvVars(validation, mutation, acm, registry, cws)
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

func getSidecarEnvVars(imageName, imageTag, registry string, selectors, profiles bool) []*corev1.EnvVar {
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
	if selectors {
		selectorEnv := corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerAgentSidecarSelectors,
			Value: "[{\"namespaceSelector\":{\"matchLabels\":{\"testKey\":\"testValue\"}},\"objectSelector\":{\"matchLabels\":{\"testKey\":\"testValue\"}}}]",
		}
		envVars = append(envVars, &selectorEnv)
	}

	if profiles {
		profileEnv := corev1.EnvVar{
			Name:  apicommon.DDAdmissionControllerAgentSidecarProfiles,
			Value: "[{\"env\":[{\"name\":\"testName\",\"value\":\"testValue\"}],\"resources\":{\"requests\":{\"cpu\":\"500m\",\"memory\":\"1Gi\"}}}]",
		}
		envVars = append(envVars, &profileEnv)
	}

	return envVars
}

func sidecarInjectionWantFunc(acm, acRegistry, sidecarRegstry, imageName, imageTag string, selectors, profiles bool) func(testing.TB, feature.PodTemplateManagers) {
	return func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)
		dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
		want := sidecarHelperFunc(getACEnvVars(false, false, acm, acRegistry, false), getSidecarEnvVars(imageName, imageTag, sidecarRegstry, selectors, profiles))
		assert.ElementsMatch(
			t,
			dcaEnvVars,
			want,
			"DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want),
		)
	}
}
