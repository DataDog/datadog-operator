// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package remoteconfig

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func TestGetAndUpdateDatadogAgentWithRetry_CRD(t *testing.T) {
	tests := []struct {
		name         string
		client       *mockClient
		expectedRuns int
	}{
		{
			name: "success on first try",
			client: &mockClient{
				mockSubResourceWriter: &mockSubResourceWriter{
					maxRetries: 1,
				},
			},
			expectedRuns: 1,
		},
		{
			name: "success after conflicts",
			client: &mockClient{
				mockSubResourceWriter: &mockSubResourceWriter{
					maxRetries:   3,
					conflictMode: true,
				},
			},
			expectedRuns: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updater := NewRemoteConfigUpdater(tt.client, logr.Logger{})
			config := OrchestratorK8sCRDRemoteConfig{
				CRDs: &CustomResourceDefinitionURLs{
					Crds: &[]string{"custom-resource-1", "custom-resource-2"},
				},
			}

			err := updater.getAndUpdateDatadogAgentWithRetry(context.Background(), config, updater.crdUpdateInstanceStatus)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedRuns, tt.client.mockSubResourceWriter.updateCount)
		})
	}
}

type mockClient struct {
	client.Client
	listError             error
	mockSubResourceWriter *mockSubResourceWriter
}

func (m *mockClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if m.listError == nil {
		ddaList := list.(*v2alpha1.DatadogAgentList)
		ddaList.Items = []v2alpha1.DatadogAgent{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "datadog-agent-test",
				},
			},
		}
	}
	return m.listError
}

func (m *mockClient) Status() client.SubResourceWriter {
	return m.mockSubResourceWriter
}

type mockSubResourceWriter struct {
	client.SubResourceWriter
	updateCount  int
	maxRetries   int
	conflictMode bool
}

func (m *mockSubResourceWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	m.updateCount++
	if m.conflictMode && m.updateCount < m.maxRetries {
		return apierrors.NewConflict(schema.GroupResource{}, "test", errors.New("conflict"))
	}
	return nil
}
