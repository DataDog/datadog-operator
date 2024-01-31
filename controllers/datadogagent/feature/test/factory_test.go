package test

import (
	"testing"

	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	v2alpha1test "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1/test"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/apm"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/cspm"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/livecontainer"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/npm"
)

func TestBuilder(t *testing.T) {

	tests := []struct {
		name                   string
		dda                    *v2alpha1.DatadogAgent
		wantCoreAgentComponent bool
		wantAgentContainer     map[apicommonv1.AgentContainerName]bool
	}{
		{
			// This test relies on the fact that by default Live Container feature is enabled
			// in the default settings which enables process agent.
			name: "Default DDA, Core and Process agent enabled",
			dda: v2alpha1test.NewDatadogAgentBuilder().
				BuildWithDefaults(),
			wantAgentContainer: map[apicommonv1.AgentContainerName]bool{
				apicommonv1.UnprivilegedSingleAgentContainerName: false,
				apicommonv1.CoreAgentContainerName:               true,
				apicommonv1.ProcessAgentContainerName:            true,
				apicommonv1.TraceAgentContainerName:              true,
				apicommonv1.SystemProbeContainerName:             false,
				apicommonv1.SecurityAgentContainerName:           false,
			},
		},
		{
			name: "Default DDA with single container strategy, 1 single container",
			dda: v2alpha1test.NewDatadogAgentBuilder().
				WithSingleContainerStrategy(true).
				BuildWithDefaults(),
			wantAgentContainer: map[apicommonv1.AgentContainerName]bool{
				apicommonv1.UnprivilegedSingleAgentContainerName: true,
				apicommonv1.CoreAgentContainerName:               false,
				apicommonv1.ProcessAgentContainerName:            false,
				apicommonv1.TraceAgentContainerName:              false,
				apicommonv1.SystemProbeContainerName:             false,
				apicommonv1.SecurityAgentContainerName:           false,
			},
		},
		{
			name: "APM enabled, 3 agents",
			dda: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				BuildWithDefaults(),
			wantAgentContainer: map[apicommonv1.AgentContainerName]bool{
				apicommonv1.UnprivilegedSingleAgentContainerName: false,
				apicommonv1.CoreAgentContainerName:               true,
				apicommonv1.ProcessAgentContainerName:            true,
				apicommonv1.TraceAgentContainerName:              true,
				apicommonv1.SystemProbeContainerName:             false,
				apicommonv1.SecurityAgentContainerName:           false,
			},
		},
		{
			name: "APM enabled with single container strategy, 1 single container",
			dda: v2alpha1test.NewDatadogAgentBuilder().
				WithSingleContainerStrategy(true).
				WithAPMEnabled(true).
				BuildWithDefaults(),
			wantAgentContainer: map[apicommonv1.AgentContainerName]bool{
				apicommonv1.UnprivilegedSingleAgentContainerName: true,
				apicommonv1.CoreAgentContainerName:               false,
				apicommonv1.ProcessAgentContainerName:            false,
				apicommonv1.TraceAgentContainerName:              false,
				apicommonv1.SystemProbeContainerName:             false,
				apicommonv1.SecurityAgentContainerName:           false,
			},
		},
		{
			name: "APM, NPM enabled, 4 agents",
			dda: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithNPMEnabled(true).
				BuildWithDefaults(),
			wantAgentContainer: map[apicommonv1.AgentContainerName]bool{
				apicommonv1.UnprivilegedSingleAgentContainerName: false,
				apicommonv1.CoreAgentContainerName:               true,
				apicommonv1.ProcessAgentContainerName:            true,
				apicommonv1.TraceAgentContainerName:              true,
				apicommonv1.SystemProbeContainerName:             true,
				apicommonv1.SecurityAgentContainerName:           false,
			},
		},
		{
			name: "APM, NPM enabled with single container strategy, 4 agents",
			dda: v2alpha1test.NewDatadogAgentBuilder().
				WithSingleContainerStrategy(true).
				WithAPMEnabled(true).
				WithNPMEnabled(true).
				BuildWithDefaults(),
			wantAgentContainer: map[apicommonv1.AgentContainerName]bool{
				apicommonv1.UnprivilegedSingleAgentContainerName: false,
				apicommonv1.CoreAgentContainerName:               true,
				apicommonv1.ProcessAgentContainerName:            true,
				apicommonv1.TraceAgentContainerName:              true,
				apicommonv1.SystemProbeContainerName:             true,
				apicommonv1.SecurityAgentContainerName:           false,
			},
		},
		{
			name: "APM, NPM, CSPM enabled, 5 agents",
			dda: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithNPMEnabled(true).
				WithCSPMEnabled(true).
				BuildWithDefaults(),
			wantAgentContainer: map[apicommonv1.AgentContainerName]bool{
				apicommonv1.UnprivilegedSingleAgentContainerName: false,
				apicommonv1.CoreAgentContainerName:               true,
				apicommonv1.ProcessAgentContainerName:            true,
				apicommonv1.TraceAgentContainerName:              true,
				apicommonv1.SystemProbeContainerName:             true,
				apicommonv1.SecurityAgentContainerName:           true,
			},
		},
		{
			name: "APM, NPM, CSPM enabled with single container strategy, 5 agents",
			dda: v2alpha1test.NewDatadogAgentBuilder().
				WithSingleContainerStrategy(true).
				WithAPMEnabled(true).
				WithNPMEnabled(true).
				WithCSPMEnabled(true).
				BuildWithDefaults(),
			wantAgentContainer: map[apicommonv1.AgentContainerName]bool{
				apicommonv1.UnprivilegedSingleAgentContainerName: false,
				apicommonv1.CoreAgentContainerName:               true,
				apicommonv1.ProcessAgentContainerName:            true,
				apicommonv1.TraceAgentContainerName:              true,
				apicommonv1.SystemProbeContainerName:             true,
				apicommonv1.SecurityAgentContainerName:           true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, requiredComponents := feature.BuildFeatures(tt.dda, &feature.Options{})

			assert.True(t, *requiredComponents.Agent.IsRequired)

			for name, required := range tt.wantAgentContainer {
				assert.Equal(t, required, wantAgentContainer(name, requiredComponents), "Check", name)
			}
		})
	}
}

func wantAgentContainer(wantedContainer apicommonv1.AgentContainerName, requiredComponents feature.RequiredComponents) bool {
	for _, agentContainerName := range requiredComponents.Agent.Containers {
		if agentContainerName == wantedContainer {
			return true
		}
	}
	return false
}
