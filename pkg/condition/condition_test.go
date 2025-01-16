// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package condition

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	assert "github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
)

func TestDeleteDatadogAgentStatusCondition(t *testing.T) {
	type args struct {
		status    *v2alpha1.DatadogAgentStatus
		condition string
	}
	tests := []struct {
		name           string
		args           args
		expectedStatus *v2alpha1.DatadogAgentStatus
	}{
		{
			name: "empty status",
			args: args{
				status:    &v2alpha1.DatadogAgentStatus{},
				condition: "fooType",
			},
			expectedStatus: &v2alpha1.DatadogAgentStatus{},
		},
		{
			name: "not present status",
			args: args{
				status: &v2alpha1.DatadogAgentStatus{
					Conditions: []v1.Condition{
						{
							Type: "barType",
						},
					},
				},
				condition: "fooType",
			},
			expectedStatus: &v2alpha1.DatadogAgentStatus{
				Conditions: []v1.Condition{
					{
						Type: "barType",
					},
				},
			},
		},
		{
			name: "status present at the end",
			args: args{
				status: &v2alpha1.DatadogAgentStatus{
					Conditions: []v1.Condition{
						{
							Type: "barType",
						},
						{
							Type: "fooType",
						},
					},
				},
				condition: "fooType",
			},
			expectedStatus: &v2alpha1.DatadogAgentStatus{
				Conditions: []v1.Condition{
					{
						Type: "barType",
					},
				},
			},
		},
		{
			name: "status present at the begining",
			args: args{
				status: &v2alpha1.DatadogAgentStatus{
					Conditions: []v1.Condition{
						{
							Type: "fooType",
						},
						{
							Type: "barType",
						},
					},
				},
				condition: "fooType",
			},
			expectedStatus: &v2alpha1.DatadogAgentStatus{
				Conditions: []v1.Condition{
					{
						Type: "barType",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			DeleteDatadogAgentStatusCondition(tt.args.status, tt.args.condition)
			assert.True(t, apiutils.IsEqualStruct(tt.args.status, tt.expectedStatus), "status \ndiff = %s", cmp.Diff(tt.args.status, tt.expectedStatus))
		})
	}
}

func TestDSUpdateWhenNil(t *testing.T) {
	var ds *appsv1.DaemonSet
	dsStatus := UpdateDaemonSetStatus(ds, []*v2alpha1.DaemonSetStatus{}, &metav1.Time{Time: time.Now()})
	dsStatus = UpdateDaemonSetStatus(ds, dsStatus, &metav1.Time{Time: time.Now()})
	dsStatus = UpdateDaemonSetStatus(ds, dsStatus, &metav1.Time{Time: time.Now()})
	assert.Equal(t, 1, len(dsStatus))
}
