package guess

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"regexp"
	"slices"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const NodeListChunkSize = 100

var awsProviderIDRegexp = regexp.MustCompile(`^aws:///[^/]+/(i-[0-9a-f]+)$`)

func GetNodesProperties(ctx context.Context, clientset *kubernetes.Clientset, ec2Client *ec2.Client) (*NodePoolsSet, error) {
	nps := NewNodePoolsSet()

	var cont string
	for {
		nodesList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{
			Limit:    NodeListChunkSize,
			Continue: cont,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list nodes: %w", err)
		}

		instanceToNode := lo.FilterSliceToMap(nodesList.Items, func(node corev1.Node) (string, corev1.Node, bool) {
			// Filter out Karpenter-managed nodes
			if _, isKarpenter := node.Labels["karpenter.k8s.aws/ec2nodeclass"]; isKarpenter {
				return "", node, false
			}

			matches := awsProviderIDRegexp.FindStringSubmatch(node.Spec.ProviderID)
			if len(matches) != 2 {
				log.Printf("Skipping node %s with unexpected provider ID: %s", node.Name, node.Spec.ProviderID)
				return "", node, false
			}
			return matches[1], node, true
		})

		if len(instanceToNode) == 0 {
			return nil, errors.New("No node not managed by Karpenter found in the cluster")
		}

		instances, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			InstanceIds: slices.Collect(maps.Keys(instanceToNode)),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to describe instances: %w", err)
		}

		for _, reservation := range instances.Reservations {
			for _, instance := range reservation.Instances {
				node := instanceToNode[*instance.InstanceId]

				nps.Add(NodePoolsSetAddParams{
					AMIID:            *instance.ImageId,
					SecurityGroupIDs: lo.Map(instance.SecurityGroups, func(sg ec2types.GroupIdentifier, _ int) string { return *sg.GroupId }),
					SubnetIDs:        []string{*instance.SubnetId},
					Labels:           node.Labels,
					Taints:           node.Spec.Taints,
					CapacityType:     convertInstanceLifecycleType(instance.InstanceLifecycle),
					Architecture:     convertArchitecture(instance.Architecture),
				})
			}
		}

		cont = nodesList.Continue
		if cont == "" {
			return nps, nil
		}
	}
}

func convertInstanceLifecycleType(ilt ec2types.InstanceLifecycleType) string {
	switch ilt {
	case ec2types.InstanceLifecycleTypeScheduled:
		return "on-demand"
	case ec2types.InstanceLifecycleTypeSpot:
		return "spot"
	case ec2types.InstanceLifecycleTypeCapacityBlock:
		return "reserved"
	default:
		return "on-demand"
	}
}

func convertArchitecture(arch ec2types.ArchitectureValues) string {
	switch arch {
	case ec2types.ArchitectureValuesX8664:
		return "amd64"
	case ec2types.ArchitectureValuesArm64:
		return "arm64"
	case ec2types.ArchitectureValuesI386:
		return "386"
	default:
		return ""
	}
}
