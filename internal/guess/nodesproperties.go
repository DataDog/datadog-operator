package guess

import (
	"context"
	"fmt"
	"log"
	"regexp"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const NodeListChunkSize = 100

type NodesProperties struct {
	Archs          map[string]struct{}
	AMIs           map[string]struct{}
	Subnets        map[string]struct{}
	SecurityGroups map[string]struct{}
}

var awsProviderIDRegexp = regexp.MustCompile(`^aws:///[^/]+/(i-[0-9a-f]+)$`)

func GetNodesProperties(ctx context.Context, clientset *kubernetes.Clientset, client *ec2.Client) (*NodesProperties, error) {
	nodesProperties := &NodesProperties{
		Archs:          map[string]struct{}{},
		AMIs:           map[string]struct{}{},
		Subnets:        map[string]struct{}{},
		SecurityGroups: map[string]struct{}{},
	}

	var cont string
	for {
		nodesList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{
			Limit:    NodeListChunkSize,
			Continue: cont,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list nodes: %w", err)
		}

		instances, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			InstanceIds: lo.Map(nodesList.Items, func(node corev1.Node, _ int) string {
				matches := awsProviderIDRegexp.FindStringSubmatch(node.Spec.ProviderID)
				if len(matches) != 2 {
					log.Printf("Skipping node %s with unexpected provider ID: %s", node.Name, node.Spec.ProviderID)
					return ""
				}
				return matches[1]
			}),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to describe instances: %w", err)
		}

		for _, reservation := range instances.Reservations {
			for _, instance := range reservation.Instances {
				nodesProperties.Archs[string(instance.Architecture)] = struct{}{}
				if instance.ImageId != nil {
					nodesProperties.AMIs[*instance.ImageId] = struct{}{}
				}
				if instance.SubnetId != nil {
					nodesProperties.Subnets[*instance.SubnetId] = struct{}{}
				}
				for _, sg := range instance.SecurityGroups {
					if sg.GroupId != nil {
						nodesProperties.SecurityGroups[*sg.GroupId] = struct{}{}
					}
				}
			}
		}

		cont = nodesList.Continue
		if cont == "" {
			return nodesProperties, nil
		}
	}
}
