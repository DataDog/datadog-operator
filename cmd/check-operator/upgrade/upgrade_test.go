// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package upgrade

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	controllercommon "github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	plugincommon "github.com/DataDog/datadog-operator/pkg/plugin/common"
)

func TestGetDDAIs(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	matchingDDAI := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{
		Name:      "agent",
		Namespace: "test-namespace",
		Labels:    map[string]string{apicommon.DatadogAgentNameLabelKey: "agent"},
	}}
	matchingProfileDDAI := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{
		Name:      "agent-profile-a",
		Namespace: "test-namespace",
		Labels:    map[string]string{apicommon.DatadogAgentNameLabelKey: "agent"},
	}}
	otherDDAI := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{
		Name:      "other-agent",
		Namespace: "test-namespace",
		Labels:    map[string]string{apicommon.DatadogAgentNameLabelKey: "other"},
	}}
	otherNamespaceDDAI := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{
		Name:      "agent",
		Namespace: "other-namespace",
		Labels:    map[string]string{apicommon.DatadogAgentNameLabelKey: "agent"},
	}}

	o := &Options{
		Options: plugincommon.Options{
			Client: fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(matchingDDAI, matchingProfileDDAI, otherDDAI, otherNamespaceDDAI).
				Build(),
			UserNamespace: "test-namespace",
		},
		datadogAgentName: "agent",
	}

	ddais, err := o.getDDAIs(context.Background())
	require.NoError(t, err)
	require.Len(t, ddais, 2)
	assert.ElementsMatch(t, []string{matchingDDAI.Name, matchingProfileDDAI.Name}, []string{ddais[0].Name, ddais[1].Name})
}

func TestIsReconcileError(t *testing.T) {
	tests := []struct {
		name       string
		conditions []metav1.Condition
		wantError  string
	}{
		{
			name: "DDAI error propagated to DDA",
			conditions: []metav1.Condition{{
				Type:    controllercommon.DatadogAgentInternalReconcileErrorConditionType,
				Status:  metav1.ConditionTrue,
				Message: "agent: forbidden",
			}},
			wantError: "datadogAgentInternal reconciliation error message: agent: forbidden",
		},
		{
			name: "DDAI direct reconcile error",
			conditions: []metav1.Condition{{
				Type:    controllercommon.DatadogAgentReconcileErrorConditionType,
				Status:  metav1.ConditionTrue,
				Message: "forbidden",
			}},
			wantError: "datadogAgent reconciliation error message: forbidden",
		},
		{
			name: "resolved DDAI reconcile error",
			conditions: []metav1.Condition{{
				Type:    controllercommon.DatadogAgentReconcileErrorConditionType,
				Status:  metav1.ConditionFalse,
				Message: "reconcile ok",
			}},
		},
		{
			name: "agent component error",
			conditions: []metav1.Condition{{
				Type:    controllercommon.AgentReconcileConditionType,
				Status:  metav1.ConditionFalse,
				Message: "daemonset failed",
			}},
			wantError: "agent reconciliation error message: daemonset failed",
		},
		{
			name: "cluster agent component error",
			conditions: []metav1.Condition{{
				Type:    controllercommon.ClusterAgentReconcileConditionType,
				Status:  metav1.ConditionFalse,
				Message: "deployment failed",
			}},
			wantError: "cluster Agent reconciliation error message: deployment failed",
		},
		{
			name: "cluster checks runner component error",
			conditions: []metav1.Condition{{
				Type:    controllercommon.ClusterChecksRunnerReconcileConditionType,
				Status:  metav1.ConditionFalse,
				Message: "deployment failed",
			}},
			wantError: "cluster Check Runner reconciliation error message: deployment failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := isReconcileError(tt.conditions)
			if tt.wantError == "" {
				assert.NoError(t, err)
				return
			}
			require.EqualError(t, err, tt.wantError)
		})
	}
}
