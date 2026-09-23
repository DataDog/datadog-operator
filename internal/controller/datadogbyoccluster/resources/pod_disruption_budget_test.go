package resources

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func TestPodDisruptionBudgetBuilder_build(t *testing.T) {
	minAvailable := intstr.FromInt32(2)
	maxUnavailable := intstr.FromString("25%")
	workload := workloadValues{
		Metadata: metav1.ObjectMeta{Name: "byoc-indexer", Namespace: "testing"},
		Selector: map[string]string{"app.kubernetes.io/component": "indexer"},
	}
	podDisruptionBudget := func(minAvailable, maxUnavailable *intstr.IntOrString) *policyv1.PodDisruptionBudget {
		return &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{Name: "byoc-indexer", Namespace: "testing"},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MinAvailable:   minAvailable,
				MaxUnavailable: maxUnavailable,
				Selector:       &metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/component": "indexer"}},
			},
		}
	}
	tests := []struct {
		name      string
		global    *datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec
		component *datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec
		want      *policyv1.PodDisruptionBudget
		wantErr   bool
	}{
		{
			name: "operator default",
			want: podDisruptionBudget(nil, ptr.To(intstr.FromInt32(1))),
		},
		{
			name:   "global override",
			global: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{MinAvailable: &minAvailable},
			want:   podDisruptionBudget(&minAvailable, nil),
		},
		{
			name:      "component override takes precedence",
			global:    &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{MinAvailable: &minAvailable},
			component: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{MaxUnavailable: &maxUnavailable},
			want:      podDisruptionBudget(nil, &maxUnavailable),
		},
		{
			name:   "empty global setting disables budget",
			global: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{},
		},
		{
			name:      "empty component setting disables budget",
			global:    &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{MinAvailable: &minAvailable},
			component: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{},
		},
		{
			name: "minAvailable and maxUnavailable conflict",
			component: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
				MinAvailable:   &minAvailable,
				MaxUnavailable: &maxUnavailable,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newPodDisruptionBudgetBuilder(workload.Metadata, workload.Selector, tt.global, tt.component).build()
			if gotErr := err != nil; gotErr != tt.wantErr {
				t.Fatalf("build() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("build() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
