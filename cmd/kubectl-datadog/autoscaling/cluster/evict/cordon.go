package evict

import (
	"context"
	"fmt"
	"log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

// cordonNode marks the node Unschedulable. The Get + mutate + Update is
// wrapped in RetryOnConflict to survive races against kubelet, the scheduler,
// and any other controller that mutates Node objects.
func cordonNode(ctx context.Context, clientset kubernetes.Interface, name string, dryRun bool) error {
	if dryRun {
		log.Printf("[dry-run] would cordon node %s", name)
		return nil
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get node %s: %w", name, err)
		}
		if node.Spec.Unschedulable {
			return nil
		}
		node.Spec.Unschedulable = true
		if _, err = clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{}); err != nil {
			return err
		}
		log.Printf("Cordoned node %s.", name)
		return nil
	})
}
