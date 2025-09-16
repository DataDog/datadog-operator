package k8s

import (
	"context"

	karpawsv1 "github.com/aws/karpenter-provider-aws/pkg/apis/v1"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateOrUpdateEC2NodeClass(ctx context.Context, client client.Client, clusterName string, AMIs, subnets, sgs []string) error {
	ec2NodeClass := &karpawsv1.EC2NodeClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "karpenter.k8s.aws/v1",
			Kind:       "EC2NodeClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "dd-karpenter",
		},
		Spec: karpawsv1.EC2NodeClassSpec{
			Role:      "KarpenterNodeRole-" + clusterName,
			AMIFamily: &karpawsv1.AMIFamilyAL2023, // TODO: FIX
			AMISelectorTerms: lo.Map(AMIs, func(ami string, _ int) karpawsv1.AMISelectorTerm {
				return karpawsv1.AMISelectorTerm{
					ID: ami,
				}
			}),
			SubnetSelectorTerms: lo.Map(subnets, func(subnet string, _ int) karpawsv1.SubnetSelectorTerm {
				return karpawsv1.SubnetSelectorTerm{
					ID: subnet,
				}
			}),
			SecurityGroupSelectorTerms: lo.Map(sgs, func(sg string, _ int) karpawsv1.SecurityGroupSelectorTerm {
				return karpawsv1.SecurityGroupSelectorTerm{
					ID: sg,
				}
			}),
		},
	}

	return createOrUpdate(ctx, client, ec2NodeClass)
}
