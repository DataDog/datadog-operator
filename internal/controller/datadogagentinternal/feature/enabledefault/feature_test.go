// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package enabledefault

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/test"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/utils"
	"github.com/DataDog/datadog-operator/pkg/testutils"
)

func Test_defaultFeature_ADP(t *testing.T) {
	adpEnabledEnvVar := &corev1.EnvVar{
		Name:  common.DDADPEnabled,
		Value: "true",
	}

	tests := test.FeatureTestSuite{
		{
			Name: "adp disabled (default)",
			DDAI: testutils.NewDatadogAgentInternalBuilder().
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
					assert.NotContains(t, agentEnvVars, adpEnabledEnvVar, "DD_ADP_ENABLED should not be set to true when ADP is not enabled")
				},
			),
		},
		{
			Name: "adp disabled (forced)",
			DDAI: testutils.NewDatadogAgentInternalBuilder().
				WithAnnotations(map[string]string{
					utils.EnableADPAnnotation: "false",
				}).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
					assert.NotContains(t, agentEnvVars, adpEnabledEnvVar, "DD_ADP_ENABLED should not be set to true when ADP is not enabled")
				},
			),
		},
		{
			Name: "adp enabled",
			DDAI: testutils.NewDatadogAgentInternalBuilder().
				WithAnnotations(map[string]string{
					utils.EnableADPAnnotation: "true",
				}).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
					assert.Contains(t, agentEnvVars, adpEnabledEnvVar, "DD_ADP_ENABLED should be set to true when ADP is enabled")
				},
			),
		},
	}

	tests.Run(t, buildDefaultFeature)
}
