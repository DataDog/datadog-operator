package k8s

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	commonk8s "github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/k8s"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/install/guess"
	"github.com/DataDog/datadog-operator/pkg/version"
)

func CreateOrUpdateNodePool(ctx context.Context, client client.Client, np guess.NodePool) error {
	requirements := []karpv1.NodeSelectorRequirementWithMinValues{}

	if architectures := np.GetArchitectures(); len(architectures) > 0 {
		requirements = append(requirements, karpv1.NodeSelectorRequirementWithMinValues{
			NodeSelectorRequirement: corev1.NodeSelectorRequirement{
				Key:      "kubernetes.io/arch",
				Operator: corev1.NodeSelectorOpIn,
				Values:   architectures,
			},
		})
	}

	if zones := np.GetZones(); len(zones) > 0 {
		requirements = append(requirements, karpv1.NodeSelectorRequirementWithMinValues{
			NodeSelectorRequirement: corev1.NodeSelectorRequirement{
				Key:      "topology.kubernetes.io/zone",
				Operator: corev1.NodeSelectorOpIn,
				Values:   zones,
			},
		})
	}

	if instanceFamilies := np.GetInstanceFamilies(); len(instanceFamilies) > 0 {
		requirements = append(requirements, karpv1.NodeSelectorRequirementWithMinValues{
			NodeSelectorRequirement: corev1.NodeSelectorRequirement{
				Key:      "karpenter.k8s.aws/instance-family",
				Operator: corev1.NodeSelectorOpIn,
				Values:   instanceFamilies,
			},
		})
	}

	if capacityTypes := np.GetCapacityTypes(); len(capacityTypes) > 0 {
		requirements = append(requirements, karpv1.NodeSelectorRequirementWithMinValues{
			NodeSelectorRequirement: corev1.NodeSelectorRequirement{
				Key:      "karpenter.sh/capacity-type",
				Operator: corev1.NodeSelectorOpIn,
				Values:   capacityTypes,
			},
		})
	}

	return commonk8s.CreateOrUpdate(ctx, client, &karpv1.NodePool{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "karpenter.sh/v1",
			Kind:       "NodePool",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: np.GetName(),
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":      "kubectl-datadog",
				"app.kubernetes.io/version":         version.GetVersion(),
				"autoscaling.datadoghq.com/created": "true",
			},
		},
		Spec: karpv1.NodePoolSpec{
			Template: karpv1.NodeClaimTemplate{
				ObjectMeta: karpv1.ObjectMeta{
					Labels: np.GetLabels(),
				},
				Spec: karpv1.NodeClaimTemplateSpec{
					NodeClassRef: &karpv1.NodeClassReference{
						Group: "karpenter.k8s.aws",
						Kind:  "EC2NodeClass",
						Name:  np.GetEC2NodeClass(),
					},
					Requirements: requirements,
					Taints:       np.GetTaints(),
				},
			},
			Disruption: karpv1.Disruption{
				Budgets: []karpv1.Budget{
					{
						Nodes: "10%",
					},
				},
				ConsolidateAfter:    karpv1.MustParseNillableDuration("5m"),
				ConsolidationPolicy: karpv1.ConsolidationPolicyWhenEmptyOrUnderutilized,
			},
		},
	})
}
