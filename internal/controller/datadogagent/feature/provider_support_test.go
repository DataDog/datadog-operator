// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package feature

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func TestFeatureSupportLevel(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		id       IDType
		want     SupportLevel
	}{
		{
			name:     "provider with no policy: everything supported",
			provider: kubernetes.GKECosProvider,
			id:       CWSIDType,
			want:     Supported,
		},
		{
			name:     "empty provider: supported",
			provider: "",
			id:       CWSIDType,
			want:     Supported,
		},
		{
			name:     "autopilot: CWS rejected",
			provider: kubernetes.GKEAutopilotProvider,
			id:       CWSIDType,
			want:     Rejected,
		},
		{
			name:     "autopilot: CSPM rejected",
			provider: kubernetes.GKEAutopilotProvider,
			id:       CSPMIDType,
			want:     Rejected,
		},
		{
			name:     "autopilot: SBOM degraded",
			provider: kubernetes.GKEAutopilotProvider,
			id:       SBOMIDType,
			want:     Degraded,
		},
		{
			name:     "autopilot: GPU degraded",
			provider: kubernetes.GKEAutopilotProvider,
			id:       GPUIDType,
			want:     Degraded,
		},
		{
			name:     "autopilot: unlisted feature supported (e.g. NPM, system-probe on modern GKE)",
			provider: kubernetes.GKEAutopilotProvider,
			id:       NPMIDType,
			want:     Supported,
		},
		{
			name:     "windows: allowlisted feature supported",
			provider: kubernetes.WindowsProvider,
			id:       APMIDType,
			want:     Supported,
		},
		{
			name:     "windows: base agent (default) supported",
			provider: kubernetes.WindowsProvider,
			id:       DefaultIDType,
			want:     Supported,
		},
		{
			name:     "windows: unlisted feature falls to Excluded default",
			provider: kubernetes.WindowsProvider,
			id:       NPMIDType,
			want:     Excluded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FeatureSupportLevel(tt.provider, tt.id); got != tt.want {
				t.Errorf("FeatureSupportLevel(%q, %q) = %v, want %v", tt.provider, tt.id, got, tt.want)
			}
		})
	}
}

// stubFeature is a no-op Feature with a fixed ID, for exercising EvaluateProviderSupport.
type stubFeature struct{ id IDType }

func (s stubFeature) ID() IDType { return s.id }
func (s stubFeature) Configure(metav1.Object, *v2alpha1.DatadogAgentSpec, *v2alpha1.RemoteConfigConfiguration) RequiredComponents {
	return RequiredComponents{}
}
func (s stubFeature) ManageDependencies(ResourceManagers) error                { return nil }
func (s stubFeature) ManageClusterAgent(PodTemplateManagers) error             { return nil }
func (s stubFeature) ManageNodeAgent(PodTemplateManagers) error                { return nil }
func (s stubFeature) ManageSingleContainerNodeAgent(PodTemplateManagers) error { return nil }
func (s stubFeature) ManageClusterChecksRunner(PodTemplateManagers) error      { return nil }
func (s stubFeature) ManageOtelAgentGateway(PodTemplateManagers) error         { return nil }

func TestEvaluateProviderSupport(t *testing.T) {
	autopilot := kubernetes.GKEAutopilotProvider
	feats := []Feature{
		stubFeature{id: CWSIDType},  // Rejected on autopilot
		stubFeature{id: SBOMIDType}, // Degraded on autopilot
		stubFeature{id: NPMIDType},  // Supported → omitted
		stubFeature{id: APMIDType},  // Supported → omitted
	}

	t.Run("empty provider yields nothing", func(t *testing.T) {
		if got := EvaluateProviderSupport(feats, ""); got != nil {
			t.Errorf("want nil, got %v", got)
		}
	})

	t.Run("reports only non-supported features", func(t *testing.T) {
		got := EvaluateProviderSupport(feats, autopilot)
		want := []ProviderSupportResult{
			{ID: CWSIDType, Level: Rejected},
			{ID: SBOMIDType, Level: Degraded},
		}
		if len(got) != len(want) {
			t.Fatalf("got %d results %v, want %d %v", len(got), got, len(want), want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("result[%d] = %+v, want %+v", i, got[i], want[i])
			}
		}
	})
}
