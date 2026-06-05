// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

type fakeProviderReader struct {
	provider string
	detected bool
	inGrace  bool
}

func (f fakeProviderReader) Provider() (string, bool)           { return f.provider, f.detected }
func (f fakeProviderReader) InGracePeriod(_ time.Duration) bool { return f.inGrace }

func ddaWith(annotations map[string]string, statusProvider string) *v2alpha1.DatadogAgent {
	return &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{Annotations: annotations},
		Status:     v2alpha1.DatadogAgentStatus{ClusterProvider: statusProvider},
	}
}

func TestResolveClusterProvider(t *testing.T) {
	tests := []struct {
		name         string
		detector     ProviderReader
		instance     *v2alpha1.DatadogAgent
		wantProvider string
		wantSource   string
		wantHold     bool
	}{
		{
			name:         "user override wins over detection",
			detector:     fakeProviderReader{provider: "eks", detected: true},
			instance:     ddaWith(map[string]string{kubernetes.ProviderAnnotationKey: "openshift-rhcos"}, ""),
			wantProvider: "openshift-rhcos",
			wantSource:   clusterProviderSourceUser,
		},
		{
			name:         "user override honored verbatim even when default",
			detector:     fakeProviderReader{provider: "eks", detected: true},
			instance:     ddaWith(map[string]string{kubernetes.ProviderAnnotationKey: kubernetes.DefaultProvider}, "eks"),
			wantProvider: kubernetes.DefaultProvider,
			wantSource:   clusterProviderSourceUser,
		},
		{
			name:         "detection disabled (nil detector)",
			detector:     nil,
			instance:     ddaWith(nil, ""),
			wantProvider: "",
			wantSource:   clusterProviderSourceDisabled,
		},
		{
			name:         "live specific detection",
			detector:     fakeProviderReader{provider: "eks", detected: true},
			instance:     ddaWith(nil, ""),
			wantProvider: "eks",
			wantSource:   clusterProviderSourceDetected,
		},
		{
			name:         "no-downgrade: live default does not replace persisted specific",
			detector:     fakeProviderReader{provider: kubernetes.DefaultProvider, detected: true},
			instance:     ddaWith(nil, "eks"),
			wantProvider: "eks",
			wantSource:   clusterProviderSourceDetected,
		},
		{
			name:         "live specific replaces persisted specific",
			detector:     fakeProviderReader{provider: "openshift-rhcos", detected: true},
			instance:     ddaWith(nil, "eks"),
			wantProvider: "openshift-rhcos",
			wantSource:   clusterProviderSourceDetected,
		},
		{
			name:         "persisted fallback when not yet detected",
			detector:     fakeProviderReader{detected: false, inGrace: true},
			instance:     ddaWith(nil, "eks"),
			wantProvider: "eks",
			wantSource:   clusterProviderSourceDetected,
		},
		{
			name:     "hold within gate window with no signal and no status",
			detector: fakeProviderReader{detected: false, inGrace: true},
			instance: ddaWith(nil, ""),
			wantHold: true,
		},
		{
			name:         "gate elapsed proceeds with empty provider",
			detector:     fakeProviderReader{detected: false, inGrace: false},
			instance:     ddaWith(nil, ""),
			wantProvider: "",
			wantSource:   clusterProviderSourceNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{options: ReconcilerOptions{ClusterProviderDetector: tt.detector}}
			provider, source, hold := r.resolveClusterProvider(tt.instance)
			assert.Equal(t, tt.wantHold, hold)
			if tt.wantHold {
				return
			}
			assert.Equal(t, tt.wantProvider, provider)
			assert.Equal(t, tt.wantSource, source)
		})
	}
}

func TestSetClusterProviderStatus(t *testing.T) {
	now := metav1.Now()

	t.Run("disabled writes nothing", func(t *testing.T) {
		status := &v2alpha1.DatadogAgentStatus{}
		setClusterProviderStatus(status, "", clusterProviderSourceDisabled, now)
		assert.Equal(t, "", status.ClusterProvider)
		assert.Empty(t, status.Conditions)
	})

	t.Run("detected specific provider", func(t *testing.T) {
		status := &v2alpha1.DatadogAgentStatus{}
		setClusterProviderStatus(status, "eks", clusterProviderSourceDetected, now)
		assert.Equal(t, "eks", status.ClusterProvider)
		assert.Len(t, status.Conditions, 1)
		assert.Equal(t, "ProviderDetected", status.Conditions[0].Reason)
	})

	t.Run("user specified", func(t *testing.T) {
		status := &v2alpha1.DatadogAgentStatus{}
		setClusterProviderStatus(status, "openshift-rhcos", clusterProviderSourceUser, now)
		assert.Equal(t, "openshift-rhcos", status.ClusterProvider)
		assert.Equal(t, "UserSpecified", status.Conditions[0].Reason)
	})

	t.Run("no provider detected", func(t *testing.T) {
		status := &v2alpha1.DatadogAgentStatus{}
		setClusterProviderStatus(status, "", clusterProviderSourceNone, now)
		assert.Equal(t, "", status.ClusterProvider)
		assert.Equal(t, "NoProviderDetected", status.Conditions[0].Reason)
	})
}
