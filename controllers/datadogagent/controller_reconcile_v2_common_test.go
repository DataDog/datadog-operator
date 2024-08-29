package datadogagent

import (
	"testing"

	"github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

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

func Test_shouldCheckSlowStartStatus(t *testing.T) {
	tests := []struct {
		name      string
		profile   *v1alpha1.DatadogAgentProfile
		slowStart string
		expected  bool
	}{
		{
			name:      "nil profile",
			profile:   nil,
			slowStart: "true",
			expected:  false,
		},
		{
			name:      "slow start false",
			profile:   nil,
			slowStart: "false",
			expected:  false,
		},
		{
			name:      "empty profile",
			profile:   &v1alpha1.DatadogAgentProfile{},
			slowStart: "true",
			expected:  false,
		},
		{
			name: "default profile",
			profile: &v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			},
			slowStart: "true",
			expected:  false,
		},
		{
			name: "empty profile status",
			profile: &v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: v1alpha1.DatadogAgentProfileStatus{},
			},
			slowStart: "true",
			expected:  false,
		},
		{
			name: "completed slow start status",
			profile: &v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: v1alpha1.DatadogAgentProfileStatus{
					SlowStart: &v1alpha1.SlowStart{
						Status: v1alpha1.CompletedStatus,
					},
				},
			},
			slowStart: "true",
			expected:  false,
		},
		{
			name: "in progress slow start status",
			profile: &v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: v1alpha1.DatadogAgentProfileStatus{
					SlowStart: &v1alpha1.SlowStart{
						Status: v1alpha1.InProgressStatus,
					},
				},
			},
			slowStart: "true",
			expected:  true,
		},
		{
			name: "waiting slow start status",
			profile: &v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: v1alpha1.DatadogAgentProfileStatus{
					SlowStart: &v1alpha1.SlowStart{
						Status: v1alpha1.WaitingStatus,
					},
				},
			},
			slowStart: "true",
			expected:  true,
		},
		{
			name: "empty status in slow start status",
			profile: &v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: v1alpha1.DatadogAgentProfileStatus{
					SlowStart: &v1alpha1.SlowStart{
						Status: "",
					},
				},
			},
			slowStart: "true",
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(common.SlowStartEnabled, tt.slowStart)
			actual := shouldCheckSlowStartStatus(tt.profile)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
