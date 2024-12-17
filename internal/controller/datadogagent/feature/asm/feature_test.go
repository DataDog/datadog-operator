// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package asm

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	v2alpha1test "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1/test"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"

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
			agentEnvs := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]

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

func TestASMFeature(t *testing.T) {
	test.FeatureTestSuite{
		{
			Name: "ASM not enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithASMEnabled(false, false, false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "ASM Threats enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithASMEnabled(true, false, false).
				Build(),

			WantConfigure: true,
			ClusterAgent:  assertEnv(envVar{name: DDAdmissionControllerAppsecEnabled, value: "true", present: true}),
		},
		{
			Name: "ASM Threats enabled, admission controller not enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(false).
				WithASMEnabled(true, false, false).
				Build(),

			WantConfigure: false,
		},
		{
			Name: "ASM Threats enabled, admission controller not configured",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithASMEnabled(true, false, false).
				Build(),

			WantConfigure: false,
		},
		{
			Name: "ASM SCA enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithASMEnabled(false, true, false).
				WithAdmissionControllerEnabled(true).
				Build(),

			WantConfigure: true,
			ClusterAgent:  assertEnv(envVar{name: DDAdmissionControllerAppsecSCAEnabled, value: "true", present: true}),
		},
		{
			Name: "ASM IAST enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithASMEnabled(false, false, true).
				WithAdmissionControllerEnabled(true).
				Build(),

			WantConfigure: true,
			ClusterAgent:  assertEnv(envVar{name: DDAdmissionControllerIASTEnabled, value: "true", present: true}),
		},
		{
			Name: "ASM all enabled",
			DDA: v2alpha1test.NewDatadogAgentBuilder().
				WithAdmissionControllerEnabled(true).
				WithASMEnabled(true, true, true).
				Build(),

			WantConfigure: true,
			ClusterAgent: assertEnv(
				envVar{
					name:    DDAdmissionControllerAppsecEnabled,
					value:   "true",
					present: true,
				}, envVar{
					name:    DDAdmissionControllerAppsecSCAEnabled,
					value:   "true",
					present: true,
				}, envVar{
					name:    DDAdmissionControllerIASTEnabled,
					value:   "true",
					present: true,
				}),
		},
	}.Run(t, buildASMFeature)
}
