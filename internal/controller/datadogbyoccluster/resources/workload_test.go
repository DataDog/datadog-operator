package resources

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResolveAffinity(t *testing.T) {
	cluster := testCluster()
	customAffinity := &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{{
				Weight:     50,
				Preference: corev1.NodeSelectorTerm{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "example.com/node", Operator: corev1.NodeSelectorOpExists}}},
			}},
		},
	}
	tests := []struct {
		name      string
		global    *corev1.Affinity
		component *corev1.Affinity
		want      *corev1.Affinity
	}{
		{
			name: "default pod anti-affinity",
			want: &corev1.Affinity{
				PodAntiAffinity: &corev1.PodAntiAffinity{
					PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{{
						Weight: 100,
						PodAffinityTerm: corev1.PodAffinityTerm{
							LabelSelector: &metav1.LabelSelector{MatchLabels: selectorLabels(cluster, "indexer")},
							TopologyKey:   corev1.LabelHostname,
						},
					}},
				},
			},
		},
		{
			name:   "explicit empty global affinity disables default",
			global: &corev1.Affinity{},
			want:   &corev1.Affinity{},
		},
		{
			name:      "explicit empty component affinity disables default",
			component: &corev1.Affinity{},
			want:      &corev1.Affinity{},
		},
		{
			name:      "custom affinity replaces default",
			component: customAffinity,
			want:      customAffinity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveAffinity(cluster, "indexer", tt.global, tt.component)
			if err != nil {
				t.Fatalf("resolveAffinity() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("resolveAffinity() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
