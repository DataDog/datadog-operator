package evict

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"k8s.io/client-go/kubernetes"
)

type EC2API interface {
	TerminateInstances(ctx context.Context, in *ec2.TerminateInstancesInput, opts ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error)
}

func evictStandalone(ctx context.Context, clientset kubernetes.Interface, ec2API EC2API, nodes []string, drainOpts nodeDrainOptions) error {
	panic("TODO: evictStandalone — implemented in PR https://github.com/DataDog/datadog-operator/pull/3176")
}
