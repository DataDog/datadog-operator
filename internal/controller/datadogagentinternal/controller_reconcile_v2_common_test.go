package datadogagentinternal

import (
	"testing"
	"time"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"

	assert "github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_ensureSelectorInPodTemplateLabels(t *testing.T) {
	logger := logf.Log.WithName("Test_ensureSelectorInPodTemplateLabels")

	tests := []struct {
		name              string
		selector          *metav1.LabelSelector
		podTemplateLabels map[string]string
		expectedLabels    map[string]string
	}{
		{
			name:     "Nil selector",
			selector: nil,
			podTemplateLabels: map[string]string{
				"foo": "bar",
			},
			expectedLabels: map[string]string{
				"foo": "bar",
			},
		},
		{
			name:     "Empty selector",
			selector: &metav1.LabelSelector{},
			podTemplateLabels: map[string]string{
				"foo": "bar",
			},
			expectedLabels: map[string]string{
				"foo": "bar",
			},
		},
		{
			name: "Selector in template labels",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			},
			podTemplateLabels: map[string]string{
				"foo": "bar",
			},
			expectedLabels: map[string]string{
				"foo": "bar",
			},
		},
		{
			name: "Selector not in template labels",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"bar": "foo",
				},
			},
			podTemplateLabels: map[string]string{
				"foo": "bar",
			},
			expectedLabels: map[string]string{
				"foo": "bar",
				"bar": "foo",
			},
		},
		{
			name: "Selector label value does not match template labels",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "foo",
				},
			},
			podTemplateLabels: map[string]string{
				"foo": "bar",
			},
			expectedLabels: map[string]string{
				"foo": "foo",
			},
		},
		{
			name: "Nil pod template labels",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "foo",
				},
			},
			podTemplateLabels: nil,
			expectedLabels: map[string]string{
				"foo": "foo",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels := ensureSelectorInPodTemplateLabels(logger, tt.selector, tt.podTemplateLabels)
			assert.Equal(t, tt.expectedLabels, labels)
		})
	}
}

func Test_shouldCheckCreateStrategyStatus(t *testing.T) {
	tests := []struct {
		name           string
		profile        *v1alpha1.DatadogAgentProfile
		CreateStrategy string
		expected       bool
	}{
		{
			name:           "nil profile",
			profile:        nil,
			CreateStrategy: "true",
			expected:       false,
		},
		{
			name:           "create strategy false",
			profile:        nil,
			CreateStrategy: "false",
			expected:       false,
		},
		{
			name:           "empty profile",
			profile:        &v1alpha1.DatadogAgentProfile{},
			CreateStrategy: "true",
			expected:       false,
		},
		{
			name: "default profile",
			profile: &v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			},
			CreateStrategy: "true",
			expected:       false,
		},
		{
			name: "empty profile status",
			profile: &v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: v1alpha1.DatadogAgentProfileStatus{},
			},
			CreateStrategy: "true",
			expected:       false,
		},
		{
			name: "completed create strategy status",
			profile: &v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: v1alpha1.DatadogAgentProfileStatus{
					CreateStrategy: &v1alpha1.CreateStrategy{
						Status: v1alpha1.CompletedStatus,
					},
				},
			},
			CreateStrategy: "true",
			expected:       false,
		},
		{
			name: "in progress create strategy status",
			profile: &v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: v1alpha1.DatadogAgentProfileStatus{
					CreateStrategy: &v1alpha1.CreateStrategy{
						Status: v1alpha1.InProgressStatus,
					},
				},
			},
			CreateStrategy: "true",
			expected:       true,
		},
		{
			name: "waiting create strategy status",
			profile: &v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: v1alpha1.DatadogAgentProfileStatus{
					CreateStrategy: &v1alpha1.CreateStrategy{
						Status: v1alpha1.WaitingStatus,
					},
				},
			},
			CreateStrategy: "true",
			expected:       true,
		},
		{
			name: "empty status in create strategy status",
			profile: &v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: v1alpha1.DatadogAgentProfileStatus{
					CreateStrategy: &v1alpha1.CreateStrategy{
						Status: "",
					},
				},
			},
			CreateStrategy: "true",
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(apicommon.CreateStrategyEnabled, tt.CreateStrategy)
			actual := shouldCheckCreateStrategyStatus(tt.profile)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func Test_getDDALastUpdatedTime(t *testing.T) {
	now := metav1.Now()
	now5MinLater := metav1.NewTime(now.Add(5 * time.Minute))
	now15MinLater := metav1.NewTime(now.Add(15 * time.Minute))
	tests := []struct {
		name              string
		managedFields     []metav1.ManagedFieldsEntry
		creationTimestamp metav1.Time
		expected          metav1.Time
	}{
		{
			name:              "no managed field entries",
			managedFields:     []metav1.ManagedFieldsEntry{},
			creationTimestamp: now,
			expected:          now,
		},
		{
			name: "one new entry",
			managedFields: []metav1.ManagedFieldsEntry{
				{
					Time: &now15MinLater,
				},
			},
			creationTimestamp: now,
			expected:          now15MinLater,
		},
		{
			name: "multiple entries",
			managedFields: []metav1.ManagedFieldsEntry{
				{
					Time: &now15MinLater,
				},
				{
					Time: &now5MinLater,
				},
				{
					Time: &now,
				},
			},
			creationTimestamp: now,
			expected:          now15MinLater,
		},
		{
			name: "ignore status entry",
			managedFields: []metav1.ManagedFieldsEntry{
				{
					Subresource: "status",
					Time:        &now15MinLater,
				},
				{
					Time: &now5MinLater,
				},
				{
					Time: &now,
				},
			},
			creationTimestamp: now,
			expected:          now5MinLater,
		},
		{
			name: "nil time entry",
			managedFields: []metav1.ManagedFieldsEntry{
				{
					Time: &now15MinLater,
				},
				{
					Manager: "test",
				},
				{
					Time: &now,
				},
			},
			creationTimestamp: now,
			expected:          now15MinLater,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := getDDALastUpdatedTime(tt.managedFields, tt.creationTimestamp)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func Test_shouldProfileWaitForCanary(t *testing.T) {
	logger := logf.Log.WithName("Test_shouldProfileWaitForCanary")

	tests := []struct {
		name        string
		annotations map[string]string
		expected    bool
	}{
		{
			name:        "Nil annotations",
			annotations: nil,
			expected:    false,
		},
		{
			name:        "Empty annotations",
			annotations: map[string]string{},
			expected:    false,
		},
		{
			name: "No relevant annotations",
			annotations: map[string]string{
				"foo": "bar",
			},
			expected: false,
		},
		{
			name: "Relevant annotation exists",
			annotations: map[string]string{
				"foo":                   "bar",
				profileWaitForCanaryKey: "true",
			},
			expected: true,
		},
		{
			name: "Relevant annotation exists and is false",
			annotations: map[string]string{
				"foo":                   "bar",
				profileWaitForCanaryKey: "false",
			},
			expected: false,
		},
		{
			name: "Relevant annotation exists, but value is not bool",
			annotations: map[string]string{
				"foo":                   "bar",
				profileWaitForCanaryKey: "yes",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldWait := shouldProfileWaitForCanary(logger, tt.annotations)
			assert.Equal(t, tt.expected, shouldWait)
		})
	}
}
