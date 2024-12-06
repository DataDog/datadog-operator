package v2alpha1

import (
	"testing"

	apiutils "github.com/DataDog/datadog-operator/api/crds/utils"
	"github.com/google/go-cmp/cmp"
	assert "github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDeleteDatadogAgentStatusCondition(t *testing.T) {
	type args struct {
		status    *DatadogAgentStatus
		condition string
	}
	tests := []struct {
		name           string
		args           args
		expectedStatus *DatadogAgentStatus
	}{
		{
			name: "empty status",
			args: args{
				status:    &DatadogAgentStatus{},
				condition: "fooType",
			},
			expectedStatus: &DatadogAgentStatus{},
		},
		{
			name: "not present status",
			args: args{
				status: &DatadogAgentStatus{
					Conditions: []v1.Condition{
						{
							Type: "barType",
						},
					},
				},
				condition: "fooType",
			},
			expectedStatus: &DatadogAgentStatus{
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
				status: &DatadogAgentStatus{
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
			expectedStatus: &DatadogAgentStatus{
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
				status: &DatadogAgentStatus{
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
			expectedStatus: &DatadogAgentStatus{
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
