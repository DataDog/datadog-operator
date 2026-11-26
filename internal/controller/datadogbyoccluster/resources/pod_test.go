package resources

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildResources_Affinity(t *testing.T) {
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
							LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
								"app.kubernetes.io/name":      "cloudprem",
								"app.kubernetes.io/instance":  "byoc",
								"app.kubernetes.io/component": "indexer",
							}},
							TopologyKey: corev1.LabelHostname,
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
			cluster := testCluster()
			cluster.Spec.Global.Affinity = tt.global
			cluster.Spec.Components.Indexer.Affinity = tt.component

			resources, err := buildResources(cluster, testImages())
			if err != nil {
				t.Fatalf("BuildResources() unexpected error: %v", err)
			}
			got := resources.Component(IndexerComponentName).StatefulSet.Spec.Template.Spec.Affinity
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("affinity mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildResources_Args(t *testing.T) {
	tests := []struct {
		name         string
		indexerArgs  []string
		wantIndexer  []string
		wantSearcher []string
	}{
		{
			name:         "default",
			wantIndexer:  []string{"run", "--service", "indexer"},
			wantSearcher: []string{"run", "--service", "searcher"},
		},
		{
			name:         "empty args falls back to default",
			indexerArgs:  []string{},
			wantIndexer:  []string{"run", "--service", "indexer"},
			wantSearcher: []string{"run", "--service", "searcher"},
		},
		{
			name:         "indexer args",
			indexerArgs:  []string{"run", "--foo"},
			wantIndexer:  []string{"run", "--foo"},
			wantSearcher: []string{"run", "--service", "searcher"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := testCluster()
			cluster.Spec.Components.Indexer.Args = tt.indexerArgs

			resources, err := buildResources(cluster, testImages())
			if err != nil {
				t.Fatalf("BuildResources() unexpected error: %v", err)
			}
			got := resources.Component(IndexerComponentName).StatefulSet.Spec.Template.Spec.Containers[0].Args
			if diff := cmp.Diff(tt.wantIndexer, got); diff != "" {
				t.Errorf("indexer args mismatch (-want +got):\n%s", diff)
			}
			got = resources.Component(SearcherComponentName).StatefulSet.Spec.Template.Spec.Containers[0].Args
			if diff := cmp.Diff(tt.wantSearcher, got); diff != "" {
				t.Errorf("searcher args mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
