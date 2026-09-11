// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checkrunner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/pkg/testutils"
)

func Test_checkRunnerFeature(t *testing.T) {
	checkRunnerEnabledEnvVar := &corev1.EnvVar{
		Name:  ddCheckRunnerEnabled,
		Value: "true",
	}
	// Sub-agent mode plus the IPC endpoint the Core Agent's Data Plane serves. These are the
	// ACR-side defaults, set explicitly so a default change upstream cannot silently move us.
	standaloneModeEnvVar := &corev1.EnvVar{
		Name:  ddCheckRunnerStandaloneMode,
		Value: "false",
	}
	ipcEnabledEnvVar := &corev1.EnvVar{
		Name:  ddCheckRunnerEndpointsIPCEnabled,
		Value: "true",
	}
	ipcEndpointEnvVar := &corev1.EnvVar{
		Name:  ddCheckRunnerEndpointsIPCEndpoint,
		Value: "http://localhost:5105",
	}
	checkRunnerEnvVars := []*corev1.EnvVar{standaloneModeEnvVar, ipcEnabledEnvVar, ipcEndpointEnvVar}

	tests := test.FeatureTestSuite{
		{
			Name: "check runner disabled (default)",
			DDA: testutils.NewDatadogAgentBuilder().
				BuildWithDefaults(),
			WantConfigure: false,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
					assert.NotContains(t, agentEnvVars, checkRunnerEnabledEnvVar, "DD_CHECK_RUNNER_ENABLED should not be set when the Check Runner is not enabled")
				},
			),
		},
		{
			Name: "check runner enabled via annotation",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAnnotations(map[string]string{
					featureutils.EnableCheckRunnerAnnotation: "true",
				}).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)

					// The Core Agent owns check_runner.enabled and publishes it to ACR over RAR.
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
					assert.Contains(t, agentEnvVars, checkRunnerEnabledEnvVar, "DD_CHECK_RUNNER_ENABLED should be set on the core agent")
					for _, e := range checkRunnerEnvVars {
						assert.NotContains(t, agentEnvVars, e, "%s is ACR's own config and should not be set on the core agent", e.Name)
					}

					// ACR's own settings.
					crEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.AgentCheckRunnerContainerName]
					for _, e := range checkRunnerEnvVars {
						assert.Contains(t, crEnvVars, e, "%s should be set on the check runner", e.Name)
					}
					assert.NotContains(t, crEnvVars, checkRunnerEnabledEnvVar, "DD_CHECK_RUNNER_ENABLED is a core agent setting and should not be set on the check runner")

					// No adapter is configured by the operator: which adapters ACR loads is
					// left to spec.override.nodeAgent.containers.agent-check-runner.env so it
					// can be flipped without an operator release.
					for _, e := range crEnvVars {
						assert.NotContains(t, e.Name, "ADAPTERS", "the operator should not pin an adapter; got %s", e.Name)
					}
				},
			),
		},
		{
			Name: "check runner enabled with single container strategy",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAnnotations(map[string]string{
					featureutils.EnableCheckRunnerAnnotation: "true",
				}).
				WithSingleContainerStrategy(true).
				BuildWithDefaults(),
			WantConfigure: true,
			// Under the single container strategy ACR runs as an s6 service inside the one
			// container, so both env vars must land there. Targeting the optimized-mode
			// container names instead would be silently dropped (this is the ADP bug we do
			// not reproduce). The suite dispatches to ManageSingleContainerNodeAgent on its
			// own once BuildFeatures collapses the required containers.
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)

					singleEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.UnprivilegedSingleAgentContainerName]
					assert.Contains(t, singleEnvVars, checkRunnerEnabledEnvVar, "DD_CHECK_RUNNER_ENABLED should be set on the single agent container")
					for _, e := range checkRunnerEnvVars {
						assert.Contains(t, singleEnvVars, e, "%s should be set on the single agent container", e.Name)
					}
				},
			),
		},
	}

	tests.Run(t, buildCheckRunnerFeature)
}
