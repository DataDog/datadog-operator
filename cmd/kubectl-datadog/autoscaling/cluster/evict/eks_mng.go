package evict

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	commonaws "github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/aws"
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
	UpdateNodegroupConfig(ctx context.Context, in *eks.UpdateNodegroupConfigInput, opts ...func(*eks.Options)) (*eks.UpdateNodegroupConfigOutput, error)
}

// evictEKSManagedNodeGroup delegates the drain to EKS by setting the node
// group's scaling config to min=max=desired=0. The EKS control plane then
// cordons and evicts pods from each node (respecting PodDisruptionBudgets)
// before terminating the underlying EC2 instances. We do not cordon or evict
// from kubectl-datadog because doing so concurrently with the EKS-managed
// drain can produce confusing logs and double the load on the apiserver.
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
	if drainOpts.DryRun {
		log.Printf("[dry-run] would scale EKS node group %s/%s to min=max=desired=0 and wait for EKS to drain", clusterName, nodegroupName)
		return nil
	}
	if _, err := eksAPI.UpdateNodegroupConfig(ctx, &eks.UpdateNodegroupConfigInput{
		ClusterName:   awssdk.String(clusterName),
		NodegroupName: awssdk.String(nodegroupName),
		ScalingConfig: &ekstypes.NodegroupScalingConfig{
			MinSize:     awssdk.Int32(0),
			MaxSize:     awssdk.Int32(0),
			DesiredSize: awssdk.Int32(0),
		},
	}); err != nil {
		return fmt.Errorf("UpdateNodegroupConfig: %w", err)
	}
	log.Printf("EKS node group %s/%s scaling config set to 0; waiting for EKS to drain its nodes…", clusterName, nodegroupName)

	return waitEKSNodegroupEmpty(ctx, clientset, clusterName, nodegroupName, drainOpts.NodeTimeout, drainOpts.PollInterval)
}

// waitEKSNodegroupEmpty polls the Kubernetes API for nodes carrying the
// `eks.amazonaws.com/nodegroup=<name>` label until none remain or the deadline
// expires. The EKS drain is bounded by drainOpts.NodeTimeout — for large node
// groups, callers should raise this above the default.
func waitEKSNodegroupEmpty(ctx context.Context, clientset kubernetes.Interface, clusterName, nodegroupName string, timeout, pollInterval time.Duration) error {
	selector := commonaws.LabelEKSNodegroup + "=" + nodegroupName
	deadline := time.Now().Add(timeout)
	for {
		list, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			// We already accepted the EKS scaling change, so EKS may still
			// be draining. Wrap errEKSDrainIncomplete so the orchestrator
			// keeps the temporary PDBs in place; a re-run will reach the
			// poll loop again and cleanup once EKS converges.
			return fmt.Errorf("list EKS managed node group %s/%s nodes: %w: %w", clusterName, nodegroupName, err, errEKSDrainIncomplete)
		}
		if len(list.Items) == 0 {
			log.Printf("EKS node group %s/%s fully drained.", clusterName, nodegroupName)
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("EKS node group %s/%s: %d node(s) still present after %s: %w", clusterName, nodegroupName, len(list.Items), timeout, errEKSDrainIncomplete)
		}
		select {
		case <-time.After(pollInterval):
		case <-ctx.Done():
			// Cancellation (SIGINT) — per design, temp PDBs may leak and
			// a subsequent run reaps them via the label selector. Do NOT
			// wrap errEKSDrainIncomplete here.
			return ctx.Err()
		}
	}
}
