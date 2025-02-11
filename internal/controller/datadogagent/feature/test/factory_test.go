package test

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/pkg/testutils"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/apm"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/cspm"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/enabledefault"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/gpu"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/livecontainer"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/npm"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/otelcollector"
)

func TestBuilder(t *testing.T) {

	tests := []struct {
		name                   string
		dda                    *v2alpha1.DatadogAgent
		featureOptions         feature.Options
		wantCoreAgentComponent bool
		wantAgentContainer     map[common.AgentContainerName]bool
	}{
		{
			name: "Default DDA",
			dda: testutils.NewDatadogAgentBuilder().
				BuildWithDefaults(),
			wantAgentContainer: map[common.AgentContainerName]bool{
				common.UnprivilegedSingleAgentContainerName: false,
				common.CoreAgentContainerName:               true,
				common.ProcessAgentContainerName:            false,
				common.TraceAgentContainerName:              true,
				common.SystemProbeContainerName:             false,
				common.SecurityAgentContainerName:           false,
				common.OtelAgent:                            false,
				common.AgentDataPlaneContainerName:          false,
			},
		},
		{
			name: "Container monitoring on Process agent",
			dda: testutils.NewDatadogAgentBuilder().
				WithProcessChecksInCoreAgent(false).
				BuildWithDefaults(),
			wantAgentContainer: map[common.AgentContainerName]bool{
				common.UnprivilegedSingleAgentContainerName: false,
				common.CoreAgentContainerName:               true,
				common.ProcessAgentContainerName:            true,
				common.TraceAgentContainerName:              true,
				common.SystemProbeContainerName:             false,
				common.SecurityAgentContainerName:           false,
				common.OtelAgent:                            false,
				common.AgentDataPlaneContainerName:          false,
			},
		},
		{
			name: "Default DDA with single container strategy, 1 single container",
			dda: testutils.NewDatadogAgentBuilder().
				WithSingleContainerStrategy(true).
				BuildWithDefaults(),
			wantAgentContainer: map[common.AgentContainerName]bool{
				common.UnprivilegedSingleAgentContainerName: true,
				common.CoreAgentContainerName:               false,
				common.ProcessAgentContainerName:            false,
				common.TraceAgentContainerName:              false,
				common.SystemProbeContainerName:             false,
				common.SecurityAgentContainerName:           false,
				common.OtelAgent:                            false,
				common.AgentDataPlaneContainerName:          false,
			},
		},
		{
			name: "APM enabled, 2 agents",
			dda: testutils.NewDatadogAgentBuilder().
				WithAPMEnabled(true).
				BuildWithDefaults(),
			wantAgentContainer: map[common.AgentContainerName]bool{
				common.UnprivilegedSingleAgentContainerName: false,
				common.CoreAgentContainerName:               true,
				common.ProcessAgentContainerName:            false,
				common.TraceAgentContainerName:              true,
				common.SystemProbeContainerName:             false,
				common.SecurityAgentContainerName:           false,
				common.OtelAgent:                            false,
				common.AgentDataPlaneContainerName:          false,
			},
		},
		{
			name: "APM enabled with single container strategy, 1 single container",
			dda: testutils.NewDatadogAgentBuilder().
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
				common.OtelAgent:                            false,
				common.AgentDataPlaneContainerName:          false,
			},
		},
		{
			name: "APM, NPM enabled, 4 agents",
			dda: testutils.NewDatadogAgentBuilder().
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
				common.OtelAgent:                            false,
				common.AgentDataPlaneContainerName:          false,
			},
		},
		{
			name: "APM, NPM enabled with single container strategy, 4 agents",
			dda: testutils.NewDatadogAgentBuilder().
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
				common.OtelAgent:                            false,
				common.AgentDataPlaneContainerName:          false,
			},
		},
		{
			name: "APM, NPM, CSPM enabled, 5 agents",
			dda: testutils.NewDatadogAgentBuilder().
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
				common.OtelAgent:                            false,
				common.AgentDataPlaneContainerName:          false,
			},
		},
		{
			name: "APM, NPM, CSPM enabled with single container strategy, 5 agents",
			dda: testutils.NewDatadogAgentBuilder().
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
				common.OtelAgent:                            false,
				common.AgentDataPlaneContainerName:          false,
			},
		},
		{
			name: "Default DDA, otel collector feature enabled",
			dda: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				BuildWithDefaults(),
			wantAgentContainer: map[common.AgentContainerName]bool{
				common.UnprivilegedSingleAgentContainerName: false,
				common.CoreAgentContainerName:               true,
				common.ProcessAgentContainerName:            false,
				common.TraceAgentContainerName:              true,
				common.SystemProbeContainerName:             false,
				common.SecurityAgentContainerName:           false,
				common.OtelAgent:                            true,
				common.AgentDataPlaneContainerName:          false,
			},
		},
		{
			name: "Default DDA, otel collector feature disabled",
			dda: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(false).
				BuildWithDefaults(),
			wantAgentContainer: map[common.AgentContainerName]bool{
				common.UnprivilegedSingleAgentContainerName: false,
				common.CoreAgentContainerName:               true,
				common.ProcessAgentContainerName:            false,
				common.TraceAgentContainerName:              true,
				common.SystemProbeContainerName:             false,
				common.SecurityAgentContainerName:           false,
				common.OtelAgent:                            false,
				common.AgentDataPlaneContainerName:          false,
			},
		},
		{
			name: "Default DDA, default feature Option, adp-enabled annotation true",
			dda: testutils.NewDatadogAgentBuilder().
				WithAnnotations(map[string]string{"agent.datadoghq.com/adp-enabled": "true"}).
				BuildWithDefaults(),
			wantAgentContainer: map[common.AgentContainerName]bool{
				common.UnprivilegedSingleAgentContainerName: false,
				common.CoreAgentContainerName:               true,
				common.ProcessAgentContainerName:            false,
				common.TraceAgentContainerName:              true,
				common.SystemProbeContainerName:             false,
				common.SecurityAgentContainerName:           false,
				common.OtelAgent:                            false,
				common.AgentDataPlaneContainerName:          true,
			},
		},
		{
			name: "Default DDA, default feature Option, adp-enabled annotation false",
			dda: testutils.NewDatadogAgentBuilder().
				WithAnnotations(map[string]string{"agent.datadoghq.com/adp-enabled": "false"}).
				BuildWithDefaults(),
			wantAgentContainer: map[common.AgentContainerName]bool{
				common.UnprivilegedSingleAgentContainerName: false,
				common.CoreAgentContainerName:               true,
				common.ProcessAgentContainerName:            false,
				common.TraceAgentContainerName:              true,
				common.SystemProbeContainerName:             false,
				common.SecurityAgentContainerName:           false,
				common.OtelAgent:                            false,
				common.AgentDataPlaneContainerName:          false,
			},
		},
		{
			name: "GPU monitoring enabled, 3 agents",
			dda: testutils.NewDatadogAgentBuilder().
				WithGPUMonitoringEnabled(true).
				BuildWithDefaults(),
			wantAgentContainer: map[common.AgentContainerName]bool{
				common.UnprivilegedSingleAgentContainerName: false,
				common.CoreAgentContainerName:               true,
				common.ProcessAgentContainerName:            false,
				common.TraceAgentContainerName:              true,
				common.SystemProbeContainerName:             true,
				common.SecurityAgentContainerName:           false,
				common.OtelAgent:                            false,
				common.AgentDataPlaneContainerName:          false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, requiredComponents := feature.BuildFeatures(tt.dda, &tt.featureOptions)

			assert.True(t, *requiredComponents.Agent.IsRequired)

			for name, required := range tt.wantAgentContainer {
				assert.Equal(t, required, wantAgentContainer(name, requiredComponents), "container %s", name)
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
