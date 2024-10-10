package test

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	v2alpha1test "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1/test"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/apm"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/cspm"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/livecontainer"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/npm"
)

func TestBuilder(t *testing.T) {

	tests := []struct {
		name                   string
		dda                    *v2alpha1.DatadogAgent
		wantCoreAgentComponent bool
		wantAgentContainer     map[common.AgentContainerName]bool
	}{
		{
			// This test relies on the fact that by default Live Container feature is enabled
			// in the default settings which enables process agent.
			name: "Default DDA, Core and Process agent enabled",
			dda: v2alpha1test.NewDatadogAgentBuilder().
				BuildWithDefaults(),
			wantAgentContainer: map[common.AgentContainerName]bool{
				common.UnprivilegedSingleAgentContainerName: false,
				common.CoreAgentContainerName:               true,
				common.ProcessAgentContainerName:            true,
				common.TraceAgentContainerName:              true,
				common.SystemProbeContainerName:             false,
				common.SecurityAgentContainerName:           false,
			},
		},
		{
			name: "Default DDA with single container strategy, 1 single container",
			dda: v2alpha1test.NewDatadogAgentBuilder().
				WithSingleContainerStrategy(true).
				BuildWithDefaults(),
			wantAgentContainer: map[common.AgentContainerName]bool{
				common.UnprivilegedSingleAgentContainerName: true,
				common.CoreAgentContainerName:               false,
				common.ProcessAgentContainerName:            false,
				common.TraceAgentContainerName:              false,
				common.SystemProbeContainerName:             false,
				common.SecurityAgentContainerName:           false,
			},
		},
		{
			name: "APM enabled, 3 agents",
			dda: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				BuildWithDefaults(),
			wantAgentContainer: map[common.AgentContainerName]bool{
				common.UnprivilegedSingleAgentContainerName: false,
				common.CoreAgentContainerName:               true,
				common.ProcessAgentContainerName:            true,
				common.TraceAgentContainerName:              true,
				common.SystemProbeContainerName:             false,
				common.SecurityAgentContainerName:           false,
			},
		},
		{
			name: "APM enabled with single container strategy, 1 single container",
			dda: v2alpha1test.NewDatadogAgentBuilder().
				WithSingleContainerStrategy(true).
				WithAPMEnabled(true).
				BuildWithDefaults(),
			wantAgentContainer: map[common.AgentContainerName]bool{
				common.UnprivilegedSingleAgentContainerName: true,
				common.CoreAgentContainerName:               false,
				common.ProcessAgentContainerName:            false,
				common.TraceAgentContainerName:              false,
				common.SystemProbeContainerName:             false,
				common.SecurityAgentContainerName:           false,
			},
		},
		{
			name: "APM, NPM enabled, 4 agents",
			dda: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithNPMEnabled(true).
				BuildWithDefaults(),
			wantAgentContainer: map[common.AgentContainerName]bool{
				common.UnprivilegedSingleAgentContainerName: false,
				common.CoreAgentContainerName:               true,
				common.ProcessAgentContainerName:            true,
				common.TraceAgentContainerName:              true,
				common.SystemProbeContainerName:             true,
				common.SecurityAgentContainerName:           false,
			},
		},
		{
			name: "APM, NPM enabled with single container strategy, 4 agents",
			dda: v2alpha1test.NewDatadogAgentBuilder().
				WithSingleContainerStrategy(true).
				WithAPMEnabled(true).
				WithNPMEnabled(true).
				BuildWithDefaults(),
			wantAgentContainer: map[common.AgentContainerName]bool{
				common.UnprivilegedSingleAgentContainerName: false,
				common.CoreAgentContainerName:               true,
				common.ProcessAgentContainerName:            true,
				common.TraceAgentContainerName:              true,
				common.SystemProbeContainerName:             true,
				common.SecurityAgentContainerName:           false,
			},
		},
		{
			name: "APM, NPM, CSPM enabled, 5 agents",
			dda: v2alpha1test.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				WithNPMEnabled(true).
				WithCSPMEnabled(true).
				BuildWithDefaults(),
			wantAgentContainer: map[common.AgentContainerName]bool{
				common.UnprivilegedSingleAgentContainerName: false,
				common.CoreAgentContainerName:               true,
				common.ProcessAgentContainerName:            true,
				common.TraceAgentContainerName:              true,
				common.SystemProbeContainerName:             true,
				common.SecurityAgentContainerName:           true,
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
			wantAgentContainer: map[common.AgentContainerName]bool{
				common.UnprivilegedSingleAgentContainerName: false,
				common.CoreAgentContainerName:               true,
				common.ProcessAgentContainerName:            true,
				common.TraceAgentContainerName:              true,
				common.SystemProbeContainerName:             true,
				common.SecurityAgentContainerName:           true,
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

func wantAgentContainer(wantedContainer common.AgentContainerName, requiredComponents feature.RequiredComponents) bool {
	for _, agentContainerName := range requiredComponents.Agent.Containers {
		if agentContainerName == wantedContainer {
			return true
		}
	}
	return false
}
