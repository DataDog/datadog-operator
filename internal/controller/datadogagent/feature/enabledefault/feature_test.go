// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package enabledefault

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	mergerfake "github.com/DataDog/datadog-operator/internal/controller/datadogagent/merger/fake"
	"github.com/DataDog/datadog-operator/pkg/testutils"
)

func Test_defaultFeature_ClusterAgentEnabledEnvVar(t *testing.T) {
	tests := []struct {
		name                  string
		clusterAgentDisabled  bool
		wantClusterAgentValue string
	}{
		{
			name:                  "cluster agent enabled (default)",
			clusterAgentDisabled:  false,
			wantClusterAgentValue: "",
		},
		{
			name:                  "cluster agent disabled",
			clusterAgentDisabled:  true,
			wantClusterAgentValue: "false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := testutils.NewDatadogAgentBuilder()
			if tt.clusterAgentDisabled {
				builder = builder.WithClusterAgentDisabled(true)
			}
			dda := builder.Build()

			f := buildDefaultFeature(nil).(*defaultFeature)
			f.Configure(dda, &dda.Spec, nil)

			mgr := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "agent"},
					},
				},
			})

			err := f.ManageNodeAgent(mgr)
			assert.NoError(t, err)

			envVars := mgr.EnvVarMgr.EnvVarsByC[mergerfake.AllContainers]
			var gotValue string
			var found bool
			for _, e := range envVars {
				if e.Name == common.DDClusterAgentEnabled {
					gotValue = e.Value
					found = true
				}
			}

			if tt.clusterAgentDisabled {
				assert.True(t, found, "expected DD_CLUSTER_AGENT_ENABLED to be explicitly overridden")
				assert.Equal(t, tt.wantClusterAgentValue, gotValue)
			} else {
				assert.False(t, found, "should not override DD_CLUSTER_AGENT_ENABLED when the cluster agent is enabled")
			}

			initEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.InitConfigContainerName]
			var initFound bool
			var initValue string
			for _, e := range initEnvVars {
				if e.Name == common.DDClusterAgentEnabled {
					initFound = true
					initValue = e.Value
				}
			}
			if tt.clusterAgentDisabled {
				assert.True(t, initFound, "expected DD_CLUSTER_AGENT_ENABLED to be explicitly overridden on the init-config container")
				assert.Equal(t, tt.wantClusterAgentValue, initValue)
			} else {
				assert.False(t, initFound, "should not override DD_CLUSTER_AGENT_ENABLED on the init-config container when the cluster agent is enabled")
			}
		})
	}
}

// Test_defaultFeature_Configure_ClusterAgentRequired verifies that, when the Cluster Agent is
// disabled via override, Configure resolves ClusterAgent.IsRequired to an explicit false rather
// than leaving it true (or nil). An explicit false merges with any other feature's requirement
// as "component is disabled" rather than "feature wants it but override disables it", so
// checkComponentEnabledWithOverride resolves this as a clean disable instead of persisting an
// OverrideReconcileConflict status condition on an otherwise legitimate no-Cluster-Agent setup.
func Test_defaultFeature_Configure_ClusterAgentRequired(t *testing.T) {
	tests := []struct {
		name                 string
		clusterAgentDisabled bool
		wantIsEnabled        bool
	}{
		{name: "cluster agent enabled (default)", clusterAgentDisabled: false, wantIsEnabled: true},
		{name: "cluster agent disabled", clusterAgentDisabled: true, wantIsEnabled: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := testutils.NewDatadogAgentBuilder()
			if tt.clusterAgentDisabled {
				builder = builder.WithClusterAgentDisabled(true)
			}
			dda := builder.Build()

			f := buildDefaultFeature(nil)
			reqComp := f.Configure(dda, &dda.Spec, nil)

			assert.Equal(t, tt.wantIsEnabled, reqComp.ClusterAgent.IsEnabled())
			if tt.clusterAgentDisabled {
				assert.NotNil(t, reqComp.ClusterAgent.IsRequired)
				assert.False(t, *reqComp.ClusterAgent.IsRequired)
			}
		})
	}
}
