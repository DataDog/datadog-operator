// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func Test_rcFeature_Configure(t *testing.T) {
	ddav2RCDisabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
		},
	}
	ddav2RCEnabled := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
					Enabled: apiutils.NewBoolPointer(true),
				},
			},
		},
	}
	ddav2RCDefault := v2alpha1.DatadogAgent{
		Spec: v2alpha1.DatadogAgentSpec{},
	}

	tests := test.FeatureTestSuite{
		//////////////////////////
		// v2Alpha1.DatadogAgent
		//////////////////////////
		{
			Name:          "v2alpha1 RC not enabled",
			DDAv2:         ddav2RCDisabled.DeepCopy(),
			WantConfigure: true,
			Agent:         rcAgentNodeWantFunc("false"),
		},
		{
			Name:          "v2alpha1 RC enabled",
			DDAv2:         ddav2RCEnabled.DeepCopy(),
			WantConfigure: true,
			Agent:         rcAgentNodeWantFunc("true"),
		},
		{
			Name:          "v2alpha1 RC default",
			DDAv2:         ddav2RCDefault.DeepCopy(),
			WantConfigure: true,
			Agent:         rcAgentNodeWantFunc("true"),
		},
	}

	tests.Run(t, buildRCFeature)
}

func rcAgentNodeWantFunc(value string) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			// Check environment variable
			mgr := mgrInterface.(*fake.PodTemplateManagers)
			coreAgentWant := []*corev1.EnvVar{
				{
					Name:  apicommon.DDRemoteConfigurationEnabled,
					Value: value,
				},
			}
			coreAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.AllContainers]
			assert.True(t, apiutils.IsEqualStruct(coreAgentEnvVars, coreAgentWant), "Core agent env vars \ndiff = %s", cmp.Diff(coreAgentEnvVars, coreAgentWant))
		},
	)
}
