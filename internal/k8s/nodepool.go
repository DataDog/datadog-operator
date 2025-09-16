package k8s

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

func CreateOrUpdateNodePool(ctx context.Context, client client.Client) error {
	nodePool := &karpv1.NodePool{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "karpenter.sh/v1",
			Kind:       "NodePool",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "dd-karpenter",
		},
		Spec: karpv1.NodePoolSpec{
			Template: karpv1.NodeClaimTemplate{
				Spec: karpv1.NodeClaimTemplateSpec{
					Requirements: []karpv1.NodeSelectorRequirementWithMinValues{},
					NodeClassRef: &karpv1.NodeClassReference{
						Group: "karpenter.k8s.aws",
						Kind:  "EC2NodeClass",
						Name:  "dd-karpenter",
					},
				},
			},
		},
	}

	return createOrUpdate(ctx, client, nodePool)
}
