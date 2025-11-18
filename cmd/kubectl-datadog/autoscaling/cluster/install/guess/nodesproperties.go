package guess

import (
	"context"
	"fmt"
	"log"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const nodeListChunkSize = 100

// awsProviderIDRegexp matches the AWS provider ID format for EC2 instances.
// Format: aws:///ZONE/INSTANCE_ID (e.g., aws:///us-east-1a/i-0abc123def456789)
var awsProviderIDRegexp = regexp.MustCompile(`^aws:///[^/]+/(i-[0-9a-f]+)$`)

func GetNodesProperties(ctx context.Context, clientset *kubernetes.Clientset, ec2Client *ec2.Client) (*NodePoolsSet, error) {
	nps := NewNodePoolsSet()

	var cont string
	for {
		nodesList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{
			Limit:    nodeListChunkSize,
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

		if len(instanceToNode) != 0 {
			instances, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
				InstanceIds: slices.Collect(maps.Keys(instanceToNode)),
			})
			if err != nil {
				return nil, fmt.Errorf("failed to describe instances: %w", err)
			}

			imageIds := lo.Uniq(lo.FlatMap(instances.Reservations, func(reservation ec2types.Reservation, _ int) []string {
				return lo.Map(reservation.Instances, func(instance ec2types.Instance, _ int) string {
					return *instance.ImageId
				})
			}))

			images, err := ec2Client.DescribeImages(ctx, &ec2.DescribeImagesInput{
				ImageIds: imageIds,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to describe images: %w", err)
			}
			amiIDsToFamily := lo.Associate(images.Images, func(image ec2types.Image) (string, string) {
				return *image.ImageId, detectAMIFamilyFromImage(*image.Name)
			})

			for _, reservation := range instances.Reservations {
				for _, instance := range reservation.Instances {
					node := instanceToNode[*instance.InstanceId]

					amiFamily := "Custom"
					if family, ok := amiIDsToFamily[*instance.ImageId]; ok {
						amiFamily = family
					}

					nps.Add(NodePoolsSetAddParams{
						AMIFamily:        amiFamily,
						AMIID:            *instance.ImageId,
						SubnetIDs:        []string{*instance.SubnetId},
						SecurityGroupIDs: lo.Map(instance.SecurityGroups, func(sg ec2types.GroupIdentifier, _ int) string { return *sg.GroupId }),
						Labels:           node.Labels,
						Taints:           node.Spec.Taints,
						Architecture:     convertArchitecture(instance.Architecture),
						Zones:            extractZones(instance.Placement),
						InstanceTypes:    []string{string(instance.InstanceType)},
						CapacityType:     convertInstanceLifecycleType(instance.InstanceLifecycle),
					})
				}
			}
		}

		cont = nodesList.Continue
		if cont == "" {
			return nps, nil
		}
	}
}

func detectAMIFamilyFromImage(imageName string) string {
	containsAny := func(s string, patterns ...string) bool {
		return lo.SomeBy(patterns, func(pattern string) bool {
			return strings.Contains(s, pattern)
		})
	}

	lowerName := strings.ToLower(imageName)

	switch {
	case containsAny(imageName, "amazon-linux-2023", "al2023"):
		return "AL2023"
	case containsAny(imageName, "amazon-linux-2-", "amzn2-ami"):
		return "AL2"
	case strings.Contains(lowerName, "bottlerocket"):
		return "Bottlerocket"
	case strings.Contains(imageName, "Windows_Server-2022"):
		return "Windows2022"
	case strings.Contains(imageName, "Windows_Server-2019"):
		return "Windows2019"
	default:
		return "Custom"
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

func extractZones(placement *ec2types.Placement) []string {
	if placement != nil && placement.AvailabilityZone != nil {
		return []string{*placement.AvailabilityZone}
	}
	return []string{}
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
