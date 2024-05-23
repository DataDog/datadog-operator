// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package admissioncontroller

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	v2alpha1test "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1/test"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
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
				WithAPMUDSEnabled(true, apmSocketHostPath).
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
				WithAPMUDSEnabled(true, apmSocketHostPath).
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
				sidecarInjectionWantFunc("", "", "", "agent", "7.53.0")),
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
				sidecarInjectionWantFunc("", "globalRegistry", "globalRegistry", "agent", "7.53.0")),
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
				sidecarInjectionWantFunc("", "globalRegistry", "sidecarRegistry", "agent", "7.53.0")),
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
	}

	tests.Run(t, buildAdmissionControllerFeature)
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
