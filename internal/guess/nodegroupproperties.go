package guess

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
)

// NodeGroupProperties contains the characteristics of a specific EKS node group
// required to create Karpenter resources (EC2NodeClass and NodePool)
type NodeGroupProperties struct {
	Name                      string
	AMIID                     string
	ReleaseVersion            string
	Subnets                   []string
	SecurityGroups            []string
	InstanceTypes             []string
	CapacityType              string
	Labels                    map[string]string
	Taints                    []corev1.Taint
	NodeRoleARN               string
	RemoteAccessSecurityGroup string
	LaunchTemplate            *LaunchTemplateInfo
	ScalingConfig             *ScalingConfig
	DiskSize                  *int32
}

// LaunchTemplateInfo contains the Launch Template information
type LaunchTemplateInfo struct {
	ID      string
	Name    string
	Version string
}

// ScalingConfig contains the scaling configuration of the node group
type ScalingConfig struct {
	MinSize     *int32
	MaxSize     *int32
	DesiredSize *int32
}

// GetNodeGroupsProperties retrieves the properties of all node groups in an EKS cluster
// and returns a list with the characteristics of each node group
func GetNodeGroupsProperties(ctx context.Context, client *eks.Client, clusterName string) ([]NodeGroupProperties, error) {
	nodeGroupsProperties := []NodeGroupProperties{}

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
			properties, err := extractNodeGroupProperties(ctx, client, clusterName, ngName)
			if err != nil {
				return nil, fmt.Errorf("failed to extract properties from node group %s: %w", ngName, err)
			}
			if cluster.Cluster != nil && cluster.Cluster.ResourcesVpcConfig != nil && cluster.Cluster.ResourcesVpcConfig.VpcId != nil {
				properties.SecurityGroups = append(properties.SecurityGroups, *cluster.Cluster.ResourcesVpcConfig.ClusterSecurityGroupId)
			}
			nodeGroupsProperties = append(nodeGroupsProperties, properties)
		}

		nextToken = nodegroupsList.NextToken
		if nextToken == nil {
			return nodeGroupsProperties, nil
		}
	}
}

// extractNodeGroupProperties extracts the properties of a specific node group
func extractNodeGroupProperties(ctx context.Context, client *eks.Client, clusterName, nodeGroupName string) (NodeGroupProperties, error) {
	nodegroup, err := client.DescribeNodegroup(ctx, &eks.DescribeNodegroupInput{
		ClusterName:   &clusterName,
		NodegroupName: &nodeGroupName,
	})
	if err != nil {
		return NodeGroupProperties{}, fmt.Errorf("failed to describe node group %s: %w", nodeGroupName, err)
	}

	ng := nodegroup.Nodegroup
	if ng == nil {
		return NodeGroupProperties{}, fmt.Errorf("node group %s not found", nodeGroupName)
	}

	properties := NodeGroupProperties{
		Name:           *ng.NodegroupName,
		Subnets:        ng.Subnets,
		InstanceTypes:  ng.InstanceTypes,
		Labels:         ng.Labels,
		Taints:         lo.Map(ng.Taints, func(t ekstypes.Taint, _ int) corev1.Taint { return convertTaint(t) }),
		SecurityGroups: []string{},
	}

	for l := range properties.Labels {
		if strings.Contains(l, "eksctl.io") {
			delete(properties.Labels, l)
		}
	}

	if ng.ReleaseVersion != nil {
		properties.ReleaseVersion = *ng.ReleaseVersion
	}

	if ng.CapacityType != "" {
		properties.CapacityType = string(ng.CapacityType)
	}

	if ng.NodeRole != nil {
		properties.NodeRoleARN = *ng.NodeRole
	}

	if ng.DiskSize != nil {
		properties.DiskSize = ng.DiskSize
	}

	if ng.LaunchTemplate != nil {
		properties.LaunchTemplate = &LaunchTemplateInfo{}
		if ng.LaunchTemplate.Id != nil {
			properties.LaunchTemplate.ID = *ng.LaunchTemplate.Id
		}
		if ng.LaunchTemplate.Name != nil {
			properties.LaunchTemplate.Name = *ng.LaunchTemplate.Name
		}
		if ng.LaunchTemplate.Version != nil {
			properties.LaunchTemplate.Version = *ng.LaunchTemplate.Version
		}
		// Note: To get the AMI ID from the Launch Template, an additional
		// EC2 DescribeLaunchTemplateVersions call would be required
	}

	// Extract scaling configuration
	if ng.ScalingConfig != nil {
		properties.ScalingConfig = &ScalingConfig{
			MinSize:     ng.ScalingConfig.MinSize,
			MaxSize:     ng.ScalingConfig.MaxSize,
			DesiredSize: ng.ScalingConfig.DesiredSize,
		}
	}

	// Extract security groups
	if ng.Resources != nil {
		if ng.Resources.RemoteAccessSecurityGroup != nil {
			properties.RemoteAccessSecurityGroup = *ng.Resources.RemoteAccessSecurityGroup
			properties.SecurityGroups = append(properties.SecurityGroups, *ng.Resources.RemoteAccessSecurityGroup)
		}
		// Note: There may be other security groups defined in the Launch Template
	}

	return properties, nil
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
