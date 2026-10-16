// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/composition"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/clusterchecks"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/dogstatsd"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/kubernetesstatecore"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/logcollection"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/otelagentgateway"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/remoteconfig"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/pkg/testutils"
)

var standaloneAnnotation = map[string]string{composition.ModeAnnotationKey: string(composition.ModeStandalone)}

func featureIDs(features []feature.Feature) []feature.IDType {
	ids := make([]feature.IDType, 0, len(features))
	for _, f := range features {
		ids = append(ids, f.ID())
	}
	return ids
}

func droppedIDs(dropped []feature.DroppedFeature) []feature.IDType {
	ids := make([]feature.IDType, 0, len(dropped))
	for _, d := range dropped {
		ids = append(ids, d.ID)
	}
	return ids
}

func TestBuildFeaturesNodeAgentMode(t *testing.T) {
	tests := []struct {
		name            string
		dda             *v2alpha1.DatadogAgent
		wantContainers  []common.AgentContainerName
		wantAgent       bool
		wantDCA         bool
		wantGateway     bool
		wantEnabled     []feature.IDType
		wantNotEnabled  []feature.IDType
		wantDropped     []feature.IDType
		wantNotDropped  []feature.IDType
		wantNoConfigued []feature.IDType
	}{
		{
			name: "default mode keeps the core Agent and the Cluster Agent",
			dda: testutils.NewDatadogAgentBuilder().
				WithOTelCollectorEnabled(true).
				BuildWithDefaults(),
			wantContainers: []common.AgentContainerName{common.CoreAgentContainerName, common.TraceAgentContainerName, common.OtelAgent},
			wantAgent:      true,
			wantDCA:        true,
			wantEnabled:    []feature.IDType{feature.DefaultIDType, feature.OtelAgentIDType, feature.APMIDType},
		},
		{
			name: "invalid mode value has no effect",
			dda: testutils.NewDatadogAgentBuilder().
				WithAnnotations(map[string]string{composition.ModeAnnotationKey: "ddot"}).
				WithOTelCollectorEnabled(true).
				BuildWithDefaults(),
			wantContainers: []common.AgentContainerName{common.CoreAgentContainerName, common.TraceAgentContainerName, common.OtelAgent},
			wantAgent:      true,
			wantDCA:        true,
		},
		{
			name: "standalone DDOT drops core-dependent features and the Cluster Agent",
			dda: testutils.NewDatadogAgentBuilder().
				WithAnnotations(standaloneAnnotation).
				WithOTelCollectorEnabled(true).
				WithAPMEnabled(true).
				WithLogCollectionEnabled(true).
				WithClusterChecksEnabled(true).
				BuildWithDefaults(),
			wantContainers:  []common.AgentContainerName{common.OtelAgent},
			wantAgent:       true,
			wantDCA:         false,
			wantEnabled:     []feature.IDType{feature.OtelAgentIDType},
			wantNotEnabled:  []feature.IDType{feature.DefaultIDType, feature.APMIDType, feature.LogCollectionIDType, feature.ClusterChecksIDType, feature.KubernetesStateCoreIDType},
			wantDropped:     []feature.IDType{feature.APMIDType, feature.LogCollectionIDType, feature.ClusterChecksIDType, feature.KubernetesStateCoreIDType, feature.RemoteConfigurationIDType},
			wantNotDropped:  []feature.IDType{feature.OtelAgentIDType},
			wantNoConfigued: []feature.IDType{feature.RemoteConfigurationIDType, feature.APMIDType},
		},
		{
			name: "standalone keeps the OTel Agent Gateway",
			dda: testutils.NewDatadogAgentBuilder().
				WithAnnotations(standaloneAnnotation).
				WithOTelCollectorEnabled(true).
				WithOTelAgentGatewayEnabled(true).
				BuildWithDefaults(),
			wantContainers: []common.AgentContainerName{common.OtelAgent},
			wantAgent:      true,
			wantGateway:    true,
			wantEnabled:    []feature.IDType{feature.OtelAgentIDType, feature.OtelAgentGatewayIDType},
		},
		{
			name: "standalone DDOT and host profiler",
			dda: testutils.NewDatadogAgentBuilder().
				WithAnnotations(map[string]string{
					composition.ModeAnnotationKey:             string(composition.ModeStandalone),
					featureutils.EnableHostProfilerAnnotation: "true",
				}).
				WithOTelCollectorEnabled(true).
				BuildWithDefaults(),
			wantContainers: []common.AgentContainerName{common.HostProfiler, common.OtelAgent},
			wantAgent:      true,
			wantEnabled:    []feature.IDType{feature.HostProfilerIDType, feature.OtelAgentIDType},
		},
		{
			name: "standalone host profiler only",
			dda: testutils.NewDatadogAgentBuilder().
				WithAnnotations(map[string]string{
					composition.ModeAnnotationKey:             string(composition.ModeStandalone),
					featureutils.EnableHostProfilerAnnotation: "true",
				}).
				BuildWithDefaults(),
			wantContainers: []common.AgentContainerName{common.HostProfiler},
			wantAgent:      true,
			wantEnabled:    []feature.IDType{feature.HostProfilerIDType},
		},
		{
			name: "standalone without standalone-capable features requires no node Agent",
			dda: testutils.NewDatadogAgentBuilder().
				WithAnnotations(standaloneAnnotation).
				BuildWithDefaults(),
			wantContainers: nil,
			wantAgent:      false,
		},
		{
			name: "standalone ignores the single container strategy",
			dda: testutils.NewDatadogAgentBuilder().
				WithAnnotations(standaloneAnnotation).
				WithSingleContainerStrategy(true).
				WithOTelCollectorEnabled(true).
				BuildWithDefaults(),
			wantContainers: []common.AgentContainerName{common.OtelAgent},
			wantAgent:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configured, enabled, required, _, dropped := feature.BuildFeatures(tt.dda, &tt.dda.Spec, tt.dda.Status.RemoteConfigConfiguration, &feature.Options{})

			if tt.wantContainers == nil {
				assert.Empty(t, required.Agent.Containers)
			} else {
				assert.Equal(t, tt.wantContainers, required.Agent.Containers)
			}
			assert.Equal(t, tt.wantAgent, required.Agent.IsEnabled())
			assert.Equal(t, tt.wantDCA, required.ClusterAgent.IsEnabled())
			assert.False(t, !tt.wantDCA && required.ClusterChecksRunner.IsEnabled(), "Cluster Checks Runner must be disabled without the Cluster Agent")
			assert.Equal(t, tt.wantGateway, required.OtelAgentGateway.IsEnabled())

			enabledIDs := featureIDs(enabled)
			configuredIDs := featureIDs(configured)
			for _, id := range tt.wantEnabled {
				assert.Contains(t, enabledIDs, id)
			}
			for _, id := range tt.wantNotEnabled {
				assert.NotContains(t, enabledIDs, id)
			}
			for _, id := range tt.wantDropped {
				assert.Contains(t, droppedIDs(dropped), id)
			}
			for _, id := range tt.wantNotDropped {
				assert.NotContains(t, droppedIDs(dropped), id)
			}
			for _, id := range tt.wantNoConfigued {
				assert.NotContains(t, configuredIDs, id)
			}
			if !composition.PolicyOf(tt.dda).Composed() {
				assert.Empty(t, dropped)
			}
		})
	}
}
