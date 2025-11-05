package guess

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
)

func GetNodeGroupsProperties(ctx context.Context, eksClient *eks.Client, ec2Client *ec2.Client, clusterName string) (*NodePoolsSet, error) {
	nps := NewNodePoolsSet()

	cluster, err := eksClient.DescribeCluster(ctx, &eks.DescribeClusterInput{
		Name: &clusterName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe cluster: %w", err)
	}

	var nextToken *string
	for {
		nodegroupsList, err := eksClient.ListNodegroups(ctx, &eks.ListNodegroupsInput{
			ClusterName: &clusterName,
			NextToken:   nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list node groups: %w", err)
		}

		for _, ngName := range nodegroupsList.Nodegroups {
			nodegroup, err := eksClient.DescribeNodegroup(ctx, &eks.DescribeNodegroupInput{
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
				SubnetIDs: ng.Subnets,
				Labels:    ng.Labels,
				Taints:    lo.Map(ng.Taints, func(t ekstypes.Taint, _ int) corev1.Taint { return convertTaint(t) }),
			}

			if ng.LaunchTemplate != nil && ng.LaunchTemplate.Id != nil && ng.LaunchTemplate.Version != nil {
				launchTemplateName := *ng.LaunchTemplate.Id
				if ltName := ng.LaunchTemplate.Name; ltName != nil {
					launchTemplateName = *ltName
				}

				launchTemplate, err := ec2Client.DescribeLaunchTemplateVersions(ctx, &ec2.DescribeLaunchTemplateVersionsInput{
					LaunchTemplateId: ng.LaunchTemplate.Id,
					Versions:         []string{*ng.LaunchTemplate.Version},
				})
				if err != nil {
					return nil, fmt.Errorf("failed to describe launch template %s version %s: %w", launchTemplateName, *ng.LaunchTemplate.Version, err)
				}

				if len(launchTemplate.LaunchTemplateVersions) != 1 {
					return nil, fmt.Errorf("couldn’t get launch template %s version %s description", launchTemplateName, *ng.LaunchTemplate.Version)
				}

				if imageId := launchTemplate.LaunchTemplateVersions[0].LaunchTemplateData.ImageId; imageId != nil {
					params.AMIID = *imageId
				}
				params.SecurityGroupIDs = launchTemplate.LaunchTemplateVersions[0].LaunchTemplateData.SecurityGroupIds
			}

			if len(params.SecurityGroupIDs) == 0 && cluster.Cluster != nil && cluster.Cluster.ResourcesVpcConfig != nil && cluster.Cluster.ResourcesVpcConfig.VpcId != nil {
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
