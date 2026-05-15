// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

func TestGetDefaultAgentClusterRolePolicyRules_KubeletUseAPIServer(t *testing.T) {
	tests := []struct {
		name                string
		kubeletUseAPIServer bool
		expectPodsRule      bool
	}{
		{name: "no kubelet api server -> no pods rule", kubeletUseAPIServer: false, expectPodsRule: false},
		{name: "kubelet api server -> pods get/list rule", kubeletUseAPIServer: true, expectPodsRule: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := GetDefaultAgentClusterRolePolicyRules(false, false, tt.kubeletUseAPIServer)

			found := false
			for _, r := range rules {
				for _, res := range r.Resources {
					if res != rbac.PodsResource {
						continue
					}
					found = true
					assert.ElementsMatch(t, []string{rbac.GetVerb, rbac.ListVerb}, r.Verbs,
						"pods rule should grant get and list")
					assert.Equal(t, []string{rbac.CoreAPIGroup}, r.APIGroups)
				}
			}
			assert.Equal(t, tt.expectPodsRule, found)
		})
	}
}
