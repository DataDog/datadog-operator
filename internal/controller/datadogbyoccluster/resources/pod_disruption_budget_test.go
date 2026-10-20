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

func TestBuildResources_PodDisruptionBudget(t *testing.T) {
	minAvailable := intstr.FromInt32(2)
	maxUnavailable := intstr.FromString("25%")
	podDisruptionBudget := func(minAvailable, maxUnavailable *intstr.IntOrString) *policyv1.PodDisruptionBudget {
		return &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "byoc-indexer",
				Namespace: "testing",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "cloudprem",
					"app.kubernetes.io/instance":   "byoc",
					"app.kubernetes.io/component":  "indexer",
					"app.kubernetes.io/managed-by": "datadog-operator",
					"team":                         "search",
				},
				Annotations: map[string]string{"example.com/owner": "operator"},
			},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MinAvailable:   minAvailable,
				MaxUnavailable: maxUnavailable,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
					"app.kubernetes.io/name":      "cloudprem",
					"app.kubernetes.io/instance":  "byoc",
					"app.kubernetes.io/component": "indexer",
				}},
			},
		}
	}
	tests := []struct {
		name      string
		global    *datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec
		component *datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec
		want      *policyv1.PodDisruptionBudget
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := testCluster()
			cluster.Spec.Global.PodDisruptionBudget = tt.global
			cluster.Spec.Components.Indexer.PodDisruptionBudget = tt.component

			resources, err := buildResources(cluster, testRelease())
			if err != nil {
				t.Fatalf("BuildResources() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, resources.Component(IndexerComponentName).PodDisruptionBudget); diff != "" {
				t.Errorf("PodDisruptionBudget mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildResources_PodDisruptionBudgetConflict(t *testing.T) {
	cluster := testCluster()
	cluster.Spec.Components.Indexer.PodDisruptionBudget = &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
		MinAvailable:   ptr.To(intstr.FromInt32(2)),
		MaxUnavailable: ptr.To(intstr.FromString("25%")),
	}

	if _, err := buildResources(cluster, testRelease()); err == nil {
		t.Fatal("BuildResources() expected an error for conflicting minAvailable and maxUnavailable")
	}
}
