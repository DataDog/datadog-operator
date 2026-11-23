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
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
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
	checkRunnerEnvVars := []*corev1.EnvVar{checkRunnerEnabledEnvVar, standaloneModeEnvVar, ipcEnabledEnvVar, ipcEndpointEnvVar}
	// ADP-side counterpart: without this ADP never serves the Checks IPC endpoint ACR sends to.
	dataPlaneChecksEnabledEnvVar := &corev1.EnvVar{
		Name:  common.DDDataPlaneChecksEnabled,
		Value: "true",
	}

	allEnvVars := append(append([]*corev1.EnvVar{}, checkRunnerEnvVars...), dataPlaneChecksEnabledEnvVar)

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
					for _, e := range allEnvVars {
						assert.NotContains(t, agentEnvVars, e, "%s should not be set when the Check Runner is not enabled", e.Name)
					}
				},
			),
		},
		{
			Name: "check runner enabled via annotation, data plane disabled (default)",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAnnotations(map[string]string{
					featureutils.EnableCheckRunnerAnnotation: "true",
				}).
				BuildWithDefaults(),
			WantConfigure:             true,
			WantManageDependenciesErr: true,
		},
		{
			Name: "check runner enabled via annotation, data plane explicitly disabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAnnotations(map[string]string{
					featureutils.EnableCheckRunnerAnnotation: "true",
				}).
				WithDataPlaneEnabled(false).
				BuildWithDefaults(),
			WantConfigure:             true,
			WantManageDependenciesErr: true,
		},
		{
			Name: "check runner enabled via annotation",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAnnotations(map[string]string{
					featureutils.EnableCheckRunnerAnnotation: "true",
				}).
				WithDataPlaneEnabled(true).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)

					// Everything is set on the Core Agent: it publishes them over ConfigStream.
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
					for _, e := range allEnvVars {
						assert.Contains(t, agentEnvVars, e, "%s should be set on the core agent", e.Name)
					}

					// Nothing is set directly on the ACR or ADP containers
					for _, container := range []apicommon.AgentContainerName{apicommon.AgentCheckRunnerContainerName, apicommon.AgentDataPlaneContainerName} {
						envVars := mgr.EnvVarMgr.EnvVarsByC[container]
						for _, e := range allEnvVars {
							assert.NotContains(t, envVars, e, "%s should not be set directly on %s; it flows through RAR", e.Name, container)
						}
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
				WithDataPlaneEnabled(true).
				WithSingleContainerStrategy(true).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)

					singleEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.UnprivilegedSingleAgentContainerName]
					for _, e := range allEnvVars {
						assert.Contains(t, singleEnvVars, e, "%s should be set on the single agent container", e.Name)
					}
				},
			),
		},
	}

	tests.Run(t, buildCheckRunnerFeature)
}
