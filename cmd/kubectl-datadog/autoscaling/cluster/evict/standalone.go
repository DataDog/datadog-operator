package evict

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"k8s.io/client-go/kubernetes"
)

// EC2API is the subset of *ec2.Client used by evictStandalone. Defined as an
// interface so unit tests can stub the AWS SDK out cheaply.
type EC2API interface {
	TerminateInstances(ctx context.Context, in *ec2.TerminateInstancesInput, opts ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error)
}

// evictStandalone cordons and drains every standalone EC2 instance (one whose
// Kubernetes Node has an `aws:///<az>/i-<hex>` providerID but is not in any
// ASG, EKS managed node group, or Karpenter NodePool). Every node is cordoned
// up front (so pods evicted from one node are never rescheduled onto another
// node of the group that is itself about to be drained), then each instance is
// terminated as soon as its node has drained cleanly so the IaaS capacity is
// freed without waiting for the rest of the group. Terminating standalone
// instances directly is safe: by definition they have no controller that would
// relaunch them.
//
// Safety rule: an EC2 instance is only terminated when its node was fully
// drained. If draining failed (PDB-blocked, timeout, etc.), we leave the
// underlying instance intact — terminating it would kill the still-running
// pods. A re-run picks up where this one stopped.
func evictStandalone(ctx context.Context, clientset kubernetes.Interface, ec2API EC2API, nodes []string, drainOpts nodeDrainOptions) error {
	panic("TODO: evictStandalone — implemented in PR #11")
}
