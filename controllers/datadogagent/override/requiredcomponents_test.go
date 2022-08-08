// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"testing"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/stretchr/testify/assert"
)

func TestRequiredComponents(t *testing.T) {
	tests := []struct {
		name                       string
		requiredComponents         feature.RequiredComponents
		overrides                  map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride
		expectedRequiredComponents feature.RequiredComponents
	}{
		{
			name: "disable agent",
			requiredComponents: feature.RequiredComponents{
				Agent: feature.RequiredComponent{
					IsRequired: apiutils.NewBoolPointer(true),
				},
			},
			overrides: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.NodeAgentComponentName: {
					Disabled: apiutils.NewBoolPointer(true),
				},
			},
			expectedRequiredComponents: feature.RequiredComponents{
				Agent: feature.RequiredComponent{
					IsRequired: apiutils.NewBoolPointer(false),
				},
			},
		},
		{
			name: "disable cluster agent",
			requiredComponents: feature.RequiredComponents{
				ClusterAgent: feature.RequiredComponent{
					IsRequired: apiutils.NewBoolPointer(true),
				},
			},
			overrides: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.ClusterAgentComponentName: {
					Disabled: apiutils.NewBoolPointer(true),
				},
			},
			expectedRequiredComponents: feature.RequiredComponents{
				ClusterAgent: feature.RequiredComponent{
					IsRequired: apiutils.NewBoolPointer(false),
				},
			},
		},
		{
			name: "disable cluster checks runner",
			requiredComponents: feature.RequiredComponents{
				ClusterChecksRunner: feature.RequiredComponent{
					IsRequired: apiutils.NewBoolPointer(true),
				},
			},
			overrides: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.ClusterChecksRunnerComponentName: {
					Disabled: apiutils.NewBoolPointer(true),
				},
			},
			expectedRequiredComponents: feature.RequiredComponents{
				ClusterChecksRunner: feature.RequiredComponent{
					IsRequired: apiutils.NewBoolPointer(false),
				},
			},
		},
		{
			name: "don't disable",
			requiredComponents: feature.RequiredComponents{
				Agent: feature.RequiredComponent{
					IsRequired: apiutils.NewBoolPointer(true),
				},
			},
			overrides: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.NodeAgentComponentName: {
					Disabled: apiutils.NewBoolPointer(false),
				},
			},
			expectedRequiredComponents: feature.RequiredComponents{
				Agent: feature.RequiredComponent{
					IsRequired: apiutils.NewBoolPointer(true),
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			RequiredComponents(&test.requiredComponents, test.overrides)

			assert.Equal(t, test.expectedRequiredComponents, test.requiredComponents)
		})
	}
}
