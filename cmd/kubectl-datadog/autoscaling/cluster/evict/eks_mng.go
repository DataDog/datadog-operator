package evict

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/eks"
	"k8s.io/client-go/kubernetes"
)

var errEKSDrainIncomplete = errors.New("EKS managed node group drain did not complete within the timeout")

type EKSManagedNodeGroupAPI interface {
	DescribeNodegroup(ctx context.Context, in *eks.DescribeNodegroupInput, opts ...func(*eks.Options)) (*eks.DescribeNodegroupOutput, error)
	UpdateNodegroupConfig(ctx context.Context, in *eks.UpdateNodegroupConfigInput, opts ...func(*eks.Options)) (*eks.UpdateNodegroupConfigOutput, error)
}

func evictEKSManagedNodeGroup(ctx context.Context, eksAPI EKSManagedNodeGroupAPI, clientset kubernetes.Interface, clusterName, nodegroupName string, drainOpts nodeDrainOptions) error {
	panic("TODO: evictEKSManagedNodeGroup — implemented in PR https://github.com/DataDog/datadog-operator/pull/3174")
}
