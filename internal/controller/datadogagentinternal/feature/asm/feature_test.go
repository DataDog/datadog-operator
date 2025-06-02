// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package asm

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/test"
	"github.com/DataDog/datadog-operator/pkg/testutils"

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
			DDAI: testutils.NewDatadogAgentInternalBuilder().
				WithAdmissionControllerEnabled(true).
				WithASMEnabled(false, false, false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "ASM Threats enabled",
			DDAI: testutils.NewDatadogAgentInternalBuilder().
				WithAdmissionControllerEnabled(true).
				WithASMEnabled(true, false, false).
				Build(),

			WantConfigure: true,
			ClusterAgent:  assertEnv(envVar{name: DDAdmissionControllerAppsecEnabled, value: "true", present: true}),
		},
		{
			Name: "ASM Threats enabled, admission controller not enabled",
			DDAI: testutils.NewDatadogAgentInternalBuilder().
				WithAdmissionControllerEnabled(false).
				WithASMEnabled(true, false, false).
				Build(),

			WantConfigure: false,
		},
		{
			Name: "ASM Threats enabled, admission controller not configured",
			DDAI: testutils.NewDatadogAgentInternalBuilder().
				WithASMEnabled(true, false, false).
				Build(),

			WantConfigure: false,
		},
		{
			Name: "ASM SCA enabled",
			DDAI: testutils.NewDatadogAgentInternalBuilder().
				WithASMEnabled(false, true, false).
				WithAdmissionControllerEnabled(true).
				Build(),

			WantConfigure: true,
			ClusterAgent:  assertEnv(envVar{name: DDAdmissionControllerAppsecSCAEnabled, value: "true", present: true}),
		},
		{
			Name: "ASM IAST enabled",
			DDAI: testutils.NewDatadogAgentInternalBuilder().
				WithASMEnabled(false, false, true).
				WithAdmissionControllerEnabled(true).
				Build(),

			WantConfigure: true,
			ClusterAgent:  assertEnv(envVar{name: DDAdmissionControllerIASTEnabled, value: "true", present: true}),
		},
		{
			Name: "ASM all enabled",
			DDAI: testutils.NewDatadogAgentInternalBuilder().
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
