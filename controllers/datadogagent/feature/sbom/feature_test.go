// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sbom

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	// "github.com/DataDog/datadog-operator/controllers/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_sbomFeature_Configure(t *testing.T) {

	sbomDisabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				SBOM: &v2alpha1.SBOMFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}
	sbomEnabled := sbomDisabled.DeepCopy()
	{
		sbomEnabled.Spec.Features.SBOM.Enabled = apiutils.NewBoolPointer(true)
	}

	sbomEnabledContainerImageEnabled := sbomEnabled.DeepCopy()
	{
		sbomEnabledContainerImageEnabled.Spec.Features.SBOM.ContainerImage = &v2alpha1.SBOMTypeConfig{}
		sbomEnabledContainerImageEnabled.Spec.Features.SBOM.ContainerImage.Enabled = apiutils.NewBoolPointer(true)
	}

	sbomEnabledHostEnabled := sbomEnabled.DeepCopy()
	{
		sbomEnabledHostEnabled.Spec.Features.SBOM.Host = &v2alpha1.SBOMTypeConfig{}
		sbomEnabledHostEnabled.Spec.Features.SBOM.Host.Enabled = apiutils.NewBoolPointer(true)
	}

	sbomNodeAgentWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		wantEnvVars := []*corev1.EnvVar{
			{
				Name:  apicommon.DDSBOMEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDSBOMContainerImageEnabled,
				Value: "false",
			},
			{
				Name:  apicommon.DDSBOMHostEnabled,
				Value: "false",
			},
		}

		nodeAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
		assert.True(t, apiutils.IsEqualStruct(nodeAgentEnvVars, wantEnvVars), "Node agent envvars \ndiff = %s", cmp.Diff(nodeAgentEnvVars, wantEnvVars))
	}

	sbomWithContainerImageWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		wantEnvVars := []*corev1.EnvVar{
			{
				Name:  apicommon.DDSBOMEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDSBOMContainerImageEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDSBOMHostEnabled,
				Value: "false",
			},
		}

		nodeAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
		assert.True(t, apiutils.IsEqualStruct(nodeAgentEnvVars, wantEnvVars), "Node agent envvars \ndiff = %s", cmp.Diff(nodeAgentEnvVars, wantEnvVars))
	}

	sbomWithHostWantFunc := func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
		mgr := mgrInterface.(*fake.PodTemplateManagers)

		wantEnvVars := []*corev1.EnvVar{
			{
				Name:  apicommon.DDSBOMEnabled,
				Value: "true",
			},
			{
				Name:  apicommon.DDSBOMContainerImageEnabled,
				Value: "false",
			},
			{
				Name:  apicommon.DDSBOMHostEnabled,
				Value: "true",
			},
		}

		nodeAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
		assert.True(t, apiutils.IsEqualStruct(nodeAgentEnvVars, wantEnvVars), "Node agent envvars \ndiff = %s", cmp.Diff(nodeAgentEnvVars, wantEnvVars))
	}

	tests := test.FeatureTestSuite{
		{
			Name:          "SBOM not enabled",
			DDAv2:         sbomDisabled.DeepCopy(),
			WantConfigure: false,
		},
		{
			Name:          "SBOM enabled",
			DDAv2:         sbomEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(sbomNodeAgentWantFunc),
		},
		{
			Name:          "SBOM enabled, ContainerImage enabled",
			DDAv2:         sbomEnabledContainerImageEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(sbomWithContainerImageWantFunc),
		},
		{
			Name:          "SBOM enabled, Host enabled",
			DDAv2:         sbomEnabledHostEnabled,
			WantConfigure: true,
			Agent:         test.NewDefaultComponentTest().WithWantFunc(sbomWithHostWantFunc),
		},
	}

	tests.Run(t, buildSBOMFeature)
}
