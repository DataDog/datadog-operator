// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package admissioncontroller

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	defaulting "github.com/DataDog/datadog-operator/pkg/defaulting"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

const (
	// Exepcted admission controller service name with `datadog` DatadogAgent name
	datadogACServiceName = "datadog-admission-controller"
	// Exepcted admission controller webhook name with `datadog` DatadogAgent name
	datadogACWebhookName = "datadog-webhook"
)

func Test_admissionControllerFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "Admission Controller not enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				Build(),
			WantConfigure: false,
		},
		{
			Name: "Admission Controller enabled with basic setup",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("datadog").
				WithAdmissionControllerEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(datadogACServiceName, datadogACWebhookName, false, false, "", "", false)),
		},
		{
			Name: "Admission Controller enabled with valid provided service name, no webhook name",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("datadog").
				WithAdmissionControllerEnabled(true).
				WithAdmissionControllerServiceName("foo-ac-service").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc("foo-ac-service", datadogACWebhookName, false, false, "", "", false)),
		},
		{
			Name: "Admission Controller enabled with invalid (empty) provided service name, no webhook name",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("datadog").
				WithAdmissionControllerEnabled(true).
				WithAdmissionControllerServiceName("").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				// Expect the service name from DDA to be used as the provided service name is invalid
				admissionControllerWantFunc(datadogACServiceName, datadogACWebhookName, false, false, "", "", false)),
		},
		{
			Name: "Admission Controller enabled with provided service and webhook name",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("datadog").
				WithAdmissionControllerEnabled(true).
				WithAdmissionControllerServiceName("foo-ac-service").
				WithAdmissionControllerWebhookName("foo-ac-webhook").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc("foo-ac-service", "foo-ac-webhook", false, false, "", "", false)),
		},
		{
			Name: "Admission Controller enabled with validation and mutation enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("datadog").
				WithAdmissionControllerEnabled(true).
				WithAdmissionControllerValidationEnabled(true).
				WithAdmissionControllerMutationEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(datadogACServiceName, datadogACWebhookName, true, true, "", "", false)),
		},
		{
			Name: "Admission controller enabled, cwsInstrumentation enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("datadog").
				WithAdmissionControllerEnabled(true).
				WithCWSInstrumentationEnabled(true).
				WithCWSInstrumentationMode("test-mode").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(datadogACServiceName, datadogACWebhookName, false, false, "", "", true)),
		},
		{
			Name: "Admission Controller enabled with overriding registry",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("datadog").
				WithAdmissionControllerEnabled(true).
				WithRegistry("testRegistry").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(datadogACServiceName, datadogACWebhookName, false, false, "", "testRegistry", false)),
		},
		{
			Name: "Admission Controller enabled with custom registry in global config, override with feature config",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("datadog").
				WithAdmissionControllerEnabled(true).
				WithAdmissionControllerRegistry("featureRegistry").
				WithRegistry("globalRegistry").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(datadogACServiceName, datadogACWebhookName, false, false, "", "featureRegistry", false)),
		},
		{
			Name: "Admission Controller enabled with apm uds",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("datadog").
				WithAdmissionControllerEnabled(true).
				WithAPMEnabled(true).
				WithAPMUDSEnabled(true, "testHostPath").
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(datadogACServiceName, datadogACWebhookName, false, false, "socket", "", false)),
		},
		{
			Name: "Admission Controller enabled with DSD uds",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("datadog").
				WithAdmissionControllerEnabled(true).
				WithDogstatsdUnixDomainSocketConfigEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				admissionControllerWantFunc(datadogACServiceName, datadogACWebhookName, false, false, "socket", "", false)),
		},
		{
			Name: "Admission Controller enabled with sidecar basic setup",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("datadog").
				WithAdmissionControllerEnabled(true).
				WithSidecarInjectionEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent: test.NewDefaultComponentTest().WithWantFunc(
				sidecarInjectionWantFunc("", "", "", "agent", defaulting.AgentLatestVersion, false, false)),
		},
		{
			Name: "Admission Controller enabled with sidecar injection adding global registry",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("datadog").
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
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("datadog").
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
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("datadog").
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
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("datadog").
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
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("datadog").
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
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("datadog").
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

func getACEnvVars(svcName string, webhookName string, validation, mutation bool, acm, registry string, cws bool) []*corev1.EnvVar {
	envVars := []*corev1.EnvVar{
		{
			Name:  DDAdmissionControllerEnabled,
			Value: "true",
		},
		{
			Name:  DDAdmissionControllerMutateUnlabelled,
			Value: "false",
		},
		{
			Name:  DDAdmissionControllerLocalServiceName,
			Value: "datadog-agent",
		},
	}

	if svcName != "" {
		svcNameEnv := corev1.EnvVar{
			Name:  DDAdmissionControllerServiceName,
			Value: svcName,
		}
		envVars = append(envVars, &svcNameEnv)
	}

	if webhookName != "" {
		webhookNameEnv := corev1.EnvVar{
			Name:  DDAdmissionControllerWebhookName,
			Value: webhookName,
		}
		envVars = append(envVars, &webhookNameEnv)
	}

	if validation {
		validationEnv := corev1.EnvVar{
			Name:  DDAdmissionControllerValidationEnabled,
			Value: apiutils.BoolToString(&validation),
		}
		envVars = append(envVars, &validationEnv)
	}

	if mutation {
		mutationEnv := corev1.EnvVar{
			Name:  DDAdmissionControllerMutationEnabled,
			Value: apiutils.BoolToString(&mutation),
		}
		envVars = append(envVars, &mutationEnv)
	}

	if acm != "" {
		acmEnv := corev1.EnvVar{
			Name:  DDAdmissionControllerInjectConfigMode,
			Value: acm,
		}
		envVars = append(envVars, &acmEnv)
	}
	if registry != "" {
		registryEnv := corev1.EnvVar{
			Name:  DDAdmissionControllerRegistryName,
			Value: registry,
		}
		envVars = append(envVars, &registryEnv)
	}

	if cws {
		cwsEnv := []corev1.EnvVar{
			{
				Name:  DDAdmissionControllerCWSInstrumentationEnabled,
				Value: apiutils.BoolToString(&cws),
			},
			{
				Name:  DDAdmissionControllerCWSInstrumentationMode,
				Value: "test-mode",
			},
		}
		envVars = append(envVars, &cwsEnv[0], &cwsEnv[1])
	}
	return envVars
}

func admissionControllerWantFunc(svcName string, webhookName string, validation, mutation bool, acm, registry string, cws bool) func(testing.TB, feature.PodTemplateManagers) {
	return func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)
		dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
		want := getACEnvVars(svcName, webhookName, validation, mutation, acm, registry, cws)
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
			Name:  DDAdmissionControllerAgentSidecarEnabled,
			Value: "true",
		},
		{
			Name:  DDAdmissionControllerAgentSidecarClusterAgentEnabled,
			Value: "true",
		},
		{
			Name:  DDAdmissionControllerAgentSidecarProvider,
			Value: "fargate",
		},
		{
			Name:  DDAdmissionControllerAgentSidecarImageName,
			Value: imageName,
		},
		{
			Name:  DDAdmissionControllerAgentSidecarImageTag,
			Value: imageTag,
		},
	}
	if registry != "" {
		registryEnv := corev1.EnvVar{
			Name:  DDAdmissionControllerAgentSidecarRegistry,
			Value: registry,
		}
		envVars = append(envVars, &registryEnv)
	}
	if selectors {
		selectorEnv := corev1.EnvVar{
			Name:  DDAdmissionControllerAgentSidecarSelectors,
			Value: "[{\"namespaceSelector\":{\"matchLabels\":{\"testKey\":\"testValue\"}},\"objectSelector\":{\"matchLabels\":{\"testKey\":\"testValue\"}}}]",
		}
		envVars = append(envVars, &selectorEnv)
	}

	if profiles {
		profileEnv := corev1.EnvVar{
			Name:  DDAdmissionControllerAgentSidecarProfiles,
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
		want := sidecarHelperFunc(getACEnvVars(datadogACServiceName, datadogACWebhookName, false, false, acm, acRegistry, false), getSidecarEnvVars(imageName, imageTag, sidecarRegstry, selectors, profiles))
		assert.ElementsMatch(
			t,
			dcaEnvVars,
			want,
			"DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want),
		)
	}
}
