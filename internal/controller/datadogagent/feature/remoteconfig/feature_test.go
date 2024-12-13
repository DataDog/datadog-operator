// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	v2alpha1test "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1/test"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func Test_rcFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "RC not enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithRemoteConfigEnabled(false).
				Build(),
			WantConfigure: true,
			Agent:         rcAgentNodeWantFunc(false),
			ClusterAgent:  rcClusterAgentNodeWantFunc(false),
		},
		{
			Name: "RC enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithRemoteConfigEnabled(true).
				Build(),
			WantConfigure: true,
			Agent:         rcAgentNodeWantFunc(true),
			ClusterAgent:  rcClusterAgentNodeWantFunc(true),
		},
		{
			Name: "RC default (no datadogagent_default.go)",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				Build(),
			WantConfigure: true,
			Agent:         rcAgentNodeWantFunc(false),
			ClusterAgent:  rcClusterAgentNodeWantFunc(false),
		},
	}

	tests.Run(t, buildRCFeature)
}

func rcAgentNodeWantFunc(rcEnabled bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			// Check environment variable
			expectedEnvVars := []*corev1.EnvVar{
				{
					Name:  apicommon.DDRemoteConfigurationEnabled,
					Value: apiutils.BoolToString(&rcEnabled),
				},
			}
			actualEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.AllContainers]
			checkEqual(t, "Core agent env var", expectedEnvVars, actualEnvVars)
		},
	)
}

func rcClusterAgentNodeWantFunc(rcEnabled bool) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			// Check environment variable
			expectedEnvVars := []*corev1.EnvVar{
				{
					Name:  apicommon.DDRemoteConfigurationEnabled,
					Value: apiutils.BoolToString(&rcEnabled),
				},
			}
			actualEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.AllContainers]
			checkEqual(t, "Cluster agent env var", expectedEnvVars, actualEnvVars)

			// Check cluster agent volume
			expectedVolumes := make([]*corev1.Volume, 0)
			if rcEnabled {
				expectedVolumes = append(expectedVolumes, rcVolume)
			}
			actualVolumes := mgr.VolumeMgr.Volumes
			checkEqual(t, "Cluster agent volume", expectedVolumes, actualVolumes)

			// Check cluster agent volume mount
			var expectedVolumeMounts []*corev1.VolumeMount
			if rcEnabled {
				expectedVolumeMounts = append(expectedVolumeMounts, rcVolumeMount)
			}
			actualVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.AllContainers]
			checkEqual(t, "Cluster agent volume mount", expectedVolumeMounts, actualVolumeMounts)
		},
	)
}

func checkEqual(t testing.TB, description string, expected interface{}, actual interface{}) {
	assert.True(
		t,
		apiutils.IsEqualStruct(expected, actual),
		"%s\ndiff = %s",
		description,
		cmp.Diff(expected, actual),
	)
}
