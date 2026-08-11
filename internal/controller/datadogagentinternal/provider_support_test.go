// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"testing"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
)

func TestProviderSupportBlocks(t *testing.T) {
	tests := []struct {
		name          string
		results       []feature.ProviderSupportResult
		seedCondition bool // pre-set a True condition, to test clearing
		wantBlocked   bool
		wantCondition metav1.ConditionStatus
	}{
		{
			name:          "rejected feature blocks and sets condition True",
			results:       []feature.ProviderSupportResult{{ID: feature.CWSIDType, Level: feature.Rejected}},
			wantBlocked:   true,
			wantCondition: metav1.ConditionTrue,
		},
		{
			name:          "degraded feature does not block, no condition written",
			results:       []feature.ProviderSupportResult{{ID: feature.SBOMIDType, Level: feature.Degraded}},
			wantBlocked:   false,
			wantCondition: "",
		},
		{
			name:          "no results does not block",
			results:       nil,
			wantBlocked:   false,
			wantCondition: "",
		},
		{
			name:          "mixed: rejected wins, blocks",
			results:       []feature.ProviderSupportResult{{ID: feature.SBOMIDType, Level: feature.Degraded}, {ID: feature.CSPMIDType, Level: feature.Rejected}},
			wantBlocked:   true,
			wantCondition: metav1.ConditionTrue,
		},
		{
			name:          "resolved: prior True condition cleared to False",
			results:       nil,
			seedCondition: true,
			wantBlocked:   false,
			wantCondition: metav1.ConditionFalse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{recorder: record.NewFakeRecorder(10)}
			instance := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Name: "dda", Namespace: "ns"}}
			newStatus := &v1alpha1.DatadogAgentInternalStatus{}
			now := metav1.Now()

			if tt.seedCondition {
				newStatus.Conditions = append(newStatus.Conditions, metav1.Condition{
					Type:   common.FeatureNotSupportedOnProviderConditionType,
					Status: metav1.ConditionTrue,
				})
			}

			blocked := r.providerSupportBlocks(logr.Discard(), instance, tt.results, newStatus, now)
			if blocked != tt.wantBlocked {
				t.Errorf("providerSupportBlocks() = %v, want %v", blocked, tt.wantBlocked)
			}
			if got := conditionStatus(newStatus, common.FeatureNotSupportedOnProviderConditionType); got != tt.wantCondition {
				t.Errorf("condition = %q, want %q", got, tt.wantCondition)
			}
		})
	}
}

func conditionStatus(status *v1alpha1.DatadogAgentInternalStatus, condType string) metav1.ConditionStatus {
	for _, c := range status.Conditions {
		if c.Type == condType {
			return c.Status
		}
	}
	return ""
}
