package evict

import (
	"context"
	"fmt"
	"log"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

// cordonNodes marks every node in the group Unschedulable up front, before any
// of them is drained — mirroring how EKS marks a managed node group
// unschedulable before draining it. A pod evicted from one node is then never
// rescheduled onto another node of the same group that is itself about to be
// drained.
//
// It returns the Node objects that are now cordoned and safe to drain, so the
// caller can read their immutable fields (e.g. providerID) without a second
// Get. A node that is already gone is skipped silently (absent from both
// return values); a node that fails to cordon is recorded in errs and left out
// of cordoned so the caller never drains a node that can still receive pods.
func cordonNodes(ctx context.Context, clientset kubernetes.Interface, nodes []string, dryRun bool) (cordoned []*corev1.Node, errs []error) {
	for _, nodeName := range nodes {
		node, err := cordonNode(ctx, clientset, nodeName, dryRun)
		if err != nil {
			errs = append(errs, fmt.Errorf("cordon node %s: %w", nodeName, err))
			continue
		}
		if node != nil {
			cordoned = append(cordoned, node)
		}
	}
	return cordoned, errs
}

// cordonNode marks the node Unschedulable and returns the resulting Node so the
// caller can read immutable fields (e.g. providerID) without a second Get. It
// returns (nil, nil) when the node is already gone: there is nothing to
// schedule onto a deleted node, and a re-run rediscovers any surviving nodes.
// The Get + mutate + Update is wrapped in RetryOnConflict to survive races
// against kubelet, the scheduler, and any other controller that mutates Node
// objects.
func cordonNode(ctx context.Context, clientset kubernetes.Interface, name string, dryRun bool) (*corev1.Node, error) {
	if dryRun {
		node, err := clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get node %s: %w", name, err)
		}
		log.Printf("[dry-run] would cordon node %s", name)
		return node, nil
	}
	var cordoned *corev1.Node
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil // node already gone: nothing to schedule on it
			}
			return fmt.Errorf("failed to get node %s: %w", name, err)
		}
		if !node.Spec.Unschedulable {
			node.Spec.Unschedulable = true
			node, err = clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					return nil // deleted concurrently: nothing to schedule on it
				}
				return fmt.Errorf("failed to update node %s: %w", name, err)
			}
			log.Printf("Cordoned node %s.", name)
		}
		cordoned = node
		return nil
	})
	return cordoned, err
}
