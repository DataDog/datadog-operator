package evict

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/eks"
	"k8s.io/client-go/kubernetes"
)

// errEKSDrainIncomplete is returned (wrapped) by waitEKSNodegroupEmpty when
// the deadline expires while nodes still carry the node group label. The
// orchestrator uses errors.Is to distinguish this case ("EKS drain is in
// progress but slow — keep temp PDBs in place") from a failed
// UpdateNodegroupConfig call ("EKS has not started draining — cleanup is
// safe").
var errEKSDrainIncomplete = errors.New("EKS managed node group drain did not complete within the timeout")

// EKSManagedNodeGroupAPI is the subset of *eks.Client used by
// evictEKSManagedNodeGroup. Defined as an interface so unit tests can stub the
// AWS SDK out cheaply.
type EKSManagedNodeGroupAPI interface {
	DescribeNodegroup(ctx context.Context, in *eks.DescribeNodegroupInput, opts ...func(*eks.Options)) (*eks.DescribeNodegroupOutput, error)
	UpdateNodegroupConfig(ctx context.Context, in *eks.UpdateNodegroupConfigInput, opts ...func(*eks.Options)) (*eks.UpdateNodegroupConfigOutput, error)
}

// evictEKSManagedNodeGroup delegates the drain to EKS by setting the node
// group's scaling config to min=desired=0 (max is preserved from the existing
// scaling config because the EKS API rejects `maxSize < 1`). The EKS control
// plane then cordons and evicts pods from each node (respecting
// PodDisruptionBudgets) before terminating the underlying EC2 instances. We
// do not cordon or evict from kubectl-datadog because doing so concurrently
// with the EKS-managed drain can produce confusing logs and double the load
// on the apiserver.
//
// UpdateNodegroupConfig returns immediately, but we then **wait** for the
// drain to actually complete by polling the Kubernetes API for nodes carrying
// the `eks.amazonaws.com/nodegroup=<name>` label. Without that wait, the
// caller (Run) would proceed to cleanup any temporary PodDisruptionBudgets
// before EKS even started evicting, leaving cross-type workloads (a
// Deployment with replicas on EKS MNG and on an ASG/Karpenter NodePool, for
// instance) unprotected mid-eviction.
//
// We never delete the node group: it may be Terraform-/CloudFormation-managed,
// and only the original owner should remove it.
func evictEKSManagedNodeGroup(ctx context.Context, eksAPI EKSManagedNodeGroupAPI, clientset kubernetes.Interface, clusterName, nodegroupName string, drainOpts nodeDrainOptions) error {
	panic("TODO: evictEKSManagedNodeGroup — implemented in PR #9")
}
