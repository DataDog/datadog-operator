// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestratorexplorer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
)

func TestRBACBuilderFromCustomResourceStrings(t *testing.T) {
	for _, tt := range []struct {
		name            string
		customResources []string
		expectedRules   []rbacv1.PolicyRule
	}{
		{
			name:            "empty crs",
			customResources: []string{},
			expectedRules:   nil,
		},
		{
			name:            "two crs, same group",
			customResources: []string{"datadoghq.com/v1alpha1/datadogmetrics", "datadoghq.com/v1alpha1/watermarkpodautoscalers"},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"datadogmetrics", "watermarkpodautoscalers"},
				},
			},
		},
		{
			name:            "three crs, different groups",
			customResources: []string{"datadoghq.com/v1alpha1/datadogmetrics", "datadoghq.com/v1alpha1/watermarkpodautoscalers", "cilium.io/v1/ciliumendpoints"},
			expectedRules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"cilium.io"},
					Resources: []string{"ciliumendpoints"},
				},
				{
					APIGroups: []string{"datadoghq.com"},
					Resources: []string{"datadogmetrics", "watermarkpodautoscalers"},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rbacBuilder := utils.NewRBACBuilder()
			for _, cr := range tt.customResources {
				crSplit := strings.Split(cr, "/")
				rbacBuilder.AddGroupKind(crSplit[0], crSplit[2])
			}
			actualRules := rbacBuilder.Build()
			assert.Equal(t, tt.expectedRules, actualRules)
		})
	}
}
