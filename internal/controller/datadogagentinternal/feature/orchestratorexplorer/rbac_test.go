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
		expected        map[string][]string
	}{
		{
			name:            "empty crs",
			customResources: []string{},
			expected:        map[string][]string{},
		},
		{
			name:            "two crs, same group",
			customResources: []string{"datadoghq.com/v1alpha1/datadogmetrics", "datadoghq.com/v1alpha1/watermarkpodautoscalers"},
			expected: map[string][]string{
				"datadoghq.com": {"datadogmetrics", "watermarkpodautoscalers"},
			},
		},
		{
			name:            "three crs, different groups",
			customResources: []string{"datadoghq.com/v1alpha1/datadogmetrics", "datadoghq.com/v1alpha1/watermarkpodautoscalers", "cilium.io/v1/ciliumendpoints"},
			expected: map[string][]string{
				"datadoghq.com": {"datadogmetrics", "watermarkpodautoscalers"},
				"cilium.io":     {"ciliumendpoints"},
			},
		},
	} {
		actualGroupsResources := mapAPIGroupsResources(logr.Logger{}, tt.customResources)
		assert.Equal(t, tt.expected, actualGroupsResources)
	}

}
