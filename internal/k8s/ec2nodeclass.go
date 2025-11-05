package k8s

import (
	"context"

	karpawsv1 "github.com/aws/karpenter-provider-aws/pkg/apis/v1"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/L3n41c/karpenter_installer_wizard/internal/guess"
)

func CreateOrUpdateEC2NodeClass(ctx context.Context, client client.Client, clusterName string, nc guess.EC2NodeClass) error {
	return createOrUpdate(ctx, client, &karpawsv1.EC2NodeClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "karpenter.k8s.aws/v1",
			Kind:       "EC2NodeClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: nc.Name,
		},
		Spec: karpawsv1.EC2NodeClassSpec{
			Role: "KarpenterNodeRole-" + clusterName,
			AMISelectorTerms: lo.Map(nc.AMIIDs, func(ami string, _ int) karpawsv1.AMISelectorTerm {
				return karpawsv1.AMISelectorTerm{
					Alias: "al2023@latest", // TODO: FIX
				}
			}),
			SubnetSelectorTerms: lo.Map(nc.SubnetIDs, func(subnetID string, _ int) karpawsv1.SubnetSelectorTerm {
				return karpawsv1.SubnetSelectorTerm{
					ID: subnetID,
				}
			}),
			SecurityGroupSelectorTerms: lo.Map(nc.SecurityGroupIDs, func(sgID string, _ int) karpawsv1.SecurityGroupSelectorTerm {
				return karpawsv1.SecurityGroupSelectorTerm{
					ID: sgID,
				}
			}),
		},
	})
}
