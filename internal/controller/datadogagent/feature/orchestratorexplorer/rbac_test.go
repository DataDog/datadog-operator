// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestratorexplorer

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
)

func TestMapAPIGroupsResources(t *testing.T) {
	for _, tt := range []struct {
		name            string
		customResources []string
		expected        []groupResources
	}{
		{
			name:            "empty crs",
			customResources: []string{},
			expected:        []groupResources{},
		},
		{
			name:            "two crs, same group",
			customResources: []string{"datadoghq.com/v1alpha1/datadogmetrics", "datadoghq.com/v1alpha1/watermarkpodautoscalers"},
			expected: []groupResources{
				{
					group:     "datadoghq.com",
					resources: []string{"datadogmetrics", "watermarkpodautoscalers"},
				},
			},
		},
		{
			name:            "three crs, different groups",
			customResources: []string{"datadoghq.com/v1alpha1/datadogmetrics", "datadoghq.com/v1alpha1/watermarkpodautoscalers", "cilium.io/v1/ciliumendpoints"},
			expected: []groupResources{
				{
					group:     "cilium.io",
					resources: []string{"ciliumendpoints"},
				},
				{
					group:     "datadoghq.com",
					resources: []string{"datadogmetrics", "watermarkpodautoscalers"},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			actualGroupsResources := mapAPIGroupsResources(logr.Logger{}, tt.customResources)
			assert.Equal(t, tt.expected, actualGroupsResources)
		})
	}
}
