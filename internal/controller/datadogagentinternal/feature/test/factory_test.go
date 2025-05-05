package test

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/testutils"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/apm"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/cspm"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/enabledefault"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/gpu"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/livecontainer"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/npm"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/otelcollector"
)

func TestBuilder(t *testing.T) {

	tests := []struct {
		name                   string
		ddai                   *v1alpha1.DatadogAgentInternal
		featureOptions         feature.Options
		wantCoreAgentComponent bool
		wantAgentContainer     map[common.AgentContainerName]bool
	}{
		{
			name: "Default DDA",
			ddai: testutils.NewDatadogAgentInternalBuilder().
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
			ddai: testutils.NewDatadogAgentInternalBuilder().
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
			ddai: testutils.NewDatadogAgentInternalBuilder().
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
			ddai: testutils.NewDatadogAgentInternalBuilder().
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
			ddai: testutils.NewDatadogAgentInternalBuilder().
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
			ddai: testutils.NewDatadogAgentInternalBuilder().
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
			ddai: testutils.NewDatadogAgentInternalBuilder().
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
			ddai: testutils.NewDatadogAgentInternalBuilder().
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
			ddai: testutils.NewDatadogAgentInternalBuilder().
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
			ddai: testutils.NewDatadogAgentInternalBuilder().
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
			ddai: testutils.NewDatadogAgentInternalBuilder().
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
			ddai: testutils.NewDatadogAgentInternalBuilder().
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
			ddai: testutils.NewDatadogAgentInternalBuilder().
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
			ddai: testutils.NewDatadogAgentInternalBuilder().
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
			_, _, requiredComponents := feature.BuildFeatures(tt.ddai, &tt.featureOptions)

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
