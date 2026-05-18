package evict

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
)

// EC2API is the subset of *ec2.Client used by evictStandalone. Defined as an
// interface so unit tests can stub the AWS SDK out cheaply.
type EC2API interface {
	TerminateInstances(ctx context.Context, in *ec2.TerminateInstancesInput, opts ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error)
}

// evictStandalone cordons, drains, then terminates every standalone EC2
// instance (one whose Kubernetes Node has an `aws:///<az>/i-<hex>` providerID
// but is not in any ASG, EKS managed node group, or Karpenter NodePool).
// Terminating standalone instances directly is safe: by definition they have
// no controller that would relaunch them.
//
// Safety rule: an EC2 instance is only terminated when its node was fully
// drained. If draining failed (PDB-blocked, timeout, etc.), we leave the
// underlying instance intact — terminating it would kill the still-running
// pods. A re-run picks up where this one stopped.
func evictStandalone(ctx context.Context, clientset kubernetes.Interface, ec2API EC2API, nodes []string, drainOpts nodeDrainOptions) error {
	var (
		errs               []error
		drainedInstanceIDs []string
	)

	for _, nodeName := range nodes {
		node, err := clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			errs = append(errs, fmt.Errorf("node %s: %w", nodeName, err))
			continue
		}
		id, hasInstanceID := clusterinfo.ExtractEC2InstanceID(node)
		if !hasInstanceID {
			log.Printf("Warning: node %s has unexpected providerID %q; cannot terminate the underlying instance", nodeName, node.Spec.ProviderID)
		}
		if err := cordonNode(ctx, clientset, nodeName, drainOpts.DryRun); err != nil {
			errs = append(errs, fmt.Errorf("cordon node %s: %w", nodeName, err))
			continue
		}
		if err := drainNode(ctx, clientset, nodeName, drainOpts); err != nil {
			errs = append(errs, fmt.Errorf("drain node %s: %w", nodeName, err))
			continue // do NOT terminate this instance: workloads are still on it
		}
		if hasInstanceID {
			drainedInstanceIDs = append(drainedInstanceIDs, id)
		}
	}

	if drainOpts.DryRun {
		for _, id := range drainedInstanceIDs {
			log.Printf("[dry-run] would terminate standalone instance %s", id)
		}
		return errors.Join(errs...)
	}

	if len(drainedInstanceIDs) > 0 {
		if _, err := ec2API.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
			InstanceIds: drainedInstanceIDs,
		}); err != nil {
			errs = append(errs, fmt.Errorf("TerminateInstances: %w", err))
		} else {
			log.Printf("Terminated %d standalone EC2 instance(s).", len(drainedInstanceIDs))
		}
	}
	return errors.Join(errs...)
}
