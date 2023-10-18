package datadogagent

import (
	"testing"

	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_generateNodeAffinity(t *testing.T) {
	defaultProvider := kubernetes.Provider{
		Name:          kubernetes.DefaultProvider,
		ComponentName: kubernetes.DefaultProvider,
	}
	gcpUbuntuProvider := kubernetes.Provider{
		Name:          kubernetes.GCPUbuntuProvider,
		ComponentName: kubernetes.GenerateComponentName(kubernetes.GCPCloudProvider, kubernetes.GCPUbuntuProvider),
		ProviderLabel: kubernetes.GCPProviderLabel,
		CloudProvider: kubernetes.GCPCloudProvider,
	}

	type args struct {
		affinity *corev1.Affinity
		provider kubernetes.Provider
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "nil affinity, default provider",
			args: args{
				affinity: nil,
				provider: defaultProvider,
			},
		},
		{
			name: "nil affinity, gcp ubuntu provider",
			args: args{
				affinity: nil,
				provider: gcpUbuntuProvider,
			},
		},
		{
			name: "existing affinity, but empty, default provider",
			args: args{
				affinity: &corev1.Affinity{},
				provider: defaultProvider,
			},
		},
		{
			name: "existing affinity, but empty, gcp ubuntu provider",
			args: args{
				affinity: &corev1.Affinity{},
				provider: gcpUbuntuProvider,
			},
		},
		{
			name: "existing affinity, NodeAffinity empty, default provider",
			args: args{
				affinity: &corev1.Affinity{
					PodAffinity: &corev1.PodAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"foo": "bar",
									},
								},
								TopologyKey: "foo/bar",
							},
						},
					},
				},
				provider: defaultProvider,
			},
		},
		{
			name: "existing affinity, NodeAffinity empty, gcp ubuntu provider",
			args: args{
				affinity: &corev1.Affinity{
					PodAffinity: &corev1.PodAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"foo": "bar",
									},
								},
								TopologyKey: "foo/bar",
							},
						},
					},
				},
				provider: gcpUbuntuProvider,
			},
		},
		{
			name: "existing affinity, NodeAffinity filled, default provider",
			args: args{
				affinity: &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{
								{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "foo",
											Operator: corev1.NodeSelectorOpDoesNotExist,
										},
									},
								},
							},
						},
					},
				},
				provider: defaultProvider,
			},
		},
		{
			name: "existing affinity, NodeAffinity filled, gcp ubuntu provider",
			args: args{
				affinity: &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{
								{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "foo",
											Operator: corev1.NodeSelectorOpDoesNotExist,
										},
									},
								},
							},
						},
					},
				},
				provider: gcpUbuntuProvider,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := kubernetes.NewProfiles(logf.Log.WithName("test_generateNodeAffinity"))
			r := &Reconciler{
				profiles: &p,
			}
			setUpProviders(r)

			actualAffinity := r.generateNodeAffinity(tt.args.provider, tt.args.affinity)
			na, pa, paa := getAffinityComponents(tt.args.affinity)
			wantedAffinity := generateWantedAffinity(tt.args.provider, na, pa, paa)
			assert.Equal(t, wantedAffinity, actualAffinity)
		})
	}

}

func setUpProviders(r *Reconciler) {
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-gcp-cos",
				Labels: map[string]string{
					kubernetes.GCPProviderLabel: kubernetes.GCPCosProvider,
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-gcp-ubuntu",
				Labels: map[string]string{
					kubernetes.GCPProviderLabel: kubernetes.GCPUbuntuContainerdProvider,
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-gcp-windows",
				Labels: map[string]string{
					kubernetes.GCPProviderLabel: kubernetes.GCPWindowsLTSCContainerdProvider,
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-default",
				Labels: map[string]string{
					"foo": "bar",
				},
			},
		},
	}
	for _, node := range nodes {
		r.profiles.SetProvider(&node)
	}
}

func generateWantedAffinity(provider kubernetes.Provider, na *corev1.NodeAffinity, pa *corev1.PodAffinity, paa *corev1.PodAntiAffinity) *corev1.Affinity {
	defaultNA := corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      kubernetes.GCPProviderLabel,
							Operator: corev1.NodeSelectorOpNotIn,
							Values:   []string{kubernetes.GCPCosProvider},
						},
						{
							Key:      kubernetes.GCPProviderLabel,
							Operator: corev1.NodeSelectorOpNotIn,
							Values:   []string{kubernetes.GCPUbuntuContainerdProvider},
						},
						{
							Key:      kubernetes.GCPProviderLabel,
							Operator: corev1.NodeSelectorOpNotIn,
							Values:   []string{kubernetes.GCPWindowsLTSCContainerdProvider},
						},
					},
				},
			},
		},
	}
	if na != nil {
		defaultNA = *na
	}
	if provider.Name == kubernetes.DefaultProvider {
		return &corev1.Affinity{
			NodeAffinity:    &defaultNA,
			PodAffinity:     pa,
			PodAntiAffinity: paa,
		}
	}

	providerNA := corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      provider.ProviderLabel,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{provider.Name},
						},
					},
				},
			},
		},
	}
	if na != nil {
		providerNA = *na
	}
	return &corev1.Affinity{
		NodeAffinity:    &providerNA,
		PodAffinity:     pa,
		PodAntiAffinity: paa,
	}

}

func getAffinityComponents(affinity *corev1.Affinity) (*corev1.NodeAffinity, *corev1.PodAffinity, *corev1.PodAntiAffinity) {
	if affinity == nil {
		return nil, nil, nil
	}
	return affinity.NodeAffinity, affinity.PodAffinity, affinity.PodAntiAffinity
}
