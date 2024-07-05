// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profiling

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	v2alpha1test "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1/test"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

type envVar struct {
	name    string
	value   string
	present bool
}

func assertEnv(envVars ...envVar) *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)
			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommonv1.ClusterAgentContainerName]

			for _, envVar := range envVars {
				if !envVar.present {
					for _, env := range agentEnvs {
						require.NotEqual(t, envVar.name, env.Name)
					}
					continue
				}

				expected := &corev1.EnvVar{
					Name:  envVar.name,
					Value: envVar.value,
				}
				require.Contains(t, agentEnvs, expected)
			}
		},
	)
}

func TestProfilingFeature(t *testing.T) {
	test.FeatureTestSuite{
		{
			Name: "Profiling disabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithProfilingEnabled("false").
				Build(),

			WantConfigure: true,
			ClusterAgent:  assertEnv(envVar{name: apicommon.DDAdmissionControllerProfilingEnabled, value: "false", present: true}),
		},
		{
			Name: "Profiling unspecified",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithProfilingEnabled("").
				Build(),

			WantConfigure: false,
		},
		{
			Name: "Profiling set to auto",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithProfilingEnabled("auto").
				Build(),

			WantConfigure: true,
			ClusterAgent:  assertEnv(envVar{name: apicommon.DDAdmissionControllerProfilingEnabled, value: "auto", present: true}),
		},
		{
			Name: "Profiling enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithProfilingEnabled("true").
				Build(),

			WantConfigure: true,
			ClusterAgent:  assertEnv(envVar{name: apicommon.DDAdmissionControllerProfilingEnabled, value: "true", present: true}),
		},
	}.Run(t, buildProfilingFeature)
}
