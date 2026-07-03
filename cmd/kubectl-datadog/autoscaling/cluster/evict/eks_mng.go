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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/pager"

	commonaws "github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/aws"
)

type EKSManagedNodeGroupAPI interface {
	UpdateNodegroupConfig(ctx context.Context, in *eks.UpdateNodegroupConfigInput, opts ...func(*eks.Options)) (*eks.UpdateNodegroupConfigOutput, error)
}

func evictEKSManagedNodeGroup(ctx context.Context, eksAPI EKSManagedNodeGroupAPI, clientset kubernetes.Interface, clusterName, nodegroupName string, nodes []string, drainOpts nodeDrainOptions) error {
	cordoned, errs := cordonNodes(ctx, clientset, nodes, drainOpts.DryRun)
	for _, node := range cordoned {
		if err := drainNode(ctx, clientset, node.Name, drainOpts); err != nil {
			errs = append(errs, fmt.Errorf("drain node %s: %w", node.Name, err))
		}
	}
	if len(errs) > 0 {
		// A node still holds workloads: do NOT scale to zero, which would
		// terminate instances with pods still on them and bypass their PDBs.
		// The group is left untouched for a re-run.
		return fmt.Errorf("EKS node group %s/%s drain incomplete: %w", clusterName, nodegroupName, errors.Join(errs...))
	}

	if drainOpts.DryRun {
		log.Printf("[dry-run] would scale EKS node group %s/%s to min=desired=0 (max preserved)", clusterName, nodegroupName)
		return nil
	}

	if _, err := eksAPI.UpdateNodegroupConfig(ctx, &eks.UpdateNodegroupConfigInput{
		ClusterName:   awssdk.String(clusterName),
		NodegroupName: awssdk.String(nodegroupName),
		ScalingConfig: &ekstypes.NodegroupScalingConfig{
			MinSize:     awssdk.Int32(0),
			DesiredSize: awssdk.Int32(0),
		},
	}); err != nil {
		return fmt.Errorf("UpdateNodegroupConfig: %w", err)
	}
	log.Printf("EKS node group %s/%s drained; scaling config set to min=desired=0 (max preserved).", clusterName, nodegroupName)

	return waitEKSNodegroupEmpty(ctx, clientset, clusterName, nodegroupName, drainOpts.NodeTimeout, drainOpts.PollInterval)
}

// waitEKSNodegroupEmpty polls the Kubernetes API for nodes carrying the
// `eks.amazonaws.com/nodegroup=<name>` label until none remain or the deadline
// expires (the nodes are already drained; this waits for EKS to terminate the
// instances). The wait is bounded by drainOpts.NodeTimeout — for large node
// groups, callers should raise this above the default.
func waitEKSNodegroupEmpty(ctx context.Context, clientset kubernetes.Interface, clusterName, nodegroupName string, timeout, pollInterval time.Duration) error {
	selector := commonaws.LabelEKSNodegroup + "=" + nodegroupName
	p := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
		return clientset.CoreV1().Nodes().List(ctx, opts)
	})
	var remaining int
	err := wait.PollUntilContextTimeout(ctx, pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		remaining = 0
		if err := p.EachListItem(ctx, metav1.ListOptions{LabelSelector: selector}, func(runtime.Object) error {
			remaining++
			return nil
		}); err != nil {
			return false, fmt.Errorf("list EKS managed node group %s/%s nodes: %w", clusterName, nodegroupName, err)
		}
		return remaining == 0, nil
	})
	if err != nil {
		return fmt.Errorf("EKS node group %s/%s drain incomplete, %d node(s) still present: %w", clusterName, nodegroupName, remaining, err)
	}
	log.Printf("EKS node group %s/%s fully drained.", clusterName, nodegroupName)
	return nil
}
