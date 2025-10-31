package guess

import (
	"cmp"
	"context"
	"fmt"
	"hash/fnv"
	"log"
	"maps"
	"regexp"
	"slices"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const NodeListChunkSize = 100

type NodeGroupKey struct {
	AMIID          string
	SecurityGroups []string
	Labels         map[string]string
	Taints         []ekstypes.Taint
}

func (key NodeGroupKey) Sum64() uint64 {
	h := fnv.New64()
	h.Write([]byte(key.AMIID))
	for _, k := range slices.Sorted(maps.Keys(key.Labels)) {
		h.Write([]byte(k))
		h.Write([]byte(key.Labels[k]))
	}
	for _, x := range slices.SortedFunc(slices.Values(key.Taints), func(a, b ekstypes.Taint) int {
		if a.Key != nil && b.Key != nil {
			if c := cmp.Compare(*a.Key, *b.Key); c != 0 {
				return c
			}
		} else if a.Key != nil {
			return 1
		} else if b.Key != nil {
			return -1
		}
		if a.Value != nil && b.Value != nil {
			if c := cmp.Compare(*a.Value, *b.Value); c != 0 {
				return c
			}
		} else if a.Value != nil {
			return 1
		} else if b.Value != nil {
			return -1
		}
		return cmp.Compare(a.Effect, b.Effect)
	}) {
		if x.Key != nil {
			h.Write([]byte(*x.Key))
		}
		if x.Value != nil {
			h.Write([]byte(*x.Value))
		}
		h.Write([]byte(x.Effect))
	}
	return h.Sum64()
}

var awsProviderIDRegexp = regexp.MustCompile(`^aws:///[^/]+/(i-[0-9a-f]+)$`)

func GetNodesProperties(ctx context.Context, clientset *kubernetes.Clientset, client *ec2.Client) ([]NodeGroupProperties, error) {
	nodeGroupProperties := make(map[uint64]NodeGroupProperties)

	var cont string
	for {
		nodesList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{
			Limit:    NodeListChunkSize,
			Continue: cont,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list nodes: %w", err)
		}

		nodes := lo.Filter(nodesList.Items, func(node corev1.Node, _ int) bool {
			_, ok := node.Labels["karpenter.k8s.aws/ec2nodeclass"]
			return !ok
		})

		instances, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			InstanceIds: lo.Map(nodes, func(node corev1.Node, _ int) string {
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
				sg := lo.Map(instance.SecurityGroups, func(sg ec2types.GroupIdentifier, _ int) string { return *sg.GroupId })

				h := NodeGroupKey{
					AMIID:          *instance.ImageId,
					SecurityGroups: sg,
				}.Sum64()
				ng := nodeGroupProperties[h]

				ng.AMIID = *instance.ImageId
				ng.SecurityGroups = sg

				if instance.SubnetId != nil {
					ng.Subnets = slices.Compact(slices.Sorted(slices.Values(append(ng.Subnets, *instance.SubnetId))))
				}

				nodeGroupProperties[h] = ng
			}
		}

		cont = nodesList.Continue
		if cont == "" {
			return slices.Collect(maps.Values(nodeGroupProperties)), nil
		}
	}
}
