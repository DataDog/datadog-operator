package guess

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
)

func GetNodeGroupsProperties(ctx context.Context, client *eks.Client, clusterName string) (*NodePoolsSet, error) {
	nps := NewNodePoolsSet()

	cluster, err := client.DescribeCluster(ctx, &eks.DescribeClusterInput{
		Name: &clusterName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe cluster: %w", err)
	}

	var nextToken *string
	for {
		nodegroupsList, err := client.ListNodegroups(ctx, &eks.ListNodegroupsInput{
			ClusterName: &clusterName,
			NextToken:   nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list node groups: %w", err)
		}

		for _, ngName := range nodegroupsList.Nodegroups {
			nodegroup, err := client.DescribeNodegroup(ctx, &eks.DescribeNodegroupInput{
				ClusterName:   &clusterName,
				NodegroupName: &ngName,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to describe node group %s: %w", ngName, err)
			}

			ng := nodegroup.Nodegroup
			if ng == nil {
				return nil, fmt.Errorf("node group %s not found", ngName)
			}

			params := NodePoolsSetAddParams{
				AMIID:     string(ng.AmiType),
				SubnetIDs: ng.Subnets,
				Labels:    ng.Labels,
				Taints:    lo.Map(ng.Taints, func(t ekstypes.Taint, _ int) corev1.Taint { return convertTaint(t) }),
			}

			if cluster.Cluster != nil && cluster.Cluster.ResourcesVpcConfig != nil && cluster.Cluster.ResourcesVpcConfig.VpcId != nil {
				params.SecurityGroupIDs = []string{*cluster.Cluster.ResourcesVpcConfig.ClusterSecurityGroupId}
			}

			nps.Add(params)
		}

		nextToken = nodegroupsList.NextToken
		if nextToken == nil {
			return nps, nil
		}
	}
}

func convertTaint(in ekstypes.Taint) (out corev1.Taint) {
	switch in.Effect {
	case ekstypes.TaintEffectNoExecute:
		out.Effect = corev1.TaintEffectNoExecute
	case ekstypes.TaintEffectNoSchedule:
		out.Effect = corev1.TaintEffectNoSchedule
	case ekstypes.TaintEffectPreferNoSchedule:
		out.Effect = corev1.TaintEffectPreferNoSchedule
	}

	if in.Key != nil {
		out.Key = *in.Key
	}

	if in.Value != nil {
		out.Value = *in.Value
	}

	return
}
