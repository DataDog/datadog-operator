// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package feature

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/composition"
)

// requiringFeature is a stub Feature returning fixed RequiredComponents.
type requiringFeature struct {
	stubFeature
	rc RequiredComponents
}

func (f requiringFeature) Configure(metav1.Object, *v2alpha1.DatadogAgentSpec, *v2alpha1.RemoteConfigConfiguration) RequiredComponents {
	return f.rc
}

func withFeatureBuilders(t *testing.T, features ...requiringFeature) {
	saved := featureBuilders
	featureBuilders = map[IDType]BuildFunc{}
	for _, f := range features {
		featureBuilders[f.id] = func(*Options) Feature { return f }
	}
	t.Cleanup(func() { featureBuilders = saved })
}

func standaloneObject() *metav1.ObjectMeta {
	return &metav1.ObjectMeta{Annotations: map[string]string{composition.ModeAnnotationKey: string(composition.ModeStandalone)}}
}

func TestPolicyViolations(t *testing.T) {
	standalone := composition.PolicyFor(composition.ModeStandalone)
	tests := []struct {
		name string
		rc   RequiredComponents
		want []string
	}{
		{
			name: "allowed containers",
			rc:   RequiredComponents{Agent: RequiredComponent{IsRequired: new(true), Containers: []apicommon.AgentContainerName{apicommon.OtelAgent, apicommon.HostProfiler}}},
		},
		{
			name: "core and trace containers",
			rc:   RequiredComponents{Agent: RequiredComponent{IsRequired: new(true), Containers: []apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.TraceAgentContainerName}}},
			want: []string{`node Agent container "agent"`, `node Agent container "trace-agent"`},
		},
		{
			name: "node Agent without containers implies the core Agent",
			rc:   RequiredComponents{Agent: RequiredComponent{IsRequired: new(true)}},
			want: []string{"node Agent core container"},
		},
		{
			name: "configured Cluster Agent",
			rc:   RequiredComponents{ClusterAgent: RequiredComponent{Containers: []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName}}},
			want: []string{"Cluster Agent"},
		},
		{
			name: "Cluster Checks Runner",
			rc:   RequiredComponents{ClusterChecksRunner: RequiredComponent{IsRequired: new(true)}},
			want: []string{"Cluster Checks Runner"},
		},
		{
			name: "explicitly disabled component is not a violation",
			rc:   RequiredComponents{ClusterAgent: RequiredComponent{IsRequired: new(false)}},
		},
		{
			name: "OTel Agent Gateway",
			rc:   RequiredComponents{OtelAgentGateway: RequiredComponent{IsRequired: new(true), Containers: []apicommon.AgentContainerName{apicommon.OtelAgent}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, policyViolations(standalone, tt.rc))
		})
	}

	t.Run("default policy allows everything", func(t *testing.T) {
		rc := RequiredComponents{
			Agent:        RequiredComponent{IsRequired: new(true), Containers: []apicommon.AgentContainerName{apicommon.CoreAgentContainerName}},
			ClusterAgent: RequiredComponent{IsRequired: new(true)},
		}
		assert.Empty(t, policyViolations(composition.PolicyFor(composition.ModeDefault), rc))
	})
}

func TestBuildFeaturesDropsDisallowedFeatures(t *testing.T) {
	otel := requiringFeature{stubFeature: stubFeature{id: OtelAgentIDType}, rc: RequiredComponents{
		Agent: RequiredComponent{IsRequired: new(true), Containers: []apicommon.AgentContainerName{apicommon.OtelAgent}},
	}}
	apm := requiringFeature{stubFeature: stubFeature{id: APMIDType}, rc: RequiredComponents{
		Agent: RequiredComponent{IsRequired: new(true), Containers: []apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.TraceAgentContainerName}},
	}}
	configuredOnly := requiringFeature{stubFeature: stubFeature{id: RemoteConfigurationIDType}, rc: RequiredComponents{
		Agent:        RequiredComponent{Containers: []apicommon.AgentContainerName{apicommon.CoreAgentContainerName}},
		ClusterAgent: RequiredComponent{Containers: []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName}},
	}}
	withFeatureBuilders(t, otel, apm, configuredOnly)

	spec := &v2alpha1.DatadogAgentSpec{}

	t.Run("standalone", func(t *testing.T) {
		configured, enabled, required, _, dropped := BuildFeatures(standaloneObject(), spec, nil, &Options{})

		assert.Equal(t, []Feature{otel}, enabled)
		assert.Empty(t, configured)
		assert.Equal(t, []apicommon.AgentContainerName{apicommon.OtelAgent}, required.Agent.Containers)
		assert.Empty(t, required.ClusterAgent.Containers)
		assert.Equal(t, []DroppedFeature{{ID: APMIDType, Reasons: []string{`node Agent container "agent"`, `node Agent container "trace-agent"`}}}, dropped)
	})

	t.Run("default", func(t *testing.T) {
		configured, enabled, required, _, dropped := BuildFeatures(&metav1.ObjectMeta{}, spec, nil, &Options{})

		assert.ElementsMatch(t, []Feature{otel, apm}, enabled)
		assert.Equal(t, []Feature{configuredOnly}, configured)
		assert.ElementsMatch(t, []apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.TraceAgentContainerName, apicommon.OtelAgent}, required.Agent.Containers)
		assert.Empty(t, dropped)
	})
}

func TestApplyDDASharedDependenciesSkipsContainersNotRun(t *testing.T) {
	saved := ddaSharedDependencies
	t.Cleanup(func() { ddaSharedDependencies = saved })
	ddaSharedDependencies = map[IDType]ddaSharedDependency{}

	var applied []IDType
	register := func(id IDType, container apicommon.AgentContainerName) {
		assert.NoError(t, RegisterDDASharedDependencies(id, container, func(metav1.Object, *v2alpha1.DatadogAgentSpec, metav1.Object, *v2alpha1.DatadogAgentSpec, ResourceManagers) error {
			applied = append(applied, id)
			return nil
		}))
	}
	register(APMIDType, apicommon.TraceAgentContainerName)
	register(DogstatsdIDType, apicommon.CoreAgentContainerName)
	register(OtelAgentIDType, apicommon.OtelAgent)

	assert.NoError(t, ApplyDDASharedDependencies(&metav1.ObjectMeta{}, nil, standaloneObject(), nil, nil))
	assert.Equal(t, []IDType{OtelAgentIDType}, applied)

	applied = nil
	assert.NoError(t, ApplyDDASharedDependencies(&metav1.ObjectMeta{}, nil, &metav1.ObjectMeta{}, nil, nil))
	assert.Equal(t, []IDType{APMIDType, DogstatsdIDType, OtelAgentIDType}, applied)
}
