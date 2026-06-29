package datadogagentinternal

import (
	"context"
	"testing"

	assert "github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_ensureSelectorInPodTemplateLabels(t *testing.T) {

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
			labels := ensureSelectorInPodTemplateLabels(context.Background(), tt.selector, tt.podTemplateLabels)
			assert.Equal(t, tt.expectedLabels, labels)
		})
	}
}
