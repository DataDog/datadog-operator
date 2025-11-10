package k8s

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/karpenter/install/guess"
)

func CreateOrUpdateNodePool(ctx context.Context, client client.Client, np guess.NodePool) error {
	requirements := []karpv1.NodeSelectorRequirementWithMinValues{}

	if len(np.CapacityTypes) > 0 {
		requirements = append(requirements, karpv1.NodeSelectorRequirementWithMinValues{
			NodeSelectorRequirement: corev1.NodeSelectorRequirement{
				Key:      "karpenter.sh/capacity-type",
				Operator: corev1.NodeSelectorOpIn,
				Values:   np.CapacityTypes,
			},
		})
	}

	return createOrUpdate(ctx, client, &karpv1.NodePool{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "karpenter.sh/v1",
			Kind:       "NodePool",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: np.Name,
		},
		Spec: karpv1.NodePoolSpec{
			Template: karpv1.NodeClaimTemplate{
				ObjectMeta: karpv1.ObjectMeta{
					Labels: np.Labels,
				},
				Spec: karpv1.NodeClaimTemplateSpec{
					NodeClassRef: &karpv1.NodeClassReference{
						Group: "karpenter.k8s.aws",
						Kind:  "EC2NodeClass",
						Name:  np.EC2NodeClass,
					},
					Requirements: requirements,
					Taints:       np.Taints,
				},
			},
		},
	})
}
